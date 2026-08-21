package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"

	"github.com/matryer/is"
)

// stubEngineName is a linked engine standing in for one `godai engine
// install` would have downloaded: a script that exits with --exit=N.
const stubEngineName = "stub-build"

var (
	exitBase       string
	stubConfigHome string

	// Resolving an engine falls back to any godot on PATH, so the exit code
	// tests run with one that can't have any, keeping them hermetic.
	emptyPath string
)

func setupExitCodeTests(base string) error {
	exitBase = base

	stub := filepath.Join(base, "godot-stub")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n\tcase \"$arg\" in\n\t--exit=*) exit \"${arg#--exit=}\" ;;\n\tesac\ndone\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		return err
	}

	stubConfigHome = filepath.Join(base, "stub-config")
	configDir := filepath.Join(stubConfigHome, "godai")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	engines, err := json.Marshal(map[string]any{
		"linked": map[string]any{
			stubEngineName: map[string]string{"path": stub},
		},
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(configDir, "engines.json"), engines, 0o644); err != nil {
		return err
	}

	emptyPath = filepath.Join(base, "empty-path")
	return os.MkdirAll(emptyPath, 0o755)
}

// Runs the godai binary bare - no editor, no project - and returns its exit
// code and combined output.
func godaiExitCode(t *testing.T, args ...string) (int, string) {
	t.Helper()
	return godaiExitCodeWithConfigHome(t, stubConfigHome, args...)
}

func godaiExitCodeWithConfigHome(t *testing.T, configHome string, args ...string) (int, string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the stub engine is a shell script")
	}

	cmd := exec.Command(godaiBin, args...)
	cmd.Dir = exitBase
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+filepath.Join(exitBase, "stub-data"),
		"XDG_CACHE_HOME="+filepath.Join(exitBase, "stub-cache"),
		"GODAI_NO_UPDATE_CHECK=1",
		"PATH="+emptyPath,
	)
	if coverDir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
	}

	return exitCodeOf(t, cmd)
}

func exitCodeOf(t *testing.T, cmd *exec.Cmd) (int, string) {
	t.Helper()

	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("godai %v: %v\n%s", cmd.Args[1:], err, out)
		}
		return exitErr.ExitCode(), string(out)
	}
	return 0, string(out)
}

func TestExitCodeSuccess(t *testing.T) {
	is := is.New(t)

	code, _ := godaiExitCode(t, "help")
	is.Equal(code, 0)
}

func TestExitCodeUsageError(t *testing.T) {
	is := is.New(t)

	code, _ := godaiExitCode(t, "engine", "link", "only-a-name")
	is.Equal(code, 2)
}

func TestExitCodeUnknownHelpTopic(t *testing.T) {
	is := is.New(t)

	code, _ := godaiExitCode(t, "help", "no-such-topic")
	is.Equal(code, 2)
}

// With a config home that has no engines linked or installed, there's nothing
// for `engine which` to resolve.
func TestExitCodeNotConfigured(t *testing.T) {
	is := is.New(t)

	code, _ := godaiExitCodeWithConfigHome(t, t.TempDir(), "engine", "which")
	is.Equal(code, 3)
}

func TestEngineRunPropagatesSubprocessExitCode(t *testing.T) {
	is := is.New(t)

	code, out := godaiExitCode(t, "engine", "run", stubEngineName, "--", "--exit=7")
	is.Equal(code, 7)
	is.True(!strings.Contains(out, "godai:")) // godai adds no error output of its own
}

// A subprocess exiting with 3 used to be mistaken for urfave/cli's
// unknown-help-topic error and turned into exit code 2.
func TestEngineRunPropagatesExitCodeThree(t *testing.T) {
	is := is.New(t)

	code, _ := godaiExitCode(t, "engine", "run", stubEngineName, "--", "--exit=3")
	is.Equal(code, 3)
}

func TestEngineRunSuccess(t *testing.T) {
	is := is.New(t)

	code, _ := godaiExitCode(t, "engine", "run", stubEngineName, "--", "--exit=0")
	is.Equal(code, 0)
}

// A valid project no editor has open, so waiting for one can only time out.
func TestExitCodeNoEditor(t *testing.T) {
	is := is.New(t)

	other := filepath.Join(filepath.Dir(projectPath), "no-editor")
	is.NoErr(harness.CreateTestProject(other, harness.ProjectOptions{Name: "No Editor"}))

	code, _ := exitCodeOf(t, command("editor-tool", "read_script", "res://x.gd",
		"--project-path", other, "--connect-timeout", "0.1"))
	is.Equal(code, 4)
}

func TestExitCodeTimeout(t *testing.T) {
	is := is.New(t)

	code, _ := exitCodeOf(t, command("editor-tool", "read_script", "whatever.gd",
		"--editor-tool-timeout", "0.00001"))
	is.Equal(code, 5)
}

func TestExitCodeToolFailed(t *testing.T) {
	is := is.New(t)

	code, _ := exitCodeOf(t, command("editor-tool", "read_script", "does_not_exist.gd"))
	is.Equal(code, 6)
}
