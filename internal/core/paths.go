package core

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CanonicalPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty path")
	}

	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}

	return real, nil
}

func ValidateDirectory(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		return errors.New("not a directory")
	}

	return nil
}

// ResolveGodotExecutable canonicalizes a Godot executable to an absolute path,
// looking a bare command name up on PATH, and checks that it runs.
func ResolveGodotExecutable(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}

	if filepath.Base(path) == path {
		found, err := exec.LookPath(path)
		if err != nil {
			return "", err
		}
		path = found
	}

	resolved, err := CanonicalPath(path)
	if err != nil {
		return "", err
	}

	fi, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}

	if fi.IsDir() || !fi.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}

	if err := exec.Command(resolved, "--version").Run(); err != nil {
		// An ExitError only says "exit status N", but it does tell us the file
		// ran, so the problem is what it is rather than how it's installed.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.New("does not run as a Godot executable")
		}
		return "", err
	}

	return resolved, nil
}

// checkExecutable is the cheap half of ResolveGodotExecutable, for an
// executable Godai installed itself and only needs to know is still there.
func checkExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	return nil
}

func resolveDirectory(path string) (string, error) {
	resolved, err := CanonicalPath(path)
	if err != nil {
		return "", err
	}
	if err := ValidateDirectory(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func dirExists(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return fi.IsDir(), nil
}
