package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matryer/is"
)

const (
	testAssetPrefix = "godai-cli"
	testBinaryName  = "godai"
	testVariant     = "linux-x86_64"
	// The tests ask for the linux-x86_64 build whatever platform they run on.
	testVariantExt = ""
)

type fakeRelease struct {
	tag        string
	upcoming   bool
	variants   []string
	omitBinary bool
	// omitChecksums publishes no checksums file at all, omitChecksumLine one
	// that doesn't mention our asset, and corruptAsset serves an asset that
	// doesn't match the checksum published for it.
	omitChecksums    bool
	omitChecksumLine bool
	corruptAsset     bool
}

// fakeGitLab stands in for the parts of the GitLab API the updater uses, so
// the tests work offline and can arrange releases the real project doesn't
// have.
type fakeGitLab struct {
	server           *httptest.Server
	assetRequests    atomic.Int64
	releaseRequests  atomic.Int64
	projectPath      string
	redirectAssetsTo string
}

// newFakeGitLab serves TLS because the updater refuses to download an
// executable over anything but https.
func newFakeGitLab(t *testing.T, releases []fakeRelease) *fakeGitLab {
	t.Helper()

	fake := &fakeGitLab{}
	assets := map[string][]byte{}

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/releases"):
			fake.releaseRequests.Add(1)
			path := strings.TrimSuffix(r.URL.EscapedPath(), "/releases")
			fake.projectPath = strings.TrimPrefix(path, "/api/v4/projects/")

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(fake.releaseJSON(releases))
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			fake.assetRequests.Add(1)
			if fake.redirectAssetsTo != "" {
				http.Redirect(w, r, fake.redirectAssetsTo+r.URL.Path, http.StatusFound)
				return
			}
			data, ok := assets[strings.TrimPrefix(r.URL.Path, "/assets/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	})

	fake.server = httptest.NewTLSServer(handler)
	t.Cleanup(fake.server.Close)

	for _, release := range releases {
		checksums := &strings.Builder{}
		for _, variant := range release.variantNames() {
			name := assetName(variant, release.tag)
			data := makeReleaseZip(t, variant, release.tag, release.omitBinary)

			if !(release.omitChecksumLine && variant == testVariant) {
				fmt.Fprintf(checksums, "%x  %s\n", sha256.Sum256(data), name)
			}
			if release.corruptAsset && variant == testVariant {
				data = append(data, "tampered"...)
			}
			assets[name] = data
		}
		if !release.omitChecksums {
			assets[ChecksumsName(release.tag)] = []byte(checksums.String())
		}
	}

	return fake
}

func (f *fakeGitLab) releaseJSON(releases []fakeRelease) []map[string]any {
	out := make([]map[string]any, 0, len(releases))
	for _, release := range releases {
		links := []map[string]any{
			{"name": "godai-addon-" + release.tag + ".zip", "url": f.server.URL + "/assets/addon.zip"},
		}
		for _, variant := range release.variantNames() {
			name := assetName(variant, release.tag)
			links = append(links, map[string]any{"name": name, "url": f.server.URL + "/assets/" + name})
		}
		if !release.omitChecksums {
			name := ChecksumsName(release.tag)
			links = append(links, map[string]any{"name": name, "url": f.server.URL + "/assets/" + name})
		}

		out = append(out, map[string]any{
			"tag_name":         release.tag,
			"name":             "Release " + release.tag,
			"upcoming_release": release.upcoming,
			"assets":           map[string]any{"links": links},
		})
	}
	return out
}

func (r fakeRelease) variantNames() []string {
	if r.variants == nil {
		return []string{testVariant}
	}
	return r.variants
}

func assetName(variant, tag string) string {
	return fmt.Sprintf("%s-%s-%s.zip", testAssetPrefix, variant, tag)
}

func binaryContent(tag string) string {
	return testBinaryName + " " + tag
}

// makeReleaseZip builds an asset laid out the way the release job in
// .gitlab-ci.yml lays them out.
func makeReleaseZip(t *testing.T, variant, tag string, omitBinary bool) []byte {
	t.Helper()
	is := is.New(t)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)

	dir := fmt.Sprintf("%s-%s-%s", testAssetPrefix, variant, tag)
	files := []struct{ name, content string }{
		{dir + "/README.md", "# godai"},
		{dir + "/LICENSE.txt", "MIT"},
	}
	if !omitBinary {
		ext := ""
		if strings.HasPrefix(variant, "windows-") {
			ext = ".exe"
		}
		files = append(files, struct{ name, content string }{
			name:    fmt.Sprintf("%s/%s%s", dir, testBinaryName, ext),
			content: binaryContent(tag),
		})
	}

	for _, file := range files {
		w, err := archive.Create(file.name)
		is.NoErr(err)
		_, err = w.Write([]byte(file.content))
		is.NoErr(err)
	}
	is.NoErr(archive.Close())

	return buffer.Bytes()
}

func newTestUpdater(t *testing.T, fake *fakeGitLab, currentVersion string) *Updater {
	t.Helper()
	is := is.New(t)

	updater, err := New(Config{
		CurrentVersion: currentVersion,
		BaseURL:        fake.server.URL + "/api/v4",
		Variant:        testVariant,
		HTTPClient:     fake.server.Client(),
	})
	is.NoErr(err)

	return updater
}

func installTestExecutable(t *testing.T, content string) string {
	t.Helper()
	is := is.New(t)

	exePath := filepath.Join(t.TempDir(), testBinaryName+testVariantExt)
	is.NoErr(os.WriteFile(exePath, []byte(content), 0o755))

	return exePath
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	is := is.New(t)

	data, err := os.ReadFile(path)
	is.NoErr(err)

	return string(data)
}

func TestDetectLatest(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.4.0"},
		{tag: "v0.3.2"},
	})
	updater := newTestUpdater(t, fake, "0.3.2")

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.4.0")
	is.Equal(release.Version.String(), "0.4.0")
	is.Equal(release.AssetName, "godai-cli-linux-x86_64-v0.4.0.zip")
	is.Equal(release.ChecksumsName, "checksums-v0.4.0.txt")
	is.True(updater.IsNewer(release))

	// Unescaping the slash gets a 404 from GitLab.
	is.Equal(fake.projectPath, "snopek-games%2Fgodai")
}

