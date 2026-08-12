package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func TestBareCommandShowsHelp(t *testing.T) {
	is := is.New(t)

	is.Equal(Root().DefaultCommand, "")
	is.True(Root().Command("mcp") != nil)

	is.NoErr(runQuietly(t, []string{"godai"}))
	is.NoErr(runQuietly(t, []string{"godai", "config"}))
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"godai", "bogus"},
		{"godai", "project", "bogus"},
		{"godai", "editor-tool", "bogus"},
		{"godai", "help", "bogus"},
	} {
		err := runQuietly(t, args)
		if code := ExitCodeFor(err); code != ExitUsage {
			t.Errorf("%v: got exit code %d, want %d (%v)", args, code, ExitUsage, err)
		}
	}
}

func TestStaleSavedGodotVersionDoesNotBlockCommands(t *testing.T) {
	is := is.New(t)

	if runtime.GOOS == "windows" {
		t.Skip("XDG config path logic is unix-specific")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configPath := filepath.Join(dir, "godai", "config.json")
	is.NoErr(os.MkdirAll(filepath.Dir(configPath), 0o755))
	saved, err := json.Marshal(core.SavedConfig{GodotVersion: "4.5-stable"})
	is.NoErr(err)
	is.NoErr(os.WriteFile(configPath, saved, 0o644))

	is.NoErr(runQuietly(t, []string{"godai", "config"}))

	err = runQuietly(t, []string{"godai", "--godot-path", filepath.Join(dir, "gone", "godot"), "config"})
	is.Equal(ExitCodeFor(err), ExitUsage)
}

func TestUnusableGodotEnvVarDoesNotBlockCommands(t *testing.T) {
	is := is.New(t)

	t.Setenv("GODOT", t.TempDir())

	_, err := runCLIKeepingEnv(t, []string{"godai", "config"})
	is.NoErr(err)
}

func TestUnexpectedArgumentsAreAUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"godai", "editor-tool", "get_current_scene", "/some/project"},
		{"godai", "editor", "list", "/some/project"},
		{"godai", "project", "list", "/some/project"},
		{"godai", "project", "open", "/one", "/two"},
		{"godai", "editor", "close", "/one", "/two"},
	} {
		err := runQuietly(t, args)
		if code := ExitCodeFor(err); code != ExitUsage {
			t.Errorf("%v: got exit code %d, want %d (%v)", args, code, ExitUsage, err)
		}
	}
}

func runQuietly(t *testing.T, args []string) error {
	t.Helper()

	_, err := runCLI(t, args)
	return err
}

// The flags read GODOT and GODAI_PROJECT_PATH, so leaving them set would let
// the developer's own environment decide what these tests see.
func clearEnvSources(t *testing.T) {
	t.Helper()

	for _, name := range []string{"GODOT", "GODAI_PROJECT_PATH"} {
		if value, ok := os.LookupEnv(name); ok {
			os.Unsetenv(name)
			t.Cleanup(func() { os.Setenv(name, value) })
		}
	}
}

func runCLI(t *testing.T, args []string) (string, error) {
	t.Helper()

	clearEnvSources(t)

	return runCLIKeepingEnv(t, args)
}

// The printer writes to os.Stdout directly, so that's what has to be captured.
func runCLIKeepingEnv(t *testing.T, args []string) (string, error) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	stdout := os.Stdout
	os.Stdout = writer

	root := Root()
	root.Writer = writer
	root.ErrWriter = io.Discard

	runErr := root.Run(context.Background(), args)

	os.Stdout = stdout
	writer.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	return string(out), runErr
}
