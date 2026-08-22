// Package cli contains end-to-end tests that run the real `godai` binary
// against a real Godot editor, so the command line itself - flag names, value
// parsing, quoting - is covered and not just the tools underneath it.
package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

const projectName = "Godai CLI Functional Test"

var (
	godaiBin     string
	projectPath  string
	instancesDir string

	// When set (via GODAI_COVERDIR), the binary is built with -cover and each
	// invocation runs with GOCOVERDIR pointing here.
	coverDir string
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "SKIP: functional tests don't run in -short mode")
		return 0
	}

	godotBin, err := harness.FindGodot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: functional tests: %v\n", err)
		return 0
	}

	if dir := os.Getenv("GODAI_COVERDIR"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: resolving GODAI_COVERDIR: %v\n", err)
			return 1
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating GODAI_COVERDIR: %v\n", err)
			return 1
		}
		coverDir = abs
	}

	base, err := os.MkdirTemp("", "godai-cli-functional-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	code := 1
	var editor *exec.Cmd
	defer func() {
		harness.StopEditor(editor)
		if code != 0 || os.Getenv("GODAI_TEST_KEEP") != "" {
			fmt.Fprintf(os.Stderr, "Temp dir kept at %s\n", base)
		} else {
			os.RemoveAll(base)
		}
	}()

	projectPath = filepath.Join(base, "projects", "demo")
	if err := harness.CreateTestProject(projectPath, harness.ProjectOptions{
		Name:         projectName,
		InstallAddon: true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating test project: %v\n", err)
		return 1
	}

	godaiBin, err = buildGodai(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: building godai: %v\n", err)
		return 1
	}

	if err := setupExitCodeTests(base); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: setting up the exit code tests: %v\n", err)
		return 1
	}

	// The editor writes its instance file under the isolated dirs the harness
	// points into the project, which is where --editor-instances-path sends the
	// CLI looking. Nothing touches the user's real cache.
	instancesDir = isolation.GodaiInstancesDir(harness.IsolationDir(projectPath))

	// WebSocket, because that's the transport the CLI speaks.
	editor, logPath, err := harness.LaunchEditor(godotBin, projectPath, harness.EditorOptions{
		Transport: "websocket",
		Verbose:   os.Getenv("GODAI_TEST_VERBOSE") != "",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: launching editor: %v\n", err)
		return 1
	}

	// The first launch has to import the whole project, which can be slow.
	if err := waitForInstanceFile(harness.OpenTimeout(180 * time.Second)); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v (editor log: %s)\n", err, logPath)
		return 1
	}

	code = m.Run()
	if code != 0 {
		fmt.Fprintf(os.Stderr, "Editor log: %s\n", logPath)
	}
	return code
}

func buildGodai(dir string) (string, error) {
	binPath := filepath.Join(dir, "godai")
	args := []string{"build", "-tags", "selfupdate"}
	if coverDir != "" {
		args = append(args, "-cover", "-coverpkg=gitlab.com/snopek-games/godai/cmd/godai,gitlab.com/snopek-games/godai/internal/...")
	}
	args = append(args, "-o", binPath, "./cmd/godai")
	cmd := exec.Command("go", args...)
	cmd.Dir = harness.RepoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %v\n%s", err, out)
	}
	return binPath, nil
}

// The editor announces itself by writing an instance file, which is also how
// the CLI finds it.
func waitForInstanceFile(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(instancesDir)
		if err == nil {
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) == ".json" {
					return nil
				}
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("the editor never wrote an instance file to %s", instancesDir)
}

// Builds a godai command that runs from inside the project, the way the demo
// does once it's cd'd there, so `--project-path` can be dropped.
func command(args ...string) *exec.Cmd {
	cmd := exec.Command(godaiBin, append([]string{
		"--root", filepath.Dir(projectPath),
		"--editor-instances-path", instancesDir,
	}, args...)...)
	cmd.Dir = projectPath
	if coverDir != "" {
		cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverDir)
	}
	return cmd
}

// Runs the godai binary, failing the test if it exits non-zero, and returns
// what it printed.
func godai(t *testing.T, args ...string) string {
	t.Helper()

	out, err := command(args...).CombinedOutput()
	if err != nil {
		t.Fatalf("godai %v: %v\n%s", args, err, out)
	}
	return string(out)
}
