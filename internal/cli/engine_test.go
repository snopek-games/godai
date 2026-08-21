package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func TestEngineRemoveUnsetsTheDefault(t *testing.T) {
	is := is.New(t)

	if runtime.GOOS == "windows" {
		t.Skip("stands in for a Godot binary by being an executable shell script")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	configPath := filepath.Join(dir, "godai", "config.json")

	version := linkTestEngine(t, dir)
	other := linkOtherTestEngine(t, dir)

	_, err := runCLI(t, []string{"godai", "config", "--set", core.SettingGodotVersion + "=" + version})
	is.NoErr(err)

	_, err = runCLI(t, []string{"godai", "engine", "remove", other})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath).GodotVersion, version) // untouched by removing the other engine

	_, err = runCLI(t, []string{"godai", "engine", "remove", version})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath).GodotVersion, "")
}

func TestEngineLinkRecordsTheVersionTheBuildReports(t *testing.T) {
	is := is.New(t)

	if runtime.GOOS == "windows" {
		t.Skip("stands in for a Godot binary by being an executable shell script")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	linkTestEngine(t, dir)

	out, err := runCLI(t, []string{"godai", "engine", "list"})
	is.NoErr(err)
	is.True(strings.Contains(out, "test-build (linked: 4.5-stable)"))
}

func linkOtherTestEngine(t *testing.T, dir string) string {
	t.Helper()

	godotPath := filepath.Join(dir, "other-godot")
	if err := os.WriteFile(godotPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, []string{"godai", "engine", "link", "other-build", godotPath}); err != nil {
		t.Fatal(err)
	}
	return "other-build"
}
