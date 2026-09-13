// Package offscreen contains end-to-end tests for tools that need the editor
// to actually render, which a headless editor never does. The editor draws to
// an Xvfb virtual display, so the suite is skipped where that isn't available.
package offscreen

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

const (
	projectName    = "Godai Offscreen Functional Test"
	displayWidth   = 1024
	displayHeight  = 768
	offscreenSize  = "1024x768"
	startupTimeout = 180 * time.Second
)

var client *harness.MCPClient

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "SKIP: functional tests don't run in -short mode")
		return 0
	}
	if err := harness.CheckOffscreen(); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: offscreen functional tests: %v\n", err)
		return 0
	}

	godotBin, err := harness.FindGodot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: functional tests: %v\n", err)
		return 0
	}

	// A software-rendered editor takes tons of resources, so run alone.
	release, err := harness.LockMachine(true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	defer release()

	projectDir, err := os.MkdirTemp("", "godai-offscreen-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}

	if err := harness.CreateTestProject(projectDir, harness.ProjectOptions{
		Name:            projectName,
		InstallAddon:    true,
		SkipSecretCheck: true,
		RenderingMethod: harness.OffscreenRenderingMethod,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating test project: %v\n", err)
		return 1
	}

	cmd, logPath, err := harness.LaunchEditor(godotBin, projectDir, harness.EditorOptions{
		Transport:     "http",
		Offscreen:     true,
		OffscreenSize: offscreenSize,
		Verbose:       os.Getenv("GODAI_TEST_VERBOSE") != "",
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

	instancesDir := isolation.GodaiInstancesDir(harness.IsolationDir(projectDir))
	port, err := harness.WaitForInstancePort(instancesDir, harness.OpenTimeout(startupTimeout))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	client = harness.NewHTTPClient("http://127.0.0.1:" + strconv.Itoa(port))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := waitForEditor(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: editor never became ready: %v\n", err)
		return 1
	}

	code = m.Run()
	return code
}

func waitForEditor(ctx context.Context) error {
	var lastErr error
	for ctx.Err() == nil {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, lastErr = client.Initialize(reqCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%w: %v", ctx.Err(), lastErr)
}
