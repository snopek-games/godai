package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/matryer/is"
)

func TestGetConfigPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG config path logic is linux-specific")
	}
	is := is.New(t)

	// XDG_CONFIG_HOME, when set, is used directly.
	t.Setenv("XDG_CONFIG_HOME", "/xdg/cfg")
	path, err := GetConfigPath()
	is.NoErr(err)
	is.Equal(path, "/xdg/cfg/godai-mcp/config.json")

	// Otherwise it falls back to ~/.config.
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	is.NoErr(err)
	path, err = GetConfigPath()
	is.NoErr(err)
	is.Equal(path, filepath.Join(home, ".config", "godai-mcp", "config.json"))
}

func TestGetCachePath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG cache path logic is linux-specific")
	}
	is := is.New(t)

	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	path, err := GetCachePath()
	is.NoErr(err)
	is.Equal(path, "/xdg/cache/godai-mcp")

	t.Setenv("XDG_CACHE_HOME", "")
	home, err := os.UserHomeDir()
	is.NoErr(err)
	path, err = GetCachePath()
	is.NoErr(err)
	is.Equal(path, filepath.Join(home, ".cache", "godai-mcp"))
}

func TestGetInstancesPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG cache path logic is linux-specific")
	}
	is := is.New(t)

	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	path, err := GetInstancesPath()
	is.NoErr(err)
	is.Equal(path, "/xdg/cache/godai-mcp/instances")
}

func TestSaveAndLoadConfig(t *testing.T) {
	is := is.New(t)

	// A nested path that doesn't exist yet, to exercise SaveConfig's MkdirAll.
	path := filepath.Join(t.TempDir(), "sub", "config.json")

	want := &SavedConfig{
		DefaultGodotPath: "/usr/bin/godot",
		ProjectBasePath:  "/home/me/projects",
	}
	is.NoErr(SaveConfig(path, want))

	got, err := LoadConfig(path)
	is.NoErr(err)
	is.Equal(got.DefaultGodotPath, want.DefaultGodotPath)
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
