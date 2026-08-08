package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultBaseURL     = "https://gitlab.com/api/v4"
	DefaultProjectPath = "snopek-games/godai"
	DefaultAppName     = "godai-mcp"

	backupSuffix = ".old"

	maxAssetSize     = 512 << 20
	maxBinarySize    = 512 << 20
	maxChecksumsSize = 1 << 20
	downloadTimeout  = 10 * time.Minute
	maxRedirects     = 10
)

var (
	ErrNoRelease        = errors.New("no suitable release was found")
	ErrBinaryNotFound   = errors.New("executable not found in release asset")
	ErrNoBackup         = errors.New("no previous version to roll back to")
	ErrChecksumMissing  = errors.New("release asset isn't listed in the checksums file")
	ErrChecksumMismatch = errors.New("release asset doesn't match its checksum")
)

type Config struct {
	CurrentVersion string
	BaseURL        string
	ProjectPath    string
	AppName        string
	Variant        string
	HTTPClient     *http.Client
	// AllowUnverified installs a release without checking it against the
	// published checksums, and accepts releases that publish none.
	AllowUnverified bool
}

type Updater struct {
	currentVersion  Version
	baseURL         string
	projectPath     string
	appName         string
	variant         Variant
	httpClient      *http.Client
	allowUnverified bool
}

type Release struct {
	Version       Version
	TagName       string
	AssetName     string
	AssetURL      string
	ChecksumsName string
	ChecksumsURL  string
}

func New(config Config) (*Updater, error) {
	currentVersion, err := ParseVersion(config.CurrentVersion)
	if err != nil {
		return nil, fmt.Errorf("the running version is unusable: %w", err)
	}

	var variant Variant
	if config.Variant != "" {
		variant, err = variantByName(config.Variant)
	} else {
		variant, err = VariantForRuntime()
	}
	if err != nil {
		return nil, err
	}

	updater := &Updater{
		currentVersion:  currentVersion,
		baseURL:         strings.TrimSuffix(orDefault(config.BaseURL, DefaultBaseURL), "/"),
		projectPath:     orDefault(config.ProjectPath, DefaultProjectPath),
		appName:         orDefault(config.AppName, DefaultAppName),
		variant:         variant,
		httpClient:      httpsOnlyClient(config.HTTPClient),
		allowUnverified: config.AllowUnverified,
	}

	return updater, nil
}

func httpsOnlyClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}

	checked := *client
	checked.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return requireHTTPS(req.URL.String())
	}

	return &checked
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (u *Updater) CurrentVersion() Version {
	return u.currentVersion
}

func (u *Updater) IsNewer(release *Release) bool {
	return release.Version.Compare(u.currentVersion) > 0
}

