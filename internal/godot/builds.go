package godot

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBuildsAPIURL = "https://api.github.com/repos/godotengine/godot-builds"

	checksumsAssetName = "SHA512-SUMS.txt"

	maxReleasePages  = 10
	releasesPageSize = 100

	maxDownloadSize  = 4 << 30
	maxChecksumsSize = 1 << 20
	maxJSONSize      = 64 << 20
	maxRedirects     = 10

	downloadTimeout   = 30 * time.Minute
	releasesCacheTTL  = time.Hour
	releasesCacheName = "godot-builds.json"
)

var (
	ErrReleaseNotFound = errors.New("no such Godot release")
	ErrAssetNotFound   = errors.New("release has no such download")
	ErrChecksumMissing = errors.New("download isn't listed in " + checksumsAssetName)
	ErrChecksumFailed  = errors.New("download doesn't match its checksum")
)

type BuildAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type BuildRelease struct {
	TagName    string       `json:"tag_name"`
	Draft      bool         `json:"draft"`
	Prerelease bool         `json:"prerelease"`
	Assets     []BuildAsset `json:"assets"`
}

func (r *BuildRelease) Asset(name string) (BuildAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return BuildAsset{}, false
}

type BuildsClientConfig struct {
	BaseURL    string
	CacheDir   string
	Token      string
	UserAgent  string
	HTTPClient *http.Client
}

type BuildsClient struct {
	baseURL    string
	cacheDir   string
	token      string
	userAgent  string
	secure     bool
	httpClient *http.Client
}

func NewBuildsClient(config BuildsClientConfig) *BuildsClient {
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	token := config.Token
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "godai"
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = DefaultBuildsAPIURL
	}

	// Downloads have to use the same scheme the release list came over, so
	// that the real service is always reached over TLS while a local test
	// server can serve plain HTTP.
	secure := !strings.HasPrefix(baseURL, "http://")

	checked := *client
	checked.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return checkScheme(req.URL.String(), secure)
	}

	return &BuildsClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		cacheDir:   config.CacheDir,
		token:      token,
		userAgent:  userAgent,
		secure:     secure,
		httpClient: &checked,
	}
}

func (c *BuildsClient) Release(ctx context.Context, tag string) (*BuildRelease, error) {
	body, err := c.get(ctx, c.baseURL+"/releases/tags/"+url.PathEscape(tag), true)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrReleaseNotFound, tag)
		}
		return nil, err
	}
	defer body.Close()

	release := &BuildRelease{}
	if err := json.NewDecoder(io.LimitReader(body, maxJSONSize)).Decode(release); err != nil {
		return nil, fmt.Errorf("reading the %s release: %w", tag, err)
	}

	return release, nil
}

type cachedTags struct {
	FetchedAt time.Time `json:"fetched_at"`
	Tags      []string  `json:"tags"`
}

// ReleaseTags lists every published release tag, newest first. The list is
// cached, because it takes several requests and GitHub allows few of them
// without a token.
func (c *BuildsClient) ReleaseTags(ctx context.Context, refresh bool) ([]string, error) {
	if !refresh {
		if tags, ok := c.cachedTags(); ok {
			return tags, nil
		}
	}

	tags, err := c.fetchReleaseTags(ctx)
	if err != nil {
		return nil, err
	}

	c.saveTags(tags)
	return tags, nil
}

func (c *BuildsClient) fetchReleaseTags(ctx context.Context) ([]string, error) {
	tags := []string{}

	for page := 1; page <= maxReleasePages; page++ {
		requestURL := fmt.Sprintf("%s/releases?per_page=%d&page=%d", c.baseURL, releasesPageSize, page)
		body, err := c.get(ctx, requestURL, true)
		if err != nil {
			return nil, fmt.Errorf("listing the Godot releases: %w", err)
		}

		var releases []BuildRelease
		err = json.NewDecoder(io.LimitReader(body, maxJSONSize)).Decode(&releases)
		body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading the list of Godot releases: %w", err)
		}

		for _, release := range releases {
			if !release.Draft {
				tags = append(tags, release.TagName)
			}
		}

		if len(releases) < releasesPageSize {
			break
		}
	}

	return tags, nil
}

func (c *BuildsClient) cachePath() string {
	if c.cacheDir == "" {
		return ""
	}
	return filepath.Join(c.cacheDir, releasesCacheName)
}

