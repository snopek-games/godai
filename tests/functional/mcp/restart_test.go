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

// launchConnectableEditor starts a headless editor a scanning server will
// connect to. Restart is NOT disabled, so restart_editor genuinely restarts it.
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

func TestRestartEditor(t *testing.T) {
	is := is.New(t)

	godotBin, err := harness.FindGodot()
	is.NoErr(err)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

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

	// The server spawns nothing here (the editor advertises into editorXDG, which
	// the server scans), so it needs no editor/MCP env.
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

	_, errBad := inst.client.CallTool(testContext(t), "restart_editor", map[string]any{"project_path": "/definitely/not/a/real/path"})
	is.True(errBad != nil) // invalid project_path
	notConnected, err := inst.client.CallTool(testContext(t), "restart_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(notConnected.IsError) // no editor connected for the project yet

	// The first launch imports the project, which can be slow.
	editor := launchConnectableEditor(t, godotBin, projectDir, editorXDG)
	t.Cleanup(func() { harness.StopEditor(editor) })
	waitForOpenProject(t, inst.client, projectPath, 180*time.Second)

	pidsBefore := getInstancePIDs(t, instancesDir)
	is.True(len(pidsBefore) > 0) // the editor advertised itself

	// The call returns only once the editor has relaunched and reconnected.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	res, err := inst.client.CallTool(ctx, "restart_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(!res.IsError) // restart_editor succeeded

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
