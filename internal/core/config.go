package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

type Scope int

const (
	ScopeRoots Scope = iota
	ScopeGlobal
)

type Config struct {
	Scope                  Scope
	RootPaths              []string
	EditorInstancesPath    string
	EditorScanInterval     time.Duration
	EditorRetryDelay       time.Duration
	EditorTimeout          time.Duration
	EditorToolTimeout      time.Duration
	OpenProjectTimeout     time.Duration
	GodotPath              string
	GodotVersion           string
	GodotVersionIsExplicit bool
	NoAutoInstall          bool
	ForceHeadless          bool
	ForceOffscreen         bool
	OffscreenSize          string
	ForceAutoApprove       bool
	ProjectBasePath        string
	X11Display             string
	UpdateCheckInterval    time.Duration
	UpdateCheck            string
	Debug                  bool
	SavedConfigPath        string
	CloseUnattendedOnExit  bool
}

type SavedConfig struct {
	GodotVersion    string `json:"godot_version"`
	ProjectBasePath string `json:"project_base_path"`
	UpdateCheck     string `json:"update_check,omitempty"`
}

const (
	SettingGodotVersion    = "godot_version"
	SettingProjectBasePath = "project_base_path"
	SettingUpdateCheck     = "update_check"
)

const (
	UpdateCheckOn  = "on"
	UpdateCheckOff = "off"
)

var SettingNames = []string{SettingGodotVersion, SettingProjectBasePath, SettingUpdateCheck}

func CheckSettingName(name string) error {
	if slices.Contains(SettingNames, name) {
		return nil
	}
	return fmt.Errorf("unknown setting %q; the settings are %s", name, strings.Join(SettingNames, ", "))
}

func (sc SavedConfig) Setting(name string) (string, error) {
	if err := CheckSettingName(name); err != nil {
		return "", err
	}
	switch name {
	case SettingGodotVersion:
		return sc.GodotVersion, nil
	case SettingUpdateCheck:
		return sc.UpdateCheck, nil
	default:
		return sc.ProjectBasePath, nil
	}
}

func (sc *SavedConfig) SetSetting(name, value string) error {
	if err := CheckSettingName(name); err != nil {
		return err
	}
	switch name {
	case SettingGodotVersion:
		sc.GodotVersion = value
	case SettingUpdateCheck:
		sc.UpdateCheck = value
	default:
		sc.ProjectBasePath = value
	}
	return nil
}

func GetConfigPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "godai", "config.json"), nil
	case "linux":
		xdg_config_home := os.Getenv("XDG_CONFIG_HOME")
		if xdg_config_home != "" {
			return filepath.Join(xdg_config_home, "godai", "config.json"), nil
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "godai", "config.json"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "godai", "config.json"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func GetCachePath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		cache := os.Getenv("LOCALAPPDATA")
		if cache == "" {
			cache = os.Getenv("TEMP")
		}
		if cache == "" {
			return "", errors.New("neither LOCALAPPDATA nor TEMP is set")
		}
		return filepath.Join(cache, "godai"), nil
	case "linux":
		xdg_cache_home := os.Getenv("XDG_CACHE_HOME")
		if xdg_cache_home != "" {
			return filepath.Join(xdg_cache_home, "godai"), nil
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".cache", "godai"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", "godai"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func GetInstancesPath() (string, error) {
	cachePath, err := GetCachePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(cachePath, "instances"), nil
}

func GetUpdateCheckCachePath() (string, error) {
	cachePath, err := GetCachePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(cachePath, "update-check.json"), nil
}

func LoadConfig(path string) (*SavedConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := SavedConfig{}
	if err := json.Unmarshal(b, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func SavedUpdateCheckOff(path string) bool {
	sc, err := LoadConfig(path)
	return err == nil && sc.UpdateCheck == UpdateCheckOff
}

// SavedGodotVersion is the default version named in the config file, empty
// when there isn't one or the file can't be read.
func SavedGodotVersion(path string) string {
	sc, err := LoadConfig(path)
	if err != nil {
		return ""
	}
	return sc.GodotVersion
}

// SetSavedGodotVersion writes godot_version to the config file, leaving the
// other settings alone. An empty name unsets it.
func SetSavedGodotVersion(path, name string) error {
	sc, err := LoadConfig(path)
	if err != nil {
		sc = &SavedConfig{}
	}

	sc.GodotVersion = name
	return SaveConfig(path, sc)
}

func SaveConfig(path string, config *SavedConfig) error {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}

	return nil
}
