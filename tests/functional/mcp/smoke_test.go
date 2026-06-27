package mcp

import (
	"testing"

	"github.com/matryer/is"
)

func TestServerListTools(t *testing.T) {
	is := is.New(t)

	tools, err := client.ListTools(testContext(t))
	is.NoErr(err)

	names := make(map[string]ToolDef, len(tools))
	for _, tool := range tools {
		names[tool.Name] = tool
	}

	for _, name := range []string{
		"list_projects",
		"open_godot_project",
		"list_open_projects",
		"get_mcp_configuration",
		"set_mcp_configuration",
	} {
		if _, ok := names[name]; !ok {
			t.Errorf("tools/list is missing local tool %q", name)
		}
	}

	// restart_editor is a local override of a remote tool, so it's still listed.
	_, hasRestart := names["restart_editor"]
	is.True(hasRestart)

	scene, hasScene := names["get_current_scene"]
	is.True(hasScene)
	props, _ := scene.InputSchema["properties"].(map[string]any)
	_, hasProjectPath := props["project_path"]
	is.True(hasProjectPath) // server injected project_path

	// get_current_project is marked doNotForward, so it must not be advertised.
	_, hasCurrentProject := names["get_current_project"]
	is.True(!hasCurrentProject)

	is.True(len(names) > 5) // local + forwarded remote tools
}

func TestForwardRemoteTool(t *testing.T) {
	ensureProjectOpen(t)

	// get_current_scene is read-only and succeeds even with no scene open;
	// reaching it at all proves the server -> websocket -> editor round trip.
	result := callTool(t, "get_current_scene", map[string]any{
		"project_path": projectPath,
	})
	if result.IsError {
		t.Fatalf("forwarded get_current_scene returned an error: %s", result.Text())
	}
}