// See https://docs.gitlab.com/api/releases/
type gitLabRelease struct {
	TagName         string `json:"tag_name"`
	UpcomingRelease bool   `json:"upcoming_release"`
	Assets          struct {
		Links []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"links"`
	} `json:"assets"`
}

// DetectLatest returns the newest stable release that has a build for this
// platform.
func (u *Updater) DetectLatest(ctx context.Context) (*Release, error) {
	releases, err := u.listReleases(ctx)
	if err != nil {
		return nil, err
	}

	var latest *Release
	for _, release := range releases {
		version, err := ParseVersion(release.TagName)
		if err != nil {
			slog.Debug("skipping release with an unparseable tag", "tag", release.TagName, "error", err)
			continue
		}
		if version.IsPrerelease() {
			slog.Debug("skipping pre-release", "tag", release.TagName)
			continue
		}
		if release.UpcomingRelease {
			slog.Debug("skipping upcoming release", "tag", release.TagName)
			continue
		}
		if latest != nil && version.Compare(latest.Version) <= 0 {
			continue
		}

		assetName := fmt.Sprintf("%s-%s-%s.zip", u.appName, u.variant.Name, release.TagName)
		assetURL := findAssetLink(release, assetName)
		if assetURL == "" {
			slog.Debug("skipping release without a build for this platform",
				"tag", release.TagName, "expectedAsset", assetName)
			continue
		}

		checksumsName := ChecksumsName(release.TagName)
		checksumsURL := findAssetLink(release, checksumsName)
		if checksumsURL == "" && !u.allowUnverified {
			slog.Debug("skipping release without a checksums file",
				"tag", release.TagName, "expectedAsset", checksumsName)
			continue
		}

		latest = &Release{
			Version:       version,
			TagName:       release.TagName,
			AssetName:     assetName,
			AssetURL:      assetURL,
			ChecksumsName: checksumsName,
			ChecksumsURL:  checksumsURL,
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("%w for %s (%s/%s)", ErrNoRelease, u.variant.Name, runtime.GOOS, runtime.GOARCH)
	}

	slog.Debug("found the latest release", "tag", latest.TagName, "asset", latest.AssetURL)
	return latest, nil
}

func ChecksumsName(tagName string) string {
	return "checksums-" + tagName + ".txt"
}

func findAssetLink(release gitLabRelease, name string) string {
	for _, link := range release.Assets.Links {
		if link.Name == name {
			return link.URL
		}
	}
	return ""
}

func (u *Updater) listReleases(ctx context.Context) ([]gitLabRelease, error) {
	// The project path is one path segment, so its slash stays escaped.
	requestURL := fmt.Sprintf("%s/projects/%s/releases?per_page=100",
		u.baseURL, url.PathEscape(u.projectPath))

	body, err := u.get(ctx, requestURL)
	if err != nil {
		return nil, fmt.Errorf("checking for the latest release: %w", err)
	}
	defer body.Close()

	var releases []gitLabRelease
	if err := json.NewDecoder(io.LimitReader(body, maxAssetSize)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("reading the list of releases: %w", err)
	}

	return releases, nil
}

func (u *Updater) get(ctx context.Context, requestURL string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", fmt.Sprintf("%s/%s", u.appName, u.currentVersion))
	request.Header.Set("Accept", "application/json")

	response, err := u.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("%s: unexpected response status %s", requestURL, response.Status)
	}

	return response.Body, nil
}

// Update replaces the executable at exePath with a release, and returns the
// path the previous version was kept at.
func (u *Updater) Update(ctx context.Context, release *Release, exePath string) (string, error) {
	info, err := os.Stat(exePath)
	if err != nil {
		return "", fmt.Errorf("looking at %s: %w", exePath, err)
	}

	installDir := filepath.Dir(exePath)
	if err := checkWritable(installDir, exePath); err != nil {
		return "", err
	}

	newPath, err := u.downloadExecutable(ctx, release, installDir, info.Mode().Perm())
	if err != nil {
		return "", err
	}
	defer os.Remove(newPath)

	backupPath := BackupPath(exePath)

	// Windows won't let us write to the running executable, but it will let us
	// rename it.
	if err := os.Rename(exePath, backupPath); err != nil {
		return "", fmt.Errorf("moving %s aside: %w", exePath, err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		if restoreErr := os.Rename(backupPath, exePath); restoreErr != nil {
			return "", fmt.Errorf("installing %s: %w (restoring the previous version also failed: %v; it's at %s)",
				exePath, err, restoreErr, backupPath)
		}
		return "", fmt.Errorf("installing %s: %w", exePath, err)
	}

	return backupPath, nil
}

func (u *Updater) downloadExecutable(ctx context.Context, release *Release, installDir string, mode fs.FileMode) (string, error) {
	if err := requireHTTPS(release.AssetURL); err != nil {
		return "", err
	}

	var want string
	if !u.allowUnverified {
		if err := requireHTTPS(release.ChecksumsURL); err != nil {
			return "", err
		}
		var err error
		if want, err = u.expectedChecksum(ctx, release); err != nil {
			return "", err
		}
	}

	assetPath, got, err := u.downloadAsset(ctx, release)
	if err != nil {
		return "", err
	}
	defer os.Remove(assetPath)

	switch {
	case u.allowUnverified:
		slog.Warn("installing a download that was not verified against the release checksums",
			"asset", release.AssetName, "sha256", got)
	case got != want:
		return "", fmt.Errorf("%w: %s is %s, but %s says %s",
			ErrChecksumMismatch, release.AssetName, got, release.ChecksumsName, want)
	}

	return u.unpackExecutable(assetPath, release, installDir, mode)
}

func requireHTTPS(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("release asset URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("refusing to download the release asset over %q: %s", parsed.Scheme, rawURL)
	}
	return nil
}

func (u *Updater) expectedChecksum(ctx context.Context, release *Release) (string, error) {
	slog.Debug("downloading checksums", "url", release.ChecksumsURL)

	body, err := u.get(ctx, release.ChecksumsURL)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", release.ChecksumsName, err)
	}
	defer body.Close()

	checksums, err := io.ReadAll(io.LimitReader(body, maxChecksumsSize))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", release.ChecksumsName, err)
	}

	sum, ok := findChecksum(string(checksums), release.AssetName)
	if !ok {
		return "", fmt.Errorf("%w: %s in %s", ErrChecksumMissing, release.AssetName, release.ChecksumsName)
	}

	return sum, nil
}

// findChecksum reads the sha256sum output produced by the release job.
func findChecksum(checksums, assetName string) (string, bool) {
	for line := range strings.SplitSeq(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			continue
		}
		// sha256sum marks files it read in binary mode with a leading "*".
		if strings.TrimPrefix(fields[1], "*") == assetName {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func (u *Updater) downloadAsset(ctx context.Context, release *Release) (string, string, error) {
	slog.Debug("downloading release asset", "url", release.AssetURL)

	body, err := u.get(ctx, release.AssetURL)
	if err != nil {
		return "", "", fmt.Errorf("downloading %s: %w", release.AssetName, err)
	}
	defer body.Close()

	file, err := os.CreateTemp("", "godai-mcp-*.zip")
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(body, maxAssetSize+1))
	if err != nil {
		os.Remove(file.Name())
		return "", "", fmt.Errorf("downloading %s: %w", release.AssetName, err)
	}
	if written > maxAssetSize {
		os.Remove(file.Name())
		return "", "", fmt.Errorf("%s is larger than the %d byte limit", release.AssetName, int64(maxAssetSize))
	}

	slog.Debug("downloaded release asset", "path", file.Name(), "bytes", written)
	return file.Name(), hex.EncodeToString(hash.Sum(nil)), nil
}

func (u *Updater) unpackExecutable(assetPath string, release *Release, installDir string, mode fs.FileMode) (string, error) {
	archive, err := zip.OpenReader(assetPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", release.AssetName, err)
	}
	defer archive.Close()

	wantName := u.appName + "-" + u.variant.Name + u.variant.Ext

	for _, file := range archive.File {
		if file.FileInfo().IsDir() || path.Base(file.Name) != wantName {
			continue
		}
		if file.UncompressedSize64 > maxBinarySize {
			return "", fmt.Errorf("%s in %s is larger than the %d byte limit",
				wantName, release.AssetName, int64(maxBinarySize))
		}
		return copyToTempFile(file, installDir, mode)
	}

	return "", fmt.Errorf("%w: no %q in %s", ErrBinaryNotFound, wantName, release.AssetName)
}

// The executable goes into installDir so that installing it is a rename
// within one filesystem.
func copyToTempFile(file *zip.File, installDir string, mode fs.FileMode) (destPath string, err error) {
	source, err := file.Open()
	if err != nil {
		return "", err
	}
	defer source.Close()

	dest, err := os.CreateTemp(installDir, ".godai-mcp-new-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := dest.Close(); err == nil && closeErr != nil {
			destPath, err = "", fmt.Errorf("unpacking %s: %w", file.Name, closeErr)
		}
		if err != nil {
			os.Remove(dest.Name())
		}
	}()

	var written int64
	if written, err = io.Copy(dest, io.LimitReader(source, maxBinarySize+1)); err != nil {
		return "", fmt.Errorf("unpacking %s: %w", file.Name, err)
	}
	if written > maxBinarySize {
		return "", fmt.Errorf("%s is larger than the %d byte limit", file.Name, int64(maxBinarySize))
	}
	// CreateTemp makes the file 0600.
	if err = dest.Chmod(mode | 0o100); err != nil {
		return "", err
	}

	return dest.Name(), nil
}

func BackupPath(exePath string) string {
	return exePath + backupSuffix
}

func Rollback(exePath string) error {
	backupPath := BackupPath(exePath)
	if _, err := os.Stat(backupPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w (nothing at %s)", ErrNoBackup, backupPath)
		}
		return err
	}

	installDir := filepath.Dir(exePath)
	if err := checkWritable(installDir, exePath); err != nil {
		return err
	}

	// The two files are swapped rather than one being deleted, because Windows
	// won't delete the running executable, and because it lets a rollback
	// itself be rolled back.
	swapPath, err := reserveTempPath(installDir)
	if err != nil {
		return err
	}

	if err := os.Rename(exePath, swapPath); err != nil {
		return fmt.Errorf("moving %s aside: %w", exePath, err)
	}
	if err := os.Rename(backupPath, exePath); err != nil {
		if restoreErr := os.Rename(swapPath, exePath); restoreErr != nil {
			return fmt.Errorf("restoring %s: %w (putting the current version back also failed: %v; it's at %s)",
				exePath, err, restoreErr, swapPath)
		}
		return fmt.Errorf("restoring %s: %w", exePath, err)
	}
	if err := os.Rename(swapPath, backupPath); err != nil {
		return fmt.Errorf("keeping the replaced version at %s: %w (it's at %s)", backupPath, err, swapPath)
	}

	return nil
}

func reserveTempPath(dir string) (string, error) {
	file, err := os.CreateTemp(dir, ".godai-mcp-swap-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	file.Close()
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func ExecutablePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the running executable: %w", err)
	}

	// Symlinks are resolved so that an update replaces the real file.
	resolved, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", exePath, err)
	}

	return resolved, nil
}
