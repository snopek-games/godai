package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func waitForClosedProject(t *testing.T, c *harness.MCPClient, projectPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := listOpenProjects(t, c)[projectPath]; !ok {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("editor for %s never disconnected from the server", projectPath)
}

func TestCloseEditor(t *testing.T) {
	is := is.New(t)

	godotBin, err := harness.FindGodot()
	is.NoErr(err)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	rootDir := filepath.Join(xdgBase, "projects")
	projectDir := filepath.Join(rootDir, "close_proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	is.NoErr(harness.CreateTestProject(projectDir, harness.ProjectOptions{
		Name:         "Close Project",
		InstallAddon: true,
	}))
	projectPath, err := filepath.EvalSymlinks(projectDir)
	is.NoErr(err)

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
	t.Cleanup(func() { stopServer(inst) })
	dumpLogOnFailure(t, "server log", inst.logPath)

	_, errBad := inst.client.CallTool(testContext(t), "close_editor", map[string]any{"project_path": "/definitely/not/a/real/path"})
	is.True(errBad != nil) // invalid project_path
	notConnected, err := inst.client.CallTool(testContext(t), "close_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(notConnected.IsError) // no editor connected for the project yet

	// The first launch imports the project, which can be slow.
	editor := launchConnectableEditor(t, godotBin, projectDir, editorXDG)
	// close_editor genuinely shuts the editor down; Wait for it ourselves to
	// confirm that, and make sure it can't leak if anything fails first.
	exited := make(chan error, 1)
	go func() { exited <- editor.Wait() }()
	t.Cleanup(func() {
		if editor.Process != nil {
			editor.Process.Kill()
		}
	})
	waitForOpenProject(t, inst.client, projectPath, harness.OpenTimeout(180*time.Second))

	// The call returns only once the editor has disconnected.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	res, err := inst.client.CallTool(ctx, "close_editor", map[string]any{"project_path": projectPath})
	is.NoErr(err)
	is.True(!res.IsError) // close_editor succeeded

	// The editor process actually exited rather than lingering.
	select {
	case waitErr := <-exited:
		_ = waitErr // a clean quit may still report a non-zero status; exiting at all is the point
	case <-time.After(30 * time.Second):
		t.Fatal("editor process did not exit after close_editor")
	}

	// And the server no longer lists the project as open.
	waitForClosedProject(t, inst.client, projectPath, 30*time.Second)
}
