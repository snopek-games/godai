// Package isolationtest applies the isolation environment to the current
// process for testing.
package isolationtest

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/isolation"
)

// Isolate points godai's and Godot's per-user directories at a fresh temp
// directory for the duration of the test and returns it.
func Isolate(t testing.TB) string {
	t.Helper()
	base := t.TempDir()
	// Resolve symlinks (macOS puts temp dirs behind /var -> /private/var) so
	// the returned base compares equal to paths godai canonicalizes.
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	Apply(t, base)
	return base
}

// Apply sets the isolation environment for base on the current process.
func Apply(t testing.TB, base string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// TODO(windows): remove this skip once isolation.Env supports Windows.
		t.Skip("directory isolation is not implemented on windows")
	}
	env, err := isolation.Env(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
}
