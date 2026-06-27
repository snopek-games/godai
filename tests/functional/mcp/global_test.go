package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"godai/mcp/godot"
	"godai/tests/functional/internal/harness"
)

// TestGlobalMode exercises the server's --global mode, where it discovers
// projects from the configured base path and from Godot's project manager
// (projects.cfg) rather than walking a root.
//
// Everything is isolated under a private temp tree, including the XDG_*
// directories the server reads projects.cfg and the editor instance files
// from, so the test never touches the developer's real Godot installation.
func TestGlobalMode(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	// A project discovered via --project-base-path.
	basePathDir := filepath.Join(xdgBase, "base-projects")
	baseProject := filepath.Join(basePathDir, "from_base_path")
	mustCreateProject(t, baseProject, "From Base Path")

	// A project discovered only via the project manager's projects.cfg, which
	// the server reads from XDG_DATA_HOME/godot. Writing it under our isolated
	// XDG_DATA_HOME (not the real one) is what proves the isolation keeps us off
	// the developer's actual project list.
	pmProject := filepath.Join(xdgBase, "elsewhere", "from_project_manager")
	mustCreateProject(t, pmProject, "From Project Manager")
	writeProjectsCfg(t, filepath.Join(xdgBase, "data", "godot", "projects.cfg"), pmProject)

	// Start a second server in --global mode. It has no editors to find (its
	// instances dir is empty), so it needs neither an editor nor the MCP env
	// vars; --godot-path just has to be valid.
	globalInstances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	inst, err := startServer(xdgBase, []string{
		"--global",
		"--project-base-path", basePathDir,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", globalInstances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	// list_projects must surface both discovery sources.
	projects := listProjects(t, inst.client)
	is.Equal(projects[baseProject], "From Base Path")     // found via --project-base-path
	is.Equal(projects[pmProject], "From Project Manager") // found via projects.cfg

	// In global mode the configuration round-trips the project base path (it's
	// hidden in non-global mode).
	cfg := getConfig(t, inst.client)
	is.Equal(cfg["project_base_path"], basePathDir)

	newBase := filepath.Join(xdgBase, "base-projects-2")
	mustCreateProject(t, filepath.Join(newBase, "another"), "Another")
	out := callToolOKWith(t, inst.client, "set_mcp_configuration", map[string]any{
		"project_base_path": newBase,
	})
	is.Equal(out["success"], true)

	cfg = getConfig(t, inst.client)
	is.Equal(cfg["project_base_path"], newBase) // the new base path stuck
}

// mustCreateProject makes a directory and writes a minimal Godot project into
// it with the given name.
func mustCreateProject(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := harness.CreateTestProject(dir, harness.ProjectOptions{Name: name}); err != nil {
		t.Fatal(err)
	}
}

// writeProjectsCfg writes a Godot project-manager list containing one project,
// using the addon's own config-file writer so the format round-trips.
func writeProjectsCfg(t *testing.T, path, projectPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cf := godot.NewConfigFile()
	cf.Set(projectPath, "favorite", false)
	if err := cf.WriteFile(path); err != nil {
		t.Fatal(err)
	}
}

// listProjects calls list_projects and returns a map of project_path to
// project_name.
func listProjects(t *testing.T, c *harness.MCPClient) map[string]string {
	t.Helper()
	structured := callToolOKWith(t, c, "list_projects", nil)
	out := map[string]string{}
	projects, _ := structured["projects"].([]any)
	for _, p := range projects {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["project_path"].(string)
		name, _ := m["project_name"].(string)
		out[path] = name
	}
	return out
}

func getConfig(t *testing.T, c *harness.MCPClient) map[string]any {
	t.Helper()
	return callToolOKWith(t, c, "get_mcp_configuration", nil)
}

// TestGlobalModeOpenAndConnect exercises the global-only paths around opening
// and connecting to an editor: open_godot_project bypasses the under-a-root
// check (global mode has no roots), and the GlobalConnectionScanner connects to
// the editor it spawns regardless of where the project lives.
func TestGlobalModeOpenAndConnect(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	// A project under no configured root or base path at all.
	project := filepath.Join(xdgBase, "anywhere", "global_proj")
	mustCreateProject(t, project, "Global Project")

	instances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	port, err := harness.FindFreePort()
	is.NoErr(err)

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
		"GODAI_DISABLE_RESTART=1",
	}, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() {
		killEditorInstances(instances)
		stopServer(inst.cmd)
	})

	// In global mode this succeeds even though the project is under no root: it
	// spawns the editor, and the GlobalConnectionScanner connects back to it.
	out := callToolOKWith(t, inst.client, "open_godot_project", map[string]any{
		"project_path": project,
	})
	is.Equal(out["success"], true)

	// The connected editor is now listed.
	projects := listOpenProjects(t, inst.client)
	is.Equal(projects[project], "Global Project")
}

// listOpenProjects calls list_open_projects and returns a map of project_path
// to project_name.
func listOpenProjects(t *testing.T, c *harness.MCPClient) map[string]string {
	t.Helper()
	structured := callToolOKWith(t, c, "list_open_projects", nil)
	out := map[string]string{}
	projects, _ := structured["projects"].([]any)
	for _, p := range projects {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["project_path"].(string)
		name, _ := m["project_name"].(string)
		out[path] = name
	}
	return out
}

// TestGlobalAndRootMutuallyExclusive checks the startup guard that rejects
// --global together with --root.
func TestGlobalAndRootMutuallyExclusive(t *testing.T) {
	cmd := exec.Command(serverBin, "--global", "--root", t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a non-zero exit, got success; output:\n%s", out)
	}
	if !strings.Contains(string(out), "mutually exclusive") {
		t.Fatalf("expected a mutual-exclusivity error, got:\n%s", out)
	}
}
