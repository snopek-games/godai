package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/fakebin"
)

// Godai stores canonical paths, but t.TempDir() hands back one that still has
// symlinks in it (/var on macOS) or 8.3 short names (Windows), so tests that
// compare a stored path against a temp dir have to canonicalize it first.
func canonicalTempDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp dir: %v", err)
	}
	return dir
}

func TestValidateDirectory(t *testing.T) {
	is := is.New(t)
	dir := t.TempDir()

	is.NoErr(ValidateDirectory(dir)) // a real directory is valid

	// A regular file is not a directory.
	file := filepath.Join(dir, "file.txt")
	is.NoErr(os.WriteFile(file, []byte("x"), 0o644))
	is.True(ValidateDirectory(file) != nil)

	// A missing path is an error.
	is.True(ValidateDirectory(filepath.Join(dir, "missing")) != nil)
}

func TestResolveGodotExecutable(t *testing.T) {
	is := is.New(t)
	dir := t.TempDir()

	// An empty path is an error.
	_, err := ResolveGodotExecutable("")
	is.True(err != nil)

	// A missing file is an error.
	_, err = ResolveGodotExecutable(filepath.Join(dir, "missing"))
	is.True(err != nil)

	// A directory is not a regular file.
	_, err = ResolveGodotExecutable(dir)
	is.True(err != nil)

	// A regular file that fails `--version` (not an executable) is an error.
	notExec := filepath.Join(dir, "not-godot.txt")
	is.NoErr(os.WriteFile(notExec, []byte("x"), 0o644))
	_, err = ResolveGodotExecutable(notExec)
	is.True(err != nil)

	// A regular executable that exits 0 on `--version` resolves to itself.
	godot, err := fakebin.Exit(filepath.Join(dir, "fake-godot"), 0)
	is.NoErr(err)
	canonical, err := CanonicalPath(godot)
	is.NoErr(err)
	resolved, err := ResolveGodotExecutable(godot)
	is.NoErr(err)
	is.Equal(resolved, canonical)

	// An executable that runs but fails `--version` says so, rather than
	// passing on the "exit status 1" from os/exec.
	failing, err := fakebin.Exit(filepath.Join(dir, "failing-godot"), 1)
	is.NoErr(err)
	_, err = ResolveGodotExecutable(failing)
	is.True(err != nil)
	is.True(!strings.Contains(err.Error(), "exit status"))

	// A bare command name is looked up on PATH.
	t.Setenv("PATH", dir)
	resolved, err = ResolveGodotExecutable("fake-godot")
	is.NoErr(err)
	is.Equal(resolved, canonical)

	// A name that isn't on PATH is an error, not a relative path.
	_, err = ResolveGodotExecutable("definitely-not-godot")
	is.True(err != nil)

	// A relative path is resolved against the working directory, not PATH.
	t.Chdir(dir)
	resolved, err = ResolveGodotExecutable("./" + filepath.Base(godot))
	is.NoErr(err)
	is.Equal(resolved, canonical)
}

func TestCanonicalPath(t *testing.T) {
	is := is.New(t)

	// An empty path is an error.
	_, err := CanonicalPath("")
	is.True(err != nil)

	// A real directory resolves to its symlink-free absolute form.
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	is.NoErr(err)
	got, err := CanonicalPath(dir)
	is.NoErr(err)
	is.Equal(got, resolved)

	// A relative path is made absolute.
	got, err = CanonicalPath(".")
	is.NoErr(err)
	is.True(filepath.IsAbs(got))

	// "~/" expands to the home directory.
	home, err := os.UserHomeDir()
	is.NoErr(err)
	resolvedHome, err := filepath.EvalSymlinks(home)
	is.NoErr(err)
	got, err = CanonicalPath("~/")
	is.NoErr(err)
	is.Equal(got, resolvedHome)
}