func TestDetectLatestSkipsPrereleases(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.5.0-beta.1"},
		{tag: "v0.5.0-rc.1"},
		{tag: "v0.4.0"},
	})
	updater := newTestUpdater(t, fake, "0.3.2")

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.4.0")
}

func TestDetectLatestSkipsUpcomingReleases(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.5.0", upcoming: true},
		{tag: "v0.4.0"},
	})
	updater := newTestUpdater(t, fake, "0.3.2")

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.4.0")
}

func TestDetectLatestPicksHighestVersionNotNewestRelease(t *testing.T) {
	is := is.New(t)

	// A patch to an older series can be released after a newer one.
	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.3.3"},
		{tag: "v0.4.0"},
		{tag: "v0.3.2"},
	})
	updater := newTestUpdater(t, fake, "0.3.2")

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.4.0")
}

func TestDetectLatestSkipsReleasesWithoutOurVariant(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.4.0", variants: []string{"macos-arm64", "windows-x86_64"}},
		{tag: "v0.3.2"},
	})
	updater := newTestUpdater(t, fake, "0.3.0")

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.3.2")
}

func TestDetectLatestSkipsReleasesWithoutChecksums(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.4.0", omitChecksums: true},
		{tag: "v0.3.2"},
	})
	updater := newTestUpdater(t, fake, "0.3.0")

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.3.2")
}

func TestAllowUnverified(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.4.0", omitChecksums: true, corruptAsset: true},
		{tag: "v0.3.2"},
	})
	updater, err := New(Config{
		CurrentVersion:  "0.3.2",
		BaseURL:         fake.server.URL + "/api/v4",
		Variant:         testVariant,
		HTTPClient:      fake.server.Client(),
		AllowUnverified: true,
	})
	is.NoErr(err)
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.Equal(release.TagName, "v0.4.0")
	is.Equal(release.ChecksumsURL, "")

	_, err = updater.Update(context.Background(), release, exePath)
	is.NoErr(err)
	is.Equal(readFile(t, exePath), binaryContent("v0.4.0"))
}

func TestAllowUnverifiedInstallsATamperedAsset(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0", corruptAsset: true}})
	updater, err := New(Config{
		CurrentVersion:  "0.3.2",
		BaseURL:         fake.server.URL + "/api/v4",
		Variant:         testVariant,
		HTTPClient:      fake.server.Client(),
		AllowUnverified: true,
	})
	is.NoErr(err)
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	_, err = updater.Update(context.Background(), release, exePath)
	is.NoErr(err)
	is.Equal(readFile(t, exePath), binaryContent("v0.4.0"))
}

func TestUpdateWithATamperedAsset(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0", corruptAsset: true}})
	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	_, err = updater.Update(context.Background(), release, exePath)
	is.True(errors.Is(err, ErrChecksumMismatch))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2")) // the executable is untouched
}

