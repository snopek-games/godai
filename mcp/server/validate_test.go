package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/matryer/is"
)

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

func TestValidateGodotExecutable(t *testing.T) {
	is := is.New(t)
	dir := t.TempDir()

	// A missing file is an error.
	is.True(ValidateGodotExecutable(filepath.Join(dir, "missing")) != nil)

	// A directory is not a regular file.
	is.True(ValidateGodotExecutable(dir) != nil)

	// A regular file that fails `--version` (not an executable) is an error.
	notExec := filepath.Join(dir, "not-godot.txt")
	is.NoErr(os.WriteFile(notExec, []byte("x"), 0o644))
	is.True(ValidateGodotExecutable(notExec) != nil)

	if runtime.GOOS == "windows" {
		t.Skip("the success path uses a shell-script stand-in")
	}
	// A regular executable that exits 0 on `--version` passes.
	ok := filepath.Join(dir, "fake-godot")
	is.NoErr(os.WriteFile(ok, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	is.NoErr(ValidateGodotExecutable(ok))
}

func TestCanonicalPath(t *testing.T) {
	is := is.New(t)

	// An empty path is an error.
	_, err := canonicalPath("")
	is.True(err != nil)

	// A real directory resolves to its symlink-free absolute form.
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	is.NoErr(err)
	got, err := canonicalPath(dir)
	is.NoErr(err)
	is.Equal(got, resolved)

	// A relative path is made absolute.
	got, err = canonicalPath(".")
	is.NoErr(err)
	is.True(filepath.IsAbs(got))

	// "~/" expands to the home directory.
	home, err := os.UserHomeDir()
	is.NoErr(err)
	resolvedHome, err := filepath.EvalSymlinks(home)
	is.NoErr(err)
	got, err = canonicalPath("~/")
	is.NoErr(err)
	is.Equal(got, resolvedHome)
}
