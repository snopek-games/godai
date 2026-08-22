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

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func editorInstancesDir(base string) string {
	return isolation.GodaiInstancesDir(harness.IsolationDir(base))
}

// launchConnectableEditor starts a headless editor a scanning server will
// connect to. Restart is NOT disabled, so restart_editor genuinely restarts it.
func launchConnectableEditor(t *testing.T, godotBin, projectDir, base string) *exec.Cmd {
	t.Helper()
	cmd, logPath, err := harness.LaunchEditor(godotBin, projectDir, harness.EditorOptions{
		Transport:     "websocket",
		IsolationBase: base,
		Verbose:       os.Getenv("GODAI_TEST_VERBOSE") != "",
	})
	if err != nil {
		t.Fatalf("launching editor: %v", err)
	}
	t.Logf("launched editor (log: %s)", logPath)
	dumpLogOnFailure(t, "editor log", logPath)
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

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	rootDir := filepath.Join(base, "projects")
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

	// The server spawns nothing here (the editor advertises into editorBase,
	// which the server scans), so it needs no editor/MCP env.
	editorBase := filepath.Join(base, "editor")
	instancesDir := editorInstancesDir(editorBase)
	inst, err := startServer(base, []string{
		"--root", rootDir,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instancesDir,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })
	dumpLogOnFailure(t, "server log", inst.logPath)
	// When the editor restarts itself the relaunched process is orphaned (not the
	// harness's child), so clean it up by PID from the instances dir.
	t.Cleanup(func() { killEditorInstances(instancesDir) })

	_, errBad := inst.client.CallTool(testContext(t), "restart_editor", map[string]any{"project_path": "/definitely/not/a/real/path"})
	is.True(errBad != nil) // invalid project_path
	notConnected, err := inst.client.CallTool(testContext(t), "restart_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(notConnected.IsError) // no editor connected for the project yet

	// The first launch imports the project, which can be slow.
	editor := launchConnectableEditor(t, godotBin, projectDir, editorBase)
	t.Cleanup(func() { harness.StopEditor(editor) })
	waitForOpenProject(t, inst.client, projectPath, harness.OpenTimeout(180*time.Second))

	pidsBefore := getInstancePIDs(t, instancesDir)
	is.True(len(pidsBefore) > 0) // the editor advertised itself

	// The call returns only once the editor has relaunched and reconnected.
	ctx, cancel := context.WithTimeout(context.Background(), harness.OpenTimeout(150*time.Second))
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

	// The relaunch rebuilds the command line, so a windowed editor coming back
	// would mean the display-driver flags were lost.
	current := callToolOKWith(t, inst.client, "get_current_project", map[string]any{
		"project_path": projectPath,
	})
	is.Equal(current["headless"], true)

	// An unsaved scene must not hang the restart on the editor's own
	// save-confirmation dialog, which a headless editor can never answer.
	callToolOKWith(t, inst.client, "create_scene", map[string]any{
		"project_path":   projectPath,
		"file_path":      "res://unsaved_restart.tscn",
		"root_node_type": "Node2D",
	})
	callToolOKWith(t, inst.client, "add_node", map[string]any{
		"project_path": projectPath,
		"parent_path":  ".",
		"node_type":    "Node2D",
		"properties":   map[string]any{"name": "Unsaved"},
	})

	dirtyCtx, cancelDirty := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancelDirty()
	res, err = inst.client.CallTool(dirtyCtx, "restart_editor", map[string]any{
		"project_path": projectPath,
		"skip_save":    true,
	})
	is.NoErr(err)
	is.True(!res.IsError) // restart_editor with skip_save succeeded

	pidsDirty := getInstancePIDs(t, instancesDir)
	newPID = false
	for pid := range pidsDirty {
		if !pidsAfter[pid] {
			newPID = true
		}
	}
	is.True(newPID) // relaunched despite the unsaved scene

	current = callToolOKWith(t, inst.client, "get_current_project", map[string]any{
		"project_path": projectPath,
	})
	is.Equal(current["headless"], true)
}
