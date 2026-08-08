package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type PermissionError struct {
	Dir     string
	ExePath string
	Err     error
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf("can't update %s: no permission to write to %s; %s", e.ExePath, e.Dir, e.Hint())
}

func (e *PermissionError) Unwrap() error {
	return e.Err
}

func (e *PermissionError) Hint() string {
	if hint := PackageManagerHint(e.ExePath); hint != "" {
		return hint
	}
	if runtime.GOOS == "windows" {
		return "re-run this from an Administrator command prompt, or install godai-mcp somewhere you own"
	}
	return fmt.Sprintf("re-run it as 'sudo %s self-update', or install godai-mcp somewhere you own", e.ExePath)
}

func PackageManagerHint(exePath string) string {
	if slices.Contains(strings.Split(filepath.ToSlash(exePath), "/"), "node_modules") {
		return "godai-mcp was installed by npm, which will replace this executable again on its next install; " +
			"update it with 'npm install -g @snopek-games/godai-mcp@latest' instead"
	}
	return ""
}

func checkWritable(dir, exePath string) error {
	file, err := os.CreateTemp(dir, ".godai-mcp-check-*")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return &PermissionError{Dir: dir, ExePath: exePath, Err: err}
		}
		return fmt.Errorf("checking whether %s is writable: %w", dir, err)
	}

	name := file.Name()
	file.Close()
	os.Remove(name)

	return nil
}
