// Package mcp contains end-to-end tests for the Go MCP server (./mcp).
//
// These tests build the godai-mcp binary, drive it over stdio (the transport
// a real MCP client uses), and exercise:
//
//   - the local tools in mcp/server/local_tools.go, and
//   - a smoke test proving a remote tool call is forwarded across to a real
//     Godot editor.
//
// The editor itself is started the way it is in production: by the
// open_godot_project tool, which spawns it via the configured --godot-path. To
// keep that headless, --godot-path points at a small wrapper script that adds
// --headless before exec'ing the real Godot binary.
//
// Environment variables:
//   - GODOT: path to the Godot binary (otherwise "godot"/"godot4" from PATH)
//   - GODAI_TEST_VERBOSE: stream the server and editor logs to stderr
//   - GODAI_TEST_KEEP: keep the temporary directory after the run
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
	client *harness.MCPClient

	// projectPath is the canonical path to the test project the server manages.
	projectPath string

	// instancesDir is where the editor advertises itself and where teardown
	// looks for editors to kill.
	instancesDir string

	// serverCmd is the running godai-mcp process.
	serverCmd *exec.Cmd

	// serverBin is the godai-mcp binary built once in testMain, reused by tests
	// that start their own server (e.g. the --global mode test).
	serverBin string

	// godotWrapperPath is the --headless wrapper script, reused as a valid
	// --godot-path by servers that don't spawn an editor.
	godotWrapperPath string

	// coverDir, when non-empty, is an absolute directory where coverage data
	// from the spawned godai-mcp subprocess(es) is written. It's enabled by
	// setting GODAI_COVERDIR; the binary is then built with -cover and each
	// server runs with GOCOVERDIR pointing here. Coverage is flushed when the
	// server exits normally (the SIGINT-driven graceful shutdown in stopServer).
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

	// Lay out the temp tree.
	rootDir := filepath.Join(base, "projects")
	projectDir := filepath.Join(rootDir, "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	projectPath = projectDir

	instancesDir = filepath.Join(base, "cache", "godai-mcp", "instances")

	// A bare project (no addon): open_godot_project installs and enables it,
	// exercising that path. The Go server knows the secret from the instance
	// file, so we don't disable the secret check.
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

	// The main server runs in non-global mode, scoped to our temp root, and
	// spawns the editor through the headless wrapper.
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

// serverInstance is a running godai-mcp server driven over stdio.
type serverInstance struct {
	cmd     *exec.Cmd
	client  *harness.MCPClient
	logPath string
}

// startServer launches the godai-mcp binary with the given extra args, isolating
// its XDG config/data/cache under xdgBase, applying extraEnv, and completing the
// MCP initialize handshake. The caller is responsible for stopping the process
// (and any editors it spawned).
func startServer(xdgBase string, args, extraEnv []string, verbose bool) (*serverInstance, error) {
	return startServerWithClient(xdgBase, args, extraEnv, verbose, harness.ClientConfig{})
}

// startServerWithClient is like startServer but drives the server with a client
// configured by cfg: the capabilities it advertises (e.g. roots, elicitation)
// and the handlers it uses to answer the server's callbacks.
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

// buildServer compiles the godai-mcp binary into dir and returns its path.
func buildServer(dir string) (string, error) {
	binPath := filepath.Join(dir, "godai-mcp")
	args := []string{"build"}
	if coverDir != "" {
		// Instrument the binary so the spawned server emits coverage for the
		// MCP packages to GOCOVERDIR.
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

// stopServer closes the server's stdin (so its read loop hits EOF) and waits
// for it to exit, escalating to a kill.
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

// killEditorInstances kills any editors still advertising themselves in the
// instances directory. The server doesn't own the editors it spawns, so the
// test harness has to clean them up.
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

// killProcess terminates a non-child process (an editor the server spawned),
// asking nicely first and escalating to SIGKILL. Since it isn't our child we
// can't Wait on it; we poll for its exit with signal 0 instead.
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
