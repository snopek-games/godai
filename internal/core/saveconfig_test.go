package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/matryer/is"
)

func TestSetConfigKeepsTheOtherSetting(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	godotPath := fakeGodotExecutable(t, dir)
	basePath := filepath.Join(dir, "projects")
	is.NoErr(os.MkdirAll(basePath, 0o755))

	is.NoErr(SaveConfig(configPath, &SavedConfig{
		DefaultGodotPath: godotPath,
		ProjectBasePath:  basePath,
	}))

	s, err := New(Config{SavedConfigPath: configPath})
	is.NoErr(err)

	otherBasePath := filepath.Join(dir, "other-projects")
	is.NoErr(os.MkdirAll(otherBasePath, 0o755))
	is.NoErr(s.SetConfig(SavedConfig{ProjectBasePath: otherBasePath}))

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.DefaultGodotPath, godotPath)
	is.Equal(saved.ProjectBasePath, otherBasePath)
}

func TestSaveSettingNeverClearsASavedSetting(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	godotPath := fakeGodotExecutable(t, dir)

	is.NoErr(SaveConfig(configPath, &SavedConfig{DefaultGodotPath: godotPath}))

	s, err := New(Config{SavedConfigPath: configPath})
	is.NoErr(err)
	s.logSaveSetting(SettingProjectBasePath, dir)

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.DefaultGodotPath, godotPath)
	is.Equal(saved.ProjectBasePath, dir)
}

func TestSaveSettingLeavesTheLiveConfigUnsaved(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	s, err := New(Config{
		SavedConfigPath:  configPath,
		DefaultGodotPath: fakeGodotExecutable(t, dir),
	})
	is.NoErr(err)
	s.logSaveSetting(SettingProjectBasePath, dir)

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.DefaultGodotPath, "") // came from a flag, so it isn't ours to remember
	is.Equal(saved.ProjectBasePath, dir)
}

func fakeGodotExecutable(t *testing.T, dir string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("stands in for a Godot binary by being an executable shell script")
	}

	path := filepath.Join(dir, "godot")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
