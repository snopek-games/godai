package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
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
		return "re-run this from an Administrator command prompt, or install godai somewhere you own"
	}
	return fmt.Sprintf("re-run it as 'sudo %s self-update', or install godai somewhere you own", e.ExePath)
}

func checkWritable(dir, exePath string) error {
	file, err := os.CreateTemp(dir, ".godai-check-*")
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
