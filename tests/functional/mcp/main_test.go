// Package mcp contains end-to-end tests for the Go MCP server: they build the
// godai-mcp binary and drive it over stdio.
package mcp

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"godai/tests/functional/internal/harness"
)

const projectName = "Godai MCP Functional Test"

var (
	client           *harness.MCPClient
	projectPath      string
	instancesDir     string
	serverCmd        *exec.Cmd
	serverBin        string
	godotWrapperPath string

	// When set (via GODAI_COVERDIR), the binary is built with -cover and each
	// server runs with GOCOVERDIR pointing here. Coverage is flushed only on the
	// SIGINT-driven graceful shutdown in stopServer.
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
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "SKIP: mcp functional tests use a shell wrapper and don't run on Windows")
		return 0
	}

	godotBin, err := harness.FindGodot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: functional tests: %v\n", err)
		return 0
	}

	verbose := os.Getenv("GODAI_TEST_VERBOSE") != ""

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

	base, err := os.MkdirTemp("", "godai-mcp-functional-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	// Resolve symlinks once so paths line up with the server's canonicalization
	// (it calls filepath.EvalSymlinks on everything).
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	code := 1
	defer func() {
		killEditorInstances(instancesDir)
		stopServer(serverCmd)
		if code != 0 {
			fmt.Fprintf(os.Stderr, "Temp dir kept at %s\n", base)
		} else if os.Getenv("GODAI_TEST_KEEP") != "" {
			fmt.Fprintf(os.Stderr, "Temp dir kept at %s\n", base)
		} else {
			os.RemoveAll(base)
		}
	}()

	rootDir := filepath.Join(base, "projects")
	projectDir := filepath.Join(rootDir, "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	projectPath = projectDir

	instancesDir = filepath.Join(base, "cache", "godai-mcp", "instances")

	if err := harness.CreateTestProject(projectDir, harness.ProjectOptions{Name: projectName}); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating test project: %v\n", err)
		return 1
	}

	// A wrapper that forces --headless, so the editor open_godot_project spawns
	// doesn't try to open a window.
	godotWrapper := filepath.Join(base, "godot-headless")
	wrapper := fmt.Sprintf("#!/bin/sh\nexec %q --headless \"$@\"\n", godotBin)
	if err := os.WriteFile(godotWrapper, []byte(wrapper), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: writing godot wrapper: %v\n", err)
		return 1
	}
	godotWrapperPath = godotWrapper

	serverBin, err = buildServer(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: building godai-mcp: %v\n", err)
		return 1
	}

	port, err := harness.FindFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}

	inst, err := startServer(base, []string{
		"--root", rootDir,
		"--godot-path", godotWrapper,
		"--editor-instances-path", instancesDir,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
	}, []string{
		"GODAI_MCP_TRANSPORT=websocket",
		fmt.Sprintf("GODAI_MCP_BASE_PORT=%d", port),
		"GODAI_MCP_PORT_COUNT=1",
		"GODAI_DISABLE_RESTART=1",
	}, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: starting godai-mcp: %v\n", err)
		return 1
	}
	serverCmd = inst.cmd
	client = inst.client

	code = m.Run()
	if code != 0 {
		fmt.Fprintf(os.Stderr, "Server log: %s\n", inst.logPath)
	}
	return code
}

type serverInstance struct {
	cmd     *exec.Cmd
	client  *harness.MCPClient
	logPath string
}

func startServer(xdgBase string, args, extraEnv []string, verbose bool) (*serverInstance, error) {
	return startServerWithClient(xdgBase, args, extraEnv, verbose, harness.ClientConfig{})
}

func startServerWithClient(xdgBase string, args, extraEnv []string, verbose bool, cfg harness.ClientConfig) (*serverInstance, error) {
	cmd := exec.Command(serverBin, args...)
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(xdgBase, "config"),
		"XDG_DATA_HOME="+filepath.Join(xdgBase, "data"),
		"XDG_CACHE_HOME="+filepath.Join(xdgBase, "cache"),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	if coverDir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	logPath := filepath.Join(xdgBase, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	if verbose {
		cmd.Stderr = io.MultiWriter(logFile, os.Stderr)
	} else {
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := harness.NewStdioClientWithConfig(stdin, stdout, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("%w (server log: %s)", err, logPath)
	}

	return &serverInstance{cmd: cmd, client: c, logPath: logPath}, nil
}

func buildServer(dir string) (string, error) {
	binPath := filepath.Join(dir, "godai-mcp")
	args := []string{"build"}
	if coverDir != "" {
		args = append(args, "-cover", "-coverpkg=godai/mcp/...")
	}
	args = append(args, "-o", binPath, "./mcp")
	cmd := exec.Command("go", args...)
	cmd.Dir = harness.RepoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %v\n%s", err, out)
	}
	return binPath, nil
}

func stopServer(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Signal(os.Interrupt)

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

// The server doesn't own the editors it spawns, so the harness cleans up any
// still advertising themselves in the instances directory.
func killEditorInstances(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var inst struct {
			PID int `json:"pid"`
		}
		if err := json.Unmarshal(b, &inst); err != nil || inst.PID <= 0 {
			continue
		}
		killProcess(inst.PID)
	}
}

// killProcess terminates an editor the server spawned. It isn't our child, so
// we can't Wait on it; we poll for its exit with signal 0 instead.
func killProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Signal(os.Interrupt)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return // already gone
		}
		time.Sleep(200 * time.Millisecond)
	}
	proc.Kill()
}
