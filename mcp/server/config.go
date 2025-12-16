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
	EditorBasePort   int
	EditorPortCount  int
	EditorRetryDelay time.Duration
	EditorTimeout    time.Duration
	DefaultGodotPath string
	ProjectBasePath  string
	X11Display       string
	Debug            bool
	SavedConfigPath  string
}

type SavedConfig struct {
	DefaultGodotPath string `json:"godot_path"`
	ProjectBasePath  string `json:"project_path"`
}

func GetConfigPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "godai-mcp", "config.json"), nil
	case "linux":
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
