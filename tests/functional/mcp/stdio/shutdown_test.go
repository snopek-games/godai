package stdio

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func waitForProcessExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !harness.Alive(pid) {
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

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	project := filepath.Join(base, "anywhere", "headless_proj")
	mustCreateProject(t, project, "Headless Project")

	// The server spawns the editor inheriting its own isolated cache dir, so
	// that's where it advertises itself.
	instances := isolation.GodaiInstancesDir(base)

	// GODAI_DISABLE_CLOSE is deliberately omitted: we want the editor to really
	// save and quit when the server closes it on shutdown.
	inst, err := startServer(base, []string{
		"--global",
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
	}, append([]string{
		"GODAI_MCP_TRANSPORT=websocket",
		// Without this the editor denies close_editor, since it's headless and
		// nobody is there to approve it.
		"GODAI_AUTO_APPROVE_TOOLS=1",
	}, harness.MCPPortEnv()...), os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() {
		killEditorInstances(instances)
		stopServer(inst)
	})
	dumpLogOnFailure(t, "server log", inst.logPath)
	dumpLogOnFailure(t, "editor log", filepath.Join(isolation.GodaiCacheDir(base), "editor-logs", "*.log"))

	out := callToolOKWith(t, inst.client, "open_godot_project", map[string]any{
		"project_path": project,
		"headless":     true,
	})
	is.Equal(out["success"], true)

	waitForOpenProject(t, inst.client, project, harness.OpenTimeout(180*time.Second))

	logs, _ := filepath.Glob(filepath.Join(isolation.GodaiCacheDir(base), "editor-logs", "*.log"))
	is.True(len(logs) > 0) // GODAI_EDITOR_LOG made the server capture the editor's output

	pids := getInstancePIDs(t, instances)
	is.True(len(pids) > 0) // the headless editor advertised itself

	// Stopping the server runs closeHeadlessEditors, which asks the editor to
	// save and quit.
	stopServer(inst)

	for pid := range pids {
		waitForProcessExit(t, pid, 30*time.Second)
	}
}
