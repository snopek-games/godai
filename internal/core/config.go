package core

import (
	"encoding/json"
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
	Scope               Scope
	RootPaths           []string
	EditorInstancesPath string
	EditorScanInterval  time.Duration
	EditorRetryDelay    time.Duration
	EditorTimeout       time.Duration
	EditorToolTimeout   time.Duration
	OpenProjectTimeout  time.Duration
	DefaultGodotPath    string
	ProjectBasePath     string
	X11Display          string
	UpdateCheckInterval time.Duration
	Debug               bool
	SavedConfigPath     string
	CloseHeadlessOnExit bool
}

type SavedConfig struct {
	DefaultGodotPath string `json:"godot_path"`
	ProjectBasePath  string `json:"project_base_path"`
}

const (
	SettingGodotPath       = "godot_path"
	SettingProjectBasePath = "project_base_path"
)

var SettingNames = []string{SettingGodotPath, SettingProjectBasePath}

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
	if name == SettingGodotPath {
		return sc.DefaultGodotPath, nil
	}
	return sc.ProjectBasePath, nil
}

func (sc *SavedConfig) SetSetting(name, value string) error {
	if err := CheckSettingName(name); err != nil {
		return err
	}
	if name == SettingGodotPath {
		sc.DefaultGodotPath = value
	} else {
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
		appdata := os.Getenv("TEMP")
		return filepath.Join(appdata, "godai"), nil
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

func SaveConfig(path string, config *SavedConfig) error {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return err
	}

	b, err := json.Marshal(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}

	return nil
}
