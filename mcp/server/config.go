package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type Config struct {
	Global              bool
	RootPaths           []string
	EditorInstancesPath string
	EditorScanInterval  time.Duration
	EditorRetryDelay    time.Duration
	EditorTimeout       time.Duration
	DefaultGodotPath    string
	ProjectBasePath     string
	X11Display          string
	Debug               bool
	SavedConfigPath     string
}

type SavedConfig struct {
	DefaultGodotPath string `json:"godot_path"`
	ProjectBasePath  string `json:"project_base_path,omitempty"`
}

func GetConfigPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "godai-mcp", "config.json"), nil
	case "linux":
		xdg_config_home := os.Getenv("XDG_CONFIG_HOME")
		if xdg_config_home != "" {
			return filepath.Join(xdg_config_home, "godai-mcp", "config.json"), nil
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "godai-mcp", "config.json"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "godai-mcp", "config.json"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func GetCachePath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("TEMP")
		return filepath.Join(appdata, "godai-mcp"), nil
	case "linux":
		xdg_cache_home := os.Getenv("XDG_CACHE_HOME")
		if xdg_cache_home != "" {
			return filepath.Join(xdg_cache_home, "godai-mcp"), nil
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".cache", "godai-mcp"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", "godai-mcp"), nil
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
