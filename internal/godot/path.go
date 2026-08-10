package godot

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// xdgDir resolves an XDG base directory. Per the XDG Base Directory
// Specification, the environment variable is used only when it holds an
// absolute path; otherwise the default (relative to the home directory) is
// used.
func xdgDir(envVar string, defaultRel ...string) (string, error) {
	if dir := os.Getenv(envVar); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{home}, defaultRel...)...), nil
}

func GetEditorDataPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "Godot"), nil
	case "linux":
		dir, err := xdgDir("XDG_DATA_HOME", ".local", "share")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "godot"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Godot"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func GetEditorSettingsPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "Godot"), nil
	case "linux":
		dir, err := xdgDir("XDG_CONFIG_HOME", ".config")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "godot"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Godot"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func GetEditorCachePath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		temp := os.Getenv("TEMP")
		return filepath.Join(temp, "Godot"), nil
	case "linux":
		dir, err := xdgDir("XDG_CACHE_HOME", ".cache")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "godot"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", "Godot"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func IsPathUnderAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		under, err := IsPathUnderRoot(path, root)
		if err != nil {
			slog.Debug("unable to check whether project is under root", "project", path, "root", root, "error", err)
			continue
		}
		if under {
			return true
		}
	}
	return false
}

// IsPathUnderRoot reports whether path lies within root in the physical directory
// tree. It resolves symlinks and compares directories by identity via
// os.SameFile (device+inode on Unix, volume serial + file index on Windows),
// so the check is immune to case-sensitivity differences across Linux, macOS,
// and Windows. Both path and root must exist on disk.
//
// Note: this is physical path-ancestry. A bind mount or directory junction that
// exposes the same files at a different path is treated as a distinct location
// and will not be reported as "under" root.
func IsPathUnderRoot(path, root string) (bool, error) {
	// Canonicalize both ends so a symlink anywhere in either path can't fool the
	// comparison, and so filepath.Dir walks real parents rather than lexical ones.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}

	rootInfo, err := os.Stat(root)
	if err != nil {
		return false, err
	}

	// Walk up from path, comparing each ancestor to root by file identity.
	for {
		if info, err := os.Stat(path); err == nil && os.SameFile(info, rootInfo) {
			return true, nil
		}
		parent := filepath.Dir(path)
		if parent == path { // reached the volume/filesystem root
			return false, nil
		}
		path = parent
	}
}
