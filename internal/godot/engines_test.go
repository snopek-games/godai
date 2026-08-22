package godot

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/fakebin"
)

const testTag = "4.5-stable"

func engineEntryName() string {
	switch runtime.GOOS {
	case "windows":
		return "Godot_v4.5-stable_win64.exe"
	case "darwin":
		return "Godot.app/Contents/MacOS/Godot"
	default:
		return "Godot_v4.5-stable_linux.x86_64"
	}
}

func zipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)

	for name, contents := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o755)

		writer, err := archive.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func buildsServer(t *testing.T, assets map[string][]byte) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	checksums := &strings.Builder{}
	for name, contents := range assets {
		sum := sha512.Sum512(contents)
		fmt.Fprintf(checksums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	assets[checksumsAssetName] = []byte(checksums.String())

	release := BuildRelease{TagName: testTag}
	for name := range assets {
		release.Assets = append(release.Assets, BuildAsset{
			Name: name,
			URL:  server.URL + "/download/" + name,
			Size: int64(len(assets[name])),
		})
	}

	mux.HandleFunc("/releases/tags/"+testTag, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(release)
	})
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]BuildRelease{release})
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		contents, ok := assets[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(contents)
	})

	return server
}

func testManager(t *testing.T, assets map[string][]byte) *EngineManager {
	t.Helper()

	dir := t.TempDir()
	server := buildsServer(t, assets)

	manager, err := NewEngineManager(EngineManagerConfig{
		CacheDir:     filepath.Join(dir, "cache"),
		ConfigDir:    filepath.Join(dir, "config"),
		TemplatesDir: filepath.Join(dir, "templates"),
		Client: NewBuildsClient(BuildsClientConfig{
			BaseURL:  server.URL,
			CacheDir: filepath.Join(dir, "cache"),
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	return manager
}

func mustParse(t *testing.T, s string) EngineVersion {
	t.Helper()

	version, err := ParseEngineVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func TestInstallAndRemoveEngine(t *testing.T) {
	is := is.New(t)

	version := mustParse(t, testTag)
	assetName, err := EngineAssetName(version)
	is.NoErr(err)

	manager := testManager(t, map[string][]byte{
		assetName: zipBytes(t, map[string]string{engineEntryName(): "godot"}),
	})

	engine, err := manager.Install(context.Background(), version, DownloadOptions{})
	is.NoErr(err)
	is.Equal(engine.Name, testTag)
	is.Equal(filepath.Base(engine.Path), filepath.Base(engineEntryName()))

	contents, err := os.ReadFile(engine.Path)
	is.NoErr(err)
	is.Equal(string(contents), "godot")

	engines, err := manager.List()
	is.NoErr(err)
	is.Equal(len(engines), 1)
	is.Equal(engines[0].Name, testTag)

	found, err := manager.Find("4.5")
	is.NoErr(err)
	is.Equal(found.Path, engine.Path)

	_, err = manager.Install(context.Background(), version, DownloadOptions{})
	is.True(errors.Is(err, ErrEngineInstalled)) // installing it twice is refused

	is.NoErr(manager.Remove(testTag))

	engines, err = manager.List()
	is.NoErr(err)
	is.Equal(len(engines), 0) // gone after Remove

	_, err = manager.Find(testTag)
	is.True(errors.Is(err, ErrEngineNotInstalled))
}

func TestInstallEngineRefusesABadChecksum(t *testing.T) {
	is := is.New(t)

	version := mustParse(t, testTag)
	assetName, err := EngineAssetName(version)
	is.NoErr(err)

	assets := map[string][]byte{assetName: zipBytes(t, map[string]string{engineEntryName(): "godot"})}
	manager := testManager(t, assets)

	// The checksums were published for what the release held a moment ago.
	assets[assetName] = zipBytes(t, map[string]string{engineEntryName(): "something else"})

	_, err = manager.Install(context.Background(), version, DownloadOptions{})
	is.True(errors.Is(err, ErrChecksumFailed))

	engines, err := manager.List()
	is.NoErr(err)
	is.Equal(len(engines), 0) // nothing was left behind
}

func TestInstallAndRemoveTemplates(t *testing.T) {
	is := is.New(t)

	version := mustParse(t, "4.5-mono")
	manager := testManager(t, map[string][]byte{
		TemplatesAssetName(version): zipBytes(t, map[string]string{
			"templates/version.txt":        "4.5.stable.mono",
			"templates/linux_debug.x86_64": "template",
		}),
	})

	is.True(!manager.TemplatesInstalled(version))

	is.NoErr(manager.InstallTemplates(context.Background(), version, DownloadOptions{}))
	is.True(manager.TemplatesInstalled(version))
	is.Equal(filepath.Base(manager.TemplatesPath(version)), "4.5.stable.mono")

	// The archive's own "templates" directory is dropped, because Godot looks
	// for the files directly under the version's directory.
	contents, err := os.ReadFile(filepath.Join(manager.TemplatesPath(version), "version.txt"))
	is.NoErr(err)
	is.Equal(string(contents), "4.5.stable.mono")

	err = manager.InstallTemplates(context.Background(), version, DownloadOptions{})
	is.True(errors.Is(err, ErrTemplatesInstalled))

	is.NoErr(manager.RemoveTemplates(version))
	is.True(!manager.TemplatesInstalled(version))
	is.True(errors.Is(manager.RemoveTemplates(version), ErrTemplatesNotInstalled))
}

func TestLinkedEngines(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	executable := filepath.Join(t.TempDir(), "godot")
	is.NoErr(os.WriteFile(executable, []byte("godot"), 0o755))

	_, err := manager.Link(context.Background(), "4.5", executable)
	is.True(errors.Is(err, ErrLinkNameIsVersion))

	_, err = manager.Link(context.Background(), "my-build", executable)
	is.NoErr(err)

	engine, err := manager.Find("my-build")
	is.NoErr(err)
	is.Equal(engine.Path, executable)
	is.True(engine.Linked)

	engines, err := manager.List()
	is.NoErr(err)
	is.Equal(len(engines), 1)
	is.True(engines[0].Linked)

	is.NoErr(manager.Remove("my-build"))

	_, err = manager.Find("my-build")
	is.True(err != nil) // the link is gone
}

func TestLinkingRecordsTheVersionTheBuildReports(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	executable, err := fakebin.Print(filepath.Join(t.TempDir(), "godot"), "4.4.1.stable.mono.official.b09f793f5")
	is.NoErr(err)

	engine, err := manager.Link(context.Background(), "my-build", executable)
	is.NoErr(err)
	is.Equal(engine.Name, "my-build")
	is.Equal(engine.Version.String(), "4.4.1-stable-mono")

	found, err := manager.Find("my-build")
	is.NoErr(err)
	is.Equal(found.Version.String(), "4.4.1-stable-mono")

	engines, err := manager.List()
	is.NoErr(err)
	is.Equal(len(engines), 1)
	is.Equal(engines[0].Version.String(), "4.4.1-stable-mono")
}

func TestSearchOrdersNewestFirst(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})

	versions, err := manager.Search(context.Background(), SearchOptions{})
	is.NoErr(err)
	is.Equal(len(versions), 1)
	is.Equal(versions[0].String(), testTag)

	versions, err = manager.Search(context.Background(), SearchOptions{Filter: "4.5"})
	is.NoErr(err)
	is.Equal(len(versions), 1)

	versions, err = manager.Search(context.Background(), SearchOptions{Filter: "4.6"})
	is.NoErr(err)
	is.Equal(len(versions), 0)
}

// A filter that matched anywhere in the version would answer "4.5" with 3.4.5.
func TestSearchFilterMatchesFromTheStart(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	manager.client = NewBuildsClient(BuildsClientConfig{
		BaseURL: tagsServer(t, []string{"4.5.1-stable", "4.5-stable", "3.4.5-stable"}).URL,
	})

	versions, err := manager.Search(context.Background(), SearchOptions{Filter: "4.5"})
	is.NoErr(err)
	is.Equal(len(versions), 2)
	is.Equal(versions[0].String(), "4.5.1-stable")
	is.Equal(versions[1].String(), "4.5-stable")
}

func tagsServer(t *testing.T, tags []string) *httptest.Server {
	t.Helper()

	releases := make([]BuildRelease, 0, len(tags))
	for _, tag := range tags {
		releases = append(releases, BuildRelease{TagName: tag})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}
