package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"

	"godai/tests/functional/internal/harness"
)

func editorInstancesDir(xdgBase string) string {
	return filepath.Join(xdgBase, ".xdg", "XDG_CACHE_HOME", "godai-mcp", "instances")
}

// launchConnectableEditor starts a headless, websocket-mode editor that a server
// scanning editorInstancesDir(xdgBase) will discover and connect to. Restart is
// NOT disabled, so restart_editor genuinely restarts the editor.
func launchConnectableEditor(t *testing.T, godotBin, projectDir, xdgBase string) *exec.Cmd {
	t.Helper()
	port, err := harness.FindFreePort()
	if err != nil {
		t.Fatal(err)
	}
	cmd, logPath, err := harness.LaunchEditor(godotBin, projectDir, harness.EditorOptions{
		Port:      port,
		Transport: "websocket",
		XDGBase:   xdgBase,
		Verbose:   os.Getenv("GODAI_TEST_VERBOSE") != "",
	})
	if err != nil {
		t.Fatalf("launching editor: %v", err)
	}
	t.Logf("launched editor (log: %s)", logPath)
	return cmd
}

// waitForOpenProject polls list_open_projects until the given project shows up
// (i.e. the server has connected to its editor).
func waitForOpenProject(t *testing.T, c *harness.MCPClient, projectPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := listOpenProjects(t, c)[projectPath]; ok {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("editor for %s never connected to the server", projectPath)
}

// getInstancePIDs returns the set of editor PIDs advertised in the instances dir.
func getInstancePIDs(t *testing.T, dir string) map[int]bool {
	t.Helper()
	pids := map[int]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var inst struct {
			PID int `json:"pid"`
		}
		if json.Unmarshal(b, &inst) == nil && inst.PID > 0 {
			pids[inst.PID] = true
		}
	}
	return pids
}

// TestRestartEditor exercises restart_editor end-to-end against a real editor
// that actually restarts itself: the server forwards the call to the editor,
// the editor relaunches (staying headless), and the server waits for the fresh
// connection before reporting success. We confirm a genuine restart by checking
// that a new editor process (PID) replaces the original.
func TestRestartEditor(t *testing.T) {
	is := is.New(t)

	godotBin, err := harness.FindGodot()
	is.NoErr(err)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	// A project (with the addon installed) under a root the server allows.
	rootDir := filepath.Join(xdgBase, "projects")
	projectDir := filepath.Join(rootDir, "restart_proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	is.NoErr(harness.CreateTestProject(projectDir, harness.ProjectOptions{
		Name:         "Restart Project",
		InstallAddon: true,
	}))
	projectPath, err := filepath.EvalSymlinks(projectDir)
	is.NoErr(err)

	// The editor advertises into editorXDG; the server scans the same dir. The
	// server itself spawns nothing here, so it needs no editor/MCP env.
	editorXDG := filepath.Join(xdgBase, "editor")
	instancesDir := editorInstancesDir(editorXDG)
	inst, err := startServer(xdgBase, []string{
		"--root", rootDir,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instancesDir,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })
	// When the editor restarts itself the relaunched process is orphaned (not the
	// harness's child), so clean it up by PID from the instances dir.
	t.Cleanup(func() { killEditorInstances(instancesDir) })

	// Error paths, before any editor is connected: an unknown path fails up front
	// (a plain error from canonicalPath, surfaced as a JSON-RPC error)...
	_, errBad := inst.client.CallTool(testContext(t), "restart_editor", map[string]any{"project_path": "/definitely/not/a/real/path"})
	is.True(errBad != nil) // invalid project_path
	// ...and a valid project with no editor reports (as a tool result) that it
	// isn't connected.
	notConnected, err := inst.client.CallTool(testContext(t), "restart_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(notConnected.IsError) // no editor connected for the project yet

	// Launch the editor and wait for the server to connect to it. The first
	// launch imports the project, which can be slow.
	editor := launchConnectableEditor(t, godotBin, projectDir, editorXDG)
	t.Cleanup(func() { harness.StopEditor(editor) })
	waitForOpenProject(t, inst.client, projectPath, 180*time.Second)

	pidsBefore := getInstancePIDs(t, instancesDir)
	is.True(len(pidsBefore) > 0) // the editor advertised itself

	// Restart it. The call returns only once the editor has relaunched and
	// reconnected, so give it a generous timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	res, err := inst.client.CallTool(ctx, "restart_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(!res.IsError) // restart_editor succeeded

	// A new editor process now serves the project: confirm the PID changed (a
	// genuine restart) and the project is connected again.
	pidsAfter := getInstancePIDs(t, instancesDir)
	newPID := false
	for pid := range pidsAfter {
		if !pidsBefore[pid] {
			newPID = true
		}
	}
	is.True(newPID) // the editor was actually relaunched as a new process

	projects := listOpenProjects(t, inst.client)
	is.Equal(projects[projectPath], "Restart Project")
}
