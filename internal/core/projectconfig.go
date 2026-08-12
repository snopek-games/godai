package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ProjectConfigName is Godai's own file in a Godot project, meant to be
// committed alongside project.godot.
const ProjectConfigName = ".godai.json"

const projectGodotVersionKey = "godot_version"

func ProjectConfigPath(projectPath string) string {
	return filepath.Join(projectPath, ProjectConfigName)
}

// LoadProjectConfig reads .godai.json as a plain object, so that settings Godai
// doesn't know about survive being written back.
func LoadProjectConfig(projectPath string) (map[string]any, error) {
	path := ProjectConfigPath(projectPath)

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}

	config := map[string]any{}
	if err := json.Unmarshal(b, &config); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	return config, nil
}

// SaveProjectConfig writes .godai.json, removing it once nothing is left in it.
func SaveProjectConfig(projectPath string, config map[string]any) error {
	path := ProjectConfigPath(projectPath)

	if len(config) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// ProjectGodotVersion is the version the project is pinned to, or "" if it isn't.
func ProjectGodotVersion(projectPath string) (string, error) {
	config, err := LoadProjectConfig(projectPath)
	if err != nil {
		return "", err
	}

	value, ok := config[projectGodotVersionKey]
	if !ok {
		return "", nil
	}

	version, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s: %s isn't a version", ProjectConfigPath(projectPath), projectGodotVersionKey)
	}

	return version, nil
}

func SetProjectGodotVersion(projectPath, version string) error {
	config, err := LoadProjectConfig(projectPath)
	if err != nil {
		return err
	}

	config[projectGodotVersionKey] = version
	return SaveProjectConfig(projectPath, config)
}

// UnsetProjectGodotVersion reports whether the project was pinned to begin with.
func UnsetProjectGodotVersion(projectPath string) (bool, error) {
	config, err := LoadProjectConfig(projectPath)
	if err != nil {
		return false, err
	}

	if _, ok := config[projectGodotVersionKey]; !ok {
		return false, nil
	}

	delete(config, projectGodotVersionKey)
	return true, SaveProjectConfig(projectPath, config)
}
