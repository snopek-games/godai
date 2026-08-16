package selfupdate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestUpdateWithoutWritePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions don't work this way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can write to a read-only directory")
	}
	is := is.New(t)

	fake := newFakeGitLab(t, []fakeRelease{{tag: "v0.4.0"}})
	updater := newTestUpdater(t, fake, "0.3.2")
	exePath := installTestExecutable(t, binaryContent("v0.3.2"))

	release, err := updater.DetectLatest(context.Background())
	is.NoErr(err)

	installDir := filepath.Dir(exePath)
	is.NoErr(os.Chmod(installDir, 0o555))
	// Put it back, or the temporary directory can't be cleaned up.
	t.Cleanup(func() { os.Chmod(installDir, 0o755) })

	_, err = updater.Update(context.Background(), release, exePath)

	var permissionErr *PermissionError
	is.True(errors.As(err, &permissionErr))
	is.True(errors.Is(err, fs.ErrPermission))
	is.Equal(permissionErr.Dir, installDir)
	is.True(strings.Contains(err.Error(), "sudo"))

	is.Equal(readFile(t, exePath), binaryContent("v0.3.2"))
	is.Equal(fake.assetRequests.Load(), int64(0)) // failed before downloading anything
}

func TestRollbackWithoutWritePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions don't work this way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can write to a read-only directory")
	}
	is := is.New(t)

	exePath := installTestExecutable(t, binaryContent("v0.4.0"))
	is.NoErr(os.WriteFile(BackupPath(exePath), []byte(binaryContent("v0.3.2")), 0o755))

	installDir := filepath.Dir(exePath)
	is.NoErr(os.Chmod(installDir, 0o555))
	t.Cleanup(func() { os.Chmod(installDir, 0o755) })

	err := Rollback(exePath)

	var permissionErr *PermissionError
	is.True(errors.As(err, &permissionErr))
	is.Equal(readFile(t, exePath), binaryContent("v0.4.0"))
}

func TestPermissionErrorHint(t *testing.T) {
	is := is.New(t)

	// Re-running with sudo would "work" and then be undone by the next npm
	// install, so the npm advice wins.
	npmErr := &PermissionError{
		Dir:     "/usr/lib/node_modules/@snopek-games/godai-linux-x64/bin",
		ExePath: "/usr/lib/node_modules/@snopek-games/godai-linux-x64/bin/godai",
		Err:     fs.ErrPermission,
	}
	is.True(strings.Contains(npmErr.Hint(), "npm install"))

	plainErr := &PermissionError{Dir: "/usr/local/bin", ExePath: "/usr/local/bin/godai", Err: fs.ErrPermission}
	if runtime.GOOS == "windows" {
		is.True(strings.Contains(plainErr.Hint(), "Administrator"))
	} else {
		is.True(strings.Contains(plainErr.Hint(), "sudo /usr/local/bin/godai self-update"))
	}
}
