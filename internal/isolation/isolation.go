// Package isolation sets up the environment that makes godai and the Godot
// editor resolve their per-user config, data, and cache directories under a
// single base directory, so tests and evals never touch the developer's real
// ones. Each platform needs different variables.
package isolation

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func Env(base string) ([]string, error) {
	var env []string
	switch runtime.GOOS {
	case "linux":
		env = []string{
			"XDG_CONFIG_HOME=" + ConfigHome(base),
			"XDG_DATA_HOME=" + DataHome(base),
			"XDG_CACHE_HOME=" + CacheHome(base),
		}
	case "darwin":
		env = []string{"HOME=" + base}
	default:
		// TODO(windows): set APPDATA, LOCALAPPDATA, USERPROFILE, and TMP/TEMP to
		// redirect the directories the accessors below already describe.
		return nil, fmt.Errorf("directory isolation is not implemented on %s", runtime.GOOS)
	}
	for _, dir := range []string{ConfigHome(base), DataHome(base), CacheHome(base)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return env, nil
}

func ConfigHome(base string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(base, "Library", "Application Support")
	case "windows":
		return filepath.Join(base, "AppData", "Roaming")
	default:
		return filepath.Join(base, "config")
	}
}

func DataHome(base string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(base, "Library", "Application Support")
	case "windows":
		return filepath.Join(base, "AppData", "Roaming")
	default:
		return filepath.Join(base, "data")
	}
}

func CacheHome(base string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(base, "Library", "Caches")
	case "windows":
		return filepath.Join(base, "AppData", "Local")
	default:
		return filepath.Join(base, "cache")
	}
}

func GodaiConfigDir(base string) string {
	return filepath.Join(ConfigHome(base), "godai")
}

func GodaiCacheDir(base string) string {
	return filepath.Join(CacheHome(base), "godai")
}

func GodaiInstancesDir(base string) string {
	return filepath.Join(GodaiCacheDir(base), "instances")
}

func GodotEditorDataDir(base string) string {
	if runtime.GOOS == "linux" {
		return filepath.Join(DataHome(base), "godot")
	}
	return filepath.Join(DataHome(base), "Godot")
}
