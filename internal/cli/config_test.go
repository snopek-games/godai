package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/internal/isolationtest"

	"github.com/matryer/is"
)

func TestConfigReadsAndWritesSettings(t *testing.T) {
	is := is.New(t)

	if runtime.GOOS == "windows" {
		t.Skip("stands in for a Godot binary by being an executable shell script")
	}

	dir := isolationtest.Isolate(t)
	configPath := filepath.Join(isolation.GodaiConfigDir(dir), "config.json")

	version := linkTestEngine(t, dir)
	basePath := filepath.Join(dir, "projects")
	is.NoErr(os.MkdirAll(basePath, 0o755))

	_, err := runCLI(t, []string{"godai", "config",
		"--set", core.SettingGodotVersion + "=" + version,
		"--set", core.SettingProjectBasePath + "=" + basePath,
	})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath), core.SavedConfig{GodotVersion: version, ProjectBasePath: basePath})

	otherBasePath := filepath.Join(dir, "other-projects")
	is.NoErr(os.MkdirAll(otherBasePath, 0o755))
	_, err = runCLI(t, []string{"godai", "config", "--set", core.SettingProjectBasePath + "=" + otherBasePath})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath), core.SavedConfig{GodotVersion: version, ProjectBasePath: otherBasePath})

	out, err := runCLI(t, []string{"godai", "config", core.SettingGodotVersion})
	is.NoErr(err)
	is.Equal(strings.TrimSpace(out), version)

	_, err = runCLI(t, []string{"godai", "config", "--unset", core.SettingGodotVersion})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath), core.SavedConfig{ProjectBasePath: otherBasePath})
}

// linkTestEngine gives a name to a script standing in for a Godot build.
func linkTestEngine(t *testing.T, dir string) string {
	t.Helper()

	godotPath := filepath.Join(dir, "godot")
	if err := os.WriteFile(godotPath, []byte("#!/bin/sh\necho 4.5.stable.official.a2b3c4d5e\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, []string{"godai", "engine", "link", "test-build", godotPath}); err != nil {
		t.Fatal(err)
	}
	return "test-build"
}

func TestConfigRejectsBadArguments(t *testing.T) {
	dir := isolationtest.Isolate(t)

	for _, args := range [][]string{
		{"godai", "config", "godto_path"},
		{"godai", "config", "--set", "godto_path=/somewhere"},
		{"godai", "config", "--unset", "godto_path"},
		{"godai", "config", "--set", core.SettingGodotVersion + "="},
		{"godai", "config", core.SettingGodotVersion, "--set", core.SettingGodotVersion + "=/somewhere"},
	} {
		err := runQuietly(t, args)
		if code := ExitCodeFor(err); code != ExitUsage {
			t.Errorf("%v: got exit code %d, want %d (%v)", args, code, ExitUsage, err)
		}
	}

	if _, err := os.Stat(filepath.Join(isolation.GodaiConfigDir(dir), "config.json")); !os.IsNotExist(err) {
		t.Error("a rejected `godai config` wrote the config file anyway")
	}
}

func readSavedConfig(t *testing.T, path string) core.SavedConfig {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	saved := core.SavedConfig{}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	return saved
}