func (c *BuildsClient) cachedTags() ([]string, bool) {
	path := c.cachePath()
	if path == "" {
		return nil, false
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	cached := cachedTags{}
	if err := json.Unmarshal(b, &cached); err != nil {
		return nil, false
	}
	if time.Since(cached.FetchedAt) > releasesCacheTTL || len(cached.Tags) == 0 {
		return nil, false
	}

	return cached.Tags, true
}

func (c *BuildsClient) saveTags(tags []string) {
	path := c.cachePath()
	if path == "" {
		return
	}

	b, err := json.Marshal(cachedTags{FetchedAt: time.Now(), Tags: tags})
	if err != nil {
		slog.Debug("unable to cache the release list", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Debug("unable to cache the release list", "error", err)
		return
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		slog.Debug("unable to cache the release list", "error", err)
	}
}

// Progress reports how far a download has got. Total is 0 when the server
// doesn't say how big the download is.
type Progress func(downloaded, total int64)

// Download fetches an asset to destPath and returns its SHA-512.
func (c *BuildsClient) Download(ctx context.Context, asset BuildAsset, destPath string, progress Progress) (string, error) {
	if err := checkScheme(asset.URL, c.secure); err != nil {
		return "", err
	}

	body, err := c.get(ctx, asset.URL, false)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", asset.Name, err)
	}
	defer body.Close()

	file, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha512.New()
	reader := io.LimitReader(body, maxDownloadSize+1)
	written, err := io.Copy(io.MultiWriter(file, hash), &progressReader{
		reader:   reader,
		total:    asset.Size,
		progress: progress,
	})
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", asset.Name, err)
	}
	if written > maxDownloadSize {
		return "", fmt.Errorf("%s is larger than the %d byte limit", asset.Name, int64(maxDownloadSize))
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (c *BuildsClient) VerifyChecksum(ctx context.Context, release *BuildRelease, assetName, sum string) error {
	checksums, ok := release.Asset(checksumsAssetName)
	if !ok {
		return fmt.Errorf("the %s release publishes no %s", release.TagName, checksumsAssetName)
	}

	body, err := c.get(ctx, checksums.URL, false)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", checksumsAssetName, err)
	}
	defer body.Close()

	contents, err := io.ReadAll(io.LimitReader(body, maxChecksumsSize))
	if err != nil {
		return fmt.Errorf("reading %s: %w", checksumsAssetName, err)
	}

	want, ok := findChecksum(string(contents), assetName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrChecksumMissing, assetName)
	}
	if want != strings.ToLower(sum) {
		return fmt.Errorf("%w: %s is %s, but %s says %s", ErrChecksumFailed, assetName, sum, checksumsAssetName, want)
	}

	return nil
}

// findChecksum reads the sha512sum output published with each release.
func findChecksum(checksums, assetName string) (string, bool) {
	for line := range strings.SplitSeq(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha512.Size*2 {
			continue
		}
		// sha512sum marks files it read in binary mode with a leading "*".
		if strings.TrimPrefix(fields[1], "*") == assetName {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

var errNotFound = errors.New("not found")

func (c *BuildsClient) get(ctx context.Context, requestURL string, wantJSON bool) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", c.userAgent)
	if wantJSON {
		request.Header.Set("Accept", "application/vnd.github+json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}

	switch {
	case response.StatusCode == http.StatusOK:
		return response.Body, nil
	case response.StatusCode == http.StatusNotFound:
		response.Body.Close()
		return nil, errNotFound
	case response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0":
		response.Body.Close()
		return nil, fmt.Errorf("GitHub's rate limit is used up%s; set GITHUB_TOKEN to raise it", rateLimitReset(response))
	default:
		response.Body.Close()
		return nil, fmt.Errorf("%s: unexpected response status %s", requestURL, response.Status)
	}
}

func rateLimitReset(response *http.Response) string {
	seconds, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(" until %s", time.Unix(seconds, 0).Local().Format(time.Kitchen))
}

func checkScheme(rawURL string, secure bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("download URL %q: %w", rawURL, err)
	}
	if parsed.Scheme == "https" || (!secure && parsed.Scheme == "http") {
		return nil
	}
	return fmt.Errorf("refusing to download over %q: %s", parsed.Scheme, rawURL)
}

type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	progress   Progress
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.downloaded += int64(n)
	if r.progress != nil && n > 0 {
		r.progress(r.downloaded, r.total)
	}
	return n, err
}
