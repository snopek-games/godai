package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

const testGodotVersion = "4.5-stable"

func TestSetConfigKeepsTheOtherSetting(t *testing.T) {
	is := is.New(t)

	dir := canonicalTempDir(t)
	configPath := filepath.Join(dir, "config.json")
	basePath := filepath.Join(dir, "projects")
	is.NoErr(os.MkdirAll(basePath, 0o755))

	is.NoErr(SaveConfig(configPath, &SavedConfig{
		GodotVersion:    testGodotVersion,
		ProjectBasePath: basePath,
	}))

	s, err := New(Config{SavedConfigPath: configPath})
	is.NoErr(err)

	otherBasePath := filepath.Join(dir, "other-projects")
	is.NoErr(os.MkdirAll(otherBasePath, 0o755))
	is.NoErr(s.SetConfig(SavedConfig{ProjectBasePath: otherBasePath}))

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.GodotVersion, testGodotVersion)
	is.Equal(saved.ProjectBasePath, otherBasePath)
}

func TestSetConfigValidatesUpdateCheck(t *testing.T) {
	is := is.New(t)

	configPath := filepath.Join(t.TempDir(), "config.json")
	s, err := New(Config{SavedConfigPath: configPath})
	is.NoErr(err)

	is.True(s.SetConfig(SavedConfig{UpdateCheck: "sometimes"}) != nil)

	is.NoErr(s.SetConfig(SavedConfig{UpdateCheck: UpdateCheckOff}))
	is.Equal(s.GetConfig().UpdateCheck, UpdateCheckOff)

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.UpdateCheck, UpdateCheckOff)
}

func TestSaveSettingNeverClearsASavedSetting(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	is.NoErr(SaveConfig(configPath, &SavedConfig{GodotVersion: testGodotVersion}))

	s, err := New(Config{SavedConfigPath: configPath})
	is.NoErr(err)
	s.logSaveSetting(SettingProjectBasePath, dir)

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.GodotVersion, testGodotVersion)
	is.Equal(saved.ProjectBasePath, dir)
}

func TestSaveSettingLeavesTheLiveConfigUnsaved(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	s, err := New(Config{
		SavedConfigPath: configPath,
		GodotVersion:    testGodotVersion,
	})
	is.NoErr(err)
	s.logSaveSetting(SettingProjectBasePath, dir)

	saved, err := LoadConfig(configPath)
	is.NoErr(err)
	is.Equal(saved.GodotVersion, "") // came from a flag, so it isn't ours to remember
	is.Equal(saved.ProjectBasePath, dir)
}
