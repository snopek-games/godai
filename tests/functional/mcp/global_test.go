package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/godot"
	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func TestGlobalMode(t *testing.T) {
	is := is.New(t)

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	basePathDir := filepath.Join(base, "base-projects")
	baseProject := filepath.Join(basePathDir, "from_base_path")
	mustCreateProject(t, baseProject, "From Base Path")

	// Discovered via the project manager's projects.cfg, which the server reads
	// from the editor data dir; writing it under our isolated one (not the real
	// one) keeps the test off the developer's actual project list.
	pmProject := filepath.Join(base, "elsewhere", "from_project_manager")
	mustCreateProject(t, pmProject, "From Project Manager")
	writeProjectsCfg(t, filepath.Join(isolation.GodotEditorDataDir(base), "projects.cfg"), pmProject)

	globalInstances := isolation.GodaiInstancesDir(base)
	inst, err := startServer(base, []string{
		"--global",
		"--project-base-path", basePathDir,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", globalInstances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	projects := listProjects(t, inst.client)
	is.Equal(projects[baseProject], "From Base Path")     // found via --project-base-path
	is.Equal(projects[pmProject], "From Project Manager") // found via projects.cfg

	// In global mode the project base path is part of the configuration (it's
	// hidden in non-global mode).
	cfg := getConfig(t, inst.client)
	is.Equal(cfg["project_base_path"], basePathDir)

	newBase := filepath.Join(base, "base-projects-2")
	mustCreateProject(t, filepath.Join(newBase, "another"), "Another")
	out := callToolOKWith(t, inst.client, "set_godai_settings", map[string]any{
		"project_base_path": newBase,
	})
	is.Equal(out["success"], true)

	cfg = getConfig(t, inst.client)
	is.Equal(cfg["project_base_path"], newBase) // the new base path stuck
}

func mustCreateProject(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := harness.CreateTestProject(dir, harness.ProjectOptions{Name: name}); err != nil {
		t.Fatal(err)
	}
}

// Uses the addon's own config-file writer so the projects.cfg format round-trips.
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
	return callToolOKWith(t, c, "get_godai_settings", nil)
}

func TestGlobalModeOpenAndConnect(t *testing.T) {
	is := is.New(t)

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	project := filepath.Join(base, "anywhere", "global_proj")
	mustCreateProject(t, project, "Global Project")

	instances := isolation.GodaiInstancesDir(base)

	inst, err := startServer(base, []string{
		"--global",
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
		"--editor-retry-delay", "1",
	}, append([]string{
		"GODAI_MCP_TRANSPORT=websocket",
		"GODAI_DISABLE_CLOSE=1",
		"GODAI_AUTO_APPROVE_TOOLS=1",
	}, harness.MCPPortEnv()...), os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() {
		killEditorInstances(instances)
		stopServer(inst.cmd)
	})
	dumpLogOnFailure(t, "server log", inst.logPath)
	dumpLogOnFailure(t, "editor log", filepath.Join(isolation.GodaiCacheDir(base), "editor-logs", "*.log"))

	// Succeeds even though the project is under no root: global mode spawns the
	// editor and the GlobalConnectionScanner connects back to it.
	out := callToolOKWith(t, inst.client, "open_godot_project", map[string]any{
		"project_path": project,
	})
	is.Equal(out["success"], true)

	projects := listOpenProjects(t, inst.client)
	is.Equal(projects[project], "Global Project")
}

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

func TestGlobalAndRootMutuallyExclusive(t *testing.T) {
	cmd := exec.Command(serverBin, "mcp", "--global", "--root", t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a non-zero exit, got success; output:\n%s", out)
	}
	if !strings.Contains(string(out), "mutually exclusive") {
		t.Fatalf("expected a mutual-exclusivity error, got:\n%s", out)
	}
}
