package mcp

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func listToolNames(t *testing.T, inst *serverInstance) map[string]bool {
	t.Helper()

	tools, err := inst.client.ListTools(testContext(t))
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}
	return names
}

func TestToolsetsLimitAdvertisedTools(t *testing.T) {
	is := is.New(t)

	inst, err := startServer(t.TempDir(), []string{"--toolsets", "scene"}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst) })

	names := listToolNames(t, inst)
	is.True(names["get_current_scene"])
	is.True(names["add_node"])
	is.True(!names["list_projects"])
	is.True(!names["get_project_settings"])
	is.True(!names["install_godot_version"])
	is.True(!names["restart_editor"])

	_, err = inst.client.CallTool(testContext(t), "list_projects", nil)
	is.True(err != nil) // a filtered-out tool can't be called either
	is.True(strings.Contains(err.Error(), "Unknown tool"))
}

func TestDefaultToolsetsExcludeEngineTools(t *testing.T) {
	is := is.New(t)

	inst, err := startServer(t.TempDir(), nil, nil, os.Getenv("GODAI_TEST_VERBOSE") != "")
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst) })

	names := listToolNames(t, inst)
	is.True(names["list_projects"])
	is.True(names["get_current_scene"])
	is.True(names["pin_project_to_godot_version"])
	is.True(!names["install_godot_version"])
	is.True(!names["list_installed_godot_versions"])
}

func TestUnknownToolsetFailsFast(t *testing.T) {
	out, err := exec.Command(serverBin, "mcp", "--toolsets", "bogus").CombinedOutput()
	if err == nil {
		t.Fatalf("expected a usage error, got success: %s", out)
	}
	if !strings.Contains(string(out), `unknown toolset "bogus"`) {
		t.Errorf("output doesn't name the bad toolset: %s", out)
	}
}
