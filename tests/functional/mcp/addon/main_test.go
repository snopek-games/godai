// Package addon contains end-to-end tests that connect to a real Godot editor
// over the MCP HTTP transport and exercise the addon's MCP server. By default a
// temporary project is created and a headless editor launched; set
// GODAI_TEST_PORT to connect to an already-running editor instead.
package addon

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

var (
	client *harness.MCPClient

	// projectDir is empty when connecting to an external editor via GODAI_TEST_PORT.
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

	release, err := harness.LockMachine(false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	defer release()

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

	cmd, logPath, err := harness.LaunchEditor(godotBin, projectDir, harness.EditorOptions{
		Transport: "http",
		Verbose:   os.Getenv("GODAI_TEST_VERBOSE") != "",
		// Let restart_editor and close_editor run end-to-end without shutting down the editor this harness manages.
		DisableShutdown: true,
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

	// The editor advertises the port it bound once the addon is up, and the
	// first launch has to import the whole project first, which can be slow.
	instancesDir := isolation.GodaiInstancesDir(harness.IsolationDir(projectDir))
	port, err := harness.WaitForInstancePort(instancesDir, harness.OpenTimeout(180*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	client = harness.NewHTTPClient("http://127.0.0.1:" + strconv.Itoa(port))

	if err := waitForEditor(ctx, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: editor never became ready: %v\n", err)
		return 1
	}

	code = m.Run()
	return code
}

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