func TestUpdateWithAnUnlistedAsset(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0", omitChecksumLine: true}})
	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	_, err = updater.Update(context.Background(), release, exePath)
	is.True(errors.Is(err, ErrChecksumMissing))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2")) // the executable is untouched
}

func TestFindChecksum(t *testing.T) {
	is := is.New(t)

	sum := strings.Repeat("a", 64)
	checksums := "" +
		"deadbeef  short-hash.zip\n" +
		sum + "  godai-cli-linux-x86_64-v0.4.0.zip\n" +
		strings.Repeat("b", 64) + " *godai-cli-macos-arm64-v0.4.0.zip\n"

	got, ok := findChecksum(checksums, "godai-cli-linux-x86_64-v0.4.0.zip")
	is.True(ok)
	is.Equal(got, sum)

	got, ok = findChecksum(checksums, "godai-cli-macos-arm64-v0.4.0.zip")
	is.True(ok)
	is.Equal(got, strings.Repeat("b", 64))

	_, ok = findChecksum(checksums, "short-hash.zip")
	is.True(!ok) // a truncated hash doesn't count

	_, ok = findChecksum(checksums, "godai-cli-windows-x86_64-v0.4.0.zip")
	is.True(!ok)
}

func TestDetectLatestWithNothingUsable(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{
		{tag: "v0.4.0", variants: []string{"macos-arm64"}},
		{tag: "not-a-version"},
	})
	updater := newTestUpdater(t, fake, "0.3.2")

	_, err := updater.DetectLatest(context.Background())
	is.True(errors.Is(err, ErrNoRelease))
}

func TestIsNewer(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})

	updater := newTestUpdater(t, fake, "0.4.0")
	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	is.True(!updater.IsNewer(release))

	updater = newTestUpdater(t, fake, "0.4.0-dev")
	is.True(updater.IsNewer(release)) // a -dev build updates to its own release
}

func TestUpdate(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}, {tag: "v0.3.2"}})
	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	backupPath, err := updater.Update(context.Background(), release, exePath)
	is.NoErr(err)

	is.Equal(backupPath, exePath+".old")
	is.Equal(readFile(t, exePath), binaryContent("v0.4.0"))
	is.Equal(readFile(t, backupPath), binaryContent("v0.3.2"))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(exePath)
		is.NoErr(err)
		is.Equal(info.Mode().Perm(), os.FileMode(0o755))
	}

	entries, err := os.ReadDir(filepath.Dir(exePath))
	is.NoErr(err)
	is.Equal(len(entries), 2) // nothing left behind but the executable and its backup
}

func TestUpdateRefusesInsecureAssetURL(t *testing.T) {
	is := is.New(t)

	fake := &fakeGitLab{}
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fake.assetRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fake.releaseJSON([]fakeRelease{{tag: "v0.4.0"}}))
	})
	fake.server = httptest.NewServer(handler)
	t.Cleanup(fake.server.Close)

	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	_, err = updater.Update(context.Background(), release, exePath)
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "refusing"))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2")) // the executable is untouched
}

func TestUpdateRefusesRedirectToInsecureAssetURL(t *testing.T) {
	is := is.New(t)

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not the release you were looking for"))
	}))
	t.Cleanup(plain.Close)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	fake.redirectAssetsTo = plain.URL

	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	_, err = updater.Update(context.Background(), release, exePath)
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "refusing"))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2")) // the executable is untouched
}

func TestUpdateWithoutTheExecutableInTheAsset(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0", omitBinary: true}})
	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	_, err = updater.Update(context.Background(), release, exePath)
	is.True(errors.Is(err, ErrBinaryNotFound))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2")) // the executable is untouched
}

func TestRollback(t *testing.T) {
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)
	_, err = updater.Update(context.Background(), release, exePath)
	is.NoErr(err)

	is.NoErr(Rollback(exePath))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2"))

	is.Equal(readFile(t, BackupPath(exePath)), binaryContent("v0.4.0")) // the rolled-back version becomes the backup
	is.NoErr(Rollback(exePath))
	is.Equal(readFile(t, exePath), binaryContent("v0.4.0"))
	is.Equal(readFile(t, BackupPath(exePath)), binaryContent("v0.3.2"))
}

func TestRollbackWithoutABackup(t *testing.T) {
	is := is.New(t)

	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	err := Rollback(exePath)
	is.True(errors.Is(err, ErrNoBackup))
	is.Equal(readFile(t, exePath), binaryContent("v0.3.2")) // the executable is untouched
}
