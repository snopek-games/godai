// Package addon contains end-to-end tests that connect to a real Godot editor
// over the MCP HTTP transport and exercise the addon's MCP server, including
// the tools in addons/godai/tools/default/*_tools.gd.
//
// By default, a temporary Godot project is created (with the addon copied
// into it) and a headless editor is launched against it. Alternatively, set
// GODAI_TEST_PORT to connect to an already-running editor.
//
// Environment variables:
//   - GODOT: path to the Godot binary (otherwise "godot" or "godot4" from PATH)
//   - GODAI_TEST_PORT: connect to an editor already listening on this port,
//     instead of launching a headless one
//   - GODAI_TEST_VERBOSE: stream the editor log to stderr
//   - GODAI_TEST_KEEP: keep the temporary project directory after the run
package addon

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"godai/tests/functional/internal/harness"
)

var (
	client *harness.MCPClient

	// projectDir is the Godot project the editor is running. It is empty when
	// connecting to an external editor via GODAI_TEST_PORT, in which case
	// tests that need to inspect the project directory are skipped.
	projectDir string
)

const projectName = "Godai Functional Test"

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "SKIP: functional tests don't run in -short mode")
		return 0
	}

	ctx := context.Background()

	if portStr := os.Getenv("GODAI_TEST_PORT"); portStr != "" {
		client = harness.NewHTTPClient("http://127.0.0.1:" + portStr)
		if err := waitForEditor(ctx, 10*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: cannot connect to editor on port %s: %v\n", portStr, err)
			return 1
		}
		return m.Run()
	}

	godotBin, err := harness.FindGodot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: functional tests: %v\n", err)
		return 0
	}

	dir, err := os.MkdirTemp("", "godai-functional-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	projectDir = dir

	if err := harness.CreateTestProject(projectDir, harness.ProjectOptions{
		Name:            projectName,
		InstallAddon:    true,
		SkipSecretCheck: true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating test project: %v\n", err)
		return 1
	}

	port, err := harness.FindFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	client = harness.NewHTTPClient("http://127.0.0.1:" + strconv.Itoa(port))

	cmd, logPath, err := harness.LaunchEditor(godotBin, projectDir, harness.EditorOptions{
		Port:      port,
		Transport: "http",
		Verbose:   os.Getenv("GODAI_TEST_VERBOSE") != "",
		// Let the restart_editor tool run end-to-end without actually
		// restarting (which would kill the editor this harness manages).
		DisableRestart: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: launching editor: %v\n", err)
		return 1
	}

	code := 1
	defer func() {
		harness.StopEditor(cmd)
		if code != 0 {
			fmt.Fprintf(os.Stderr, "Test project kept at %s (editor log: %s)\n", projectDir, logPath)
		} else if os.Getenv("GODAI_TEST_KEEP") != "" {
			fmt.Fprintf(os.Stderr, "Test project kept at %s\n", projectDir)
		} else {
			os.RemoveAll(projectDir)
		}
	}()

	// The first launch has to import the whole project, which can be slow.
	if err := waitForEditor(ctx, 180*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: editor never became ready: %v\n", err)
		return 1
	}

	code = m.Run()
	return code
}

// waitForEditor polls the MCP endpoint until an initialize request succeeds.
func waitForEditor(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	var lastErr error
	for time.Now().Before(deadline) {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, lastErr = client.Initialize(reqCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timed out after %s: %w", timeout, lastErr)
}
