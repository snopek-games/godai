package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func TestConfigReadsAndWritesSettings(t *testing.T) {
	is := is.New(t)

	if runtime.GOOS == "windows" {
		t.Skip("stands in for a Godot binary by being an executable shell script")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	configPath := filepath.Join(dir, "godai", "config.json")

	godotPath := filepath.Join(dir, "godot")
	is.NoErr(os.WriteFile(godotPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	basePath := filepath.Join(dir, "projects")
	is.NoErr(os.MkdirAll(basePath, 0o755))

	_, err := runCLI(t, []string{"godai", "config",
		"--set", core.SettingGodotPath + "=" + godotPath,
		"--set", core.SettingProjectBasePath + "=" + basePath,
	})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath), core.SavedConfig{DefaultGodotPath: godotPath, ProjectBasePath: basePath})

	otherBasePath := filepath.Join(dir, "other-projects")
	is.NoErr(os.MkdirAll(otherBasePath, 0o755))
	_, err = runCLI(t, []string{"godai", "config", "--set", core.SettingProjectBasePath + "=" + otherBasePath})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath), core.SavedConfig{DefaultGodotPath: godotPath, ProjectBasePath: otherBasePath})

	out, err := runCLI(t, []string{"godai", "config", core.SettingGodotPath})
	is.NoErr(err)
	is.Equal(strings.TrimSpace(out), godotPath)

	_, err = runCLI(t, []string{"godai", "config", "--unset", core.SettingGodotPath})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath), core.SavedConfig{ProjectBasePath: otherBasePath})
}

func TestConfigRejectsBadArguments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	for _, args := range [][]string{
		{"godai", "config", "godto_path"},
		{"godai", "config", "--set", "godto_path=/somewhere"},
		{"godai", "config", "--unset", "godto_path"},
		{"godai", "config", "--set", core.SettingGodotPath + "="},
		{"godai", "config", core.SettingGodotPath, "--set", core.SettingGodotPath + "=/somewhere"},
	} {
		err := runQuietly(t, args)
		if code := ExitCodeFor(err); code != ExitUsage {
			t.Errorf("%v: got exit code %d, want %d (%v)", args, code, ExitUsage, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "godai", "config.json")); !os.IsNotExist(err) {
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
