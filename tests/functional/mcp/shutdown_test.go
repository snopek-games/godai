package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func waitForProcessExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("editor process %d did not exit after the server shut down", pid)
}

// The server shuts down the headless editors it launched when it exits. We open
// one through open_godot_project (so the server tracks it as headless), then
// stop the server and confirm the editor process actually quit.
func TestCloseHeadlessEditorsOnShutdown(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	project := filepath.Join(xdgBase, "anywhere", "headless_proj")
	mustCreateProject(t, project, "Headless Project")

	// The server spawns the editor inheriting its own XDG_CACHE_HOME, so it
	// advertises into the server's cache dir.
	instances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	port, err := harness.FindFreePort()
	is.NoErr(err)

	// GODAI_DISABLE_CLOSE is deliberately omitted: we want the editor to really
	// save and quit when the server closes it on shutdown.
	inst, err := startServer(xdgBase, []string{
		"--global",
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
	}, []string{
		"GODAI_MCP_TRANSPORT=websocket",
		fmt.Sprintf("GODAI_MCP_BASE_PORT=%d", port),
		"GODAI_MCP_PORT_COUNT=1",
	}, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() {
		killEditorInstances(instances)
		stopServer(inst.cmd)
	})

	out := callToolOKWith(t, inst.client, "open_godot_project", map[string]any{
		"project_path": project,
		"headless":     true,
	})
	is.Equal(out["success"], true)

	waitForOpenProject(t, inst.client, project, 180*time.Second)

	pids := getInstancePIDs(t, instances)
	is.True(len(pids) > 0) // the headless editor advertised itself

	// Stopping the server (SIGINT) runs closeHeadlessEditors, which asks the
	// editor to save and quit.
	stopServer(inst.cmd)

	for pid := range pids {
		waitForProcessExit(t, pid, 30*time.Second)
	}
}
