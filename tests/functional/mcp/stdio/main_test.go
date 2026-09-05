// Package stdio contains end-to-end tests for the CLI's MCP server: they build
// the godai binary and drive `godai mcp` over stdio.
package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/internal/fakebin"
	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

const projectName = "Godai MCP Functional Test"

// testEngineName is the linked engine the server is given, standing in for one
// `godai engine install` would have downloaded.
const testEngineName = "test-build"

var (
	client           *harness.MCPClient
	projectPath      string
	instancesDir     string
	serverInst       *serverInstance
	serverBin        string
	godotWrapperPath string
	enginesPath      string

	// When set (via GODAI_COVERDIR), the binary is built with -cover and each
	// server runs with GOCOVERDIR pointing here. Coverage is flushed only on the
	// graceful shutdown in stopServer.
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

	release, err := harness.LockMachine(false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return 1
	}
	defer release()

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

	base, err := os.MkdirTemp("", "godai-functional-*")
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
		stopServer(serverInst)
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

	instancesDir = isolation.GodaiInstancesDir(base)

	if err := harness.CreateTestProject(projectDir, harness.ProjectOptions{Name: projectName}); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating test project: %v\n", err)
		return 1
	}

	// A wrapper that forces --headless, so the editor open_godot_project spawns
	// doesn't try to open a window.
	godotWrapper, err := fakebin.Write(filepath.Join(base, "godot-headless"),
		fmt.Sprintf("exec %q --headless \"$@\"\n", godotBin),
		// Not %q: cmd.exe takes a quoted path literally, backslashes and all.
		fmt.Sprintf("\"%s\" --headless %%*\r\n", godotBin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: writing godot wrapper: %v\n", err)
		return 1
	}
	godotWrapperPath = godotWrapper

	if err := linkTestEngine(base, godotWrapper); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: linking the test engine: %v\n", err)
		return 1
	}

	serverBin, err = buildServer(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: building godai: %v\n", err)
		return 1
	}

	inst, err := startServer(base, []string{
		"--root", rootDir,
		"--godot-path", godotWrapper,
		"--editor-instances-path", instancesDir,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
		"--toolsets", "default,engine",
	}, append([]string{
		"GODAI_MCP_TRANSPORT=websocket",
		"GODAI_DISABLE_CLOSE=1",
		"GODAI_AUTO_APPROVE_TOOLS=1",
	}, harness.MCPPortEnv()...), verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: starting godai: %v\n", err)
		return 1
	}
	serverInst = inst
	client = inst.client

	code = m.Run()
	if code != 0 {
		fmt.Fprintf(os.Stderr, "Server log: %s\n", inst.logPath)
	}
	return code
}

func linkTestEngine(base, executable string) error {
	configDir := isolation.GodaiConfigDir(base)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	enginesPath = filepath.Join(configDir, "engines.json")

	return writeLinkedEngines(map[string]any{
		testEngineName: map[string]string{"path": executable},
	})
}

func writeLinkedEngines(linked map[string]any) error {
	engines, err := json.Marshal(map[string]any{"linked": linked})
	if err != nil {
		return err
	}

	return os.WriteFile(enginesPath, engines, 0o644)
}

type serverInstance struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	client  *harness.MCPClient
	logPath string
}

// Logs under a t.TempDir are deleted with it before the failure is reported, so a failing test keeps a tail of them in its output instead.
func dumpLogOnFailure(t *testing.T, label, pattern string) {
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			blob, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			const maxTail = 4096
			if len(blob) > maxTail {
				blob = blob[len(blob)-maxTail:]
				if i := bytes.IndexByte(blob, '\n'); i >= 0 {
					blob = blob[i+1:]
				}
			}
			t.Logf("%s tail (%s):\n%s", label, path, blob)
		}
	})
}

func startServer(base string, args, extraEnv []string, verbose bool) (*serverInstance, error) {
	return startServerWithClient(base, args, extraEnv, verbose, harness.ClientConfig{})
}

func startServerWithClient(base string, args, extraEnv []string, verbose bool, cfg harness.ClientConfig) (*serverInstance, error) {
	isolationEnv, err := isolation.Env(base)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(serverBin, append([]string{"mcp", "--no-update-check"}, args...)...)
	cmd.Env = append(os.Environ(), isolationEnv...)
	// Editors the server spawns log to the isolated godai cache dir under
	// editor-logs, so a connection timeout shows what Godot printed instead of
	// nothing.
	cmd.Env = append(cmd.Env, "GODAI_EDITOR_LOG=1")
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

	logPath := filepath.Join(base, "server.log")
	logOut, releaseLog, err := harness.CaptureOutput(logPath, verbose)
	if err != nil {
		return nil, err
	}
	cmd.Stderr = logOut

	err = cmd.Start()
	releaseLog()
	if err != nil {
		return nil, err
	}

	c := harness.NewStdioClientWithConfig(stdin, stdout, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("%w (server log: %s)", err, logPath)
	}

	return &serverInstance{cmd: cmd, stdin: stdin, client: c, logPath: logPath}, nil
}

func buildServer(dir string) (string, error) {
	binPath := harness.ExePath(dir, "godai")
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

// stopServer sends SIGINT where there is one, and otherwise closes stdin,
// which ends the same read loop that triggers closeHeadlessEditors.
func stopServer(inst *serverInstance) {
	if inst == nil || inst.cmd == nil || inst.cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		inst.stdin.Close()
	} else {
		inst.cmd.Process.Signal(os.Interrupt)
	}

	done := make(chan struct{})
	go func() {
		inst.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		inst.cmd.Process.Kill()
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
// we can't Wait on it; we poll for its exit instead.
func killProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	harness.AskToStop(proc)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !harness.Alive(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	proc.Kill()
}
