package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/internal/isolationtest"
)

// The isolation accessors promise where godai will put things once the
// isolation env is applied, so the two must agree on every platform.
func TestGetConfigPath(t *testing.T) {
	is := is.New(t)

	base := isolationtest.Isolate(t)
	path, err := GetConfigPath()
	is.NoErr(err)
	is.Equal(path, filepath.Join(isolation.GodaiConfigDir(base), "config.json"))
}

func TestGetCachePath(t *testing.T) {
	is := is.New(t)

	base := isolationtest.Isolate(t)
	path, err := GetCachePath()
	is.NoErr(err)
	is.Equal(path, isolation.GodaiCacheDir(base))
}

func TestGetInstancesPath(t *testing.T) {
	is := is.New(t)

	base := isolationtest.Isolate(t)
	path, err := GetInstancesPath()
	is.NoErr(err)
	is.Equal(path, isolation.GodaiInstancesDir(base))
}

func TestPathsFallBackToHomeWithoutXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the XDG fallback only exists on linux")
	}
	is := is.New(t)

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	home, err := os.UserHomeDir()
	is.NoErr(err)

	path, err := GetConfigPath()
	is.NoErr(err)
	is.Equal(path, filepath.Join(home, ".config", "godai", "config.json"))

	path, err = GetCachePath()
	is.NoErr(err)
	is.Equal(path, filepath.Join(home, ".cache", "godai"))
}

func TestSaveAndLoadConfig(t *testing.T) {
	is := is.New(t)

	// A nested path that doesn't exist yet, to exercise SaveConfig's MkdirAll.
	path := filepath.Join(t.TempDir(), "sub", "config.json")

	want := &SavedConfig{
		GodotVersion:    "4.5-stable",
		ProjectBasePath: "/home/me/projects",
	}
	is.NoErr(SaveConfig(path, want))

	got, err := LoadConfig(path)
	is.NoErr(err)
	is.Equal(got.GodotVersion, want.GodotVersion)
	is.Equal(got.ProjectBasePath, want.ProjectBasePath)
}

func TestLoadConfigMissingFile(t *testing.T) {
	is := is.New(t)
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	is.True(err != nil) // a missing file is an error
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	is := is.New(t)
	path := filepath.Join(t.TempDir(), "config.json")
	is.NoErr(os.WriteFile(path, []byte("{not valid json"), 0o644))
	_, err := LoadConfig(path)
	is.True(err != nil) // malformed JSON is an error
}
