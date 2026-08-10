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
		"get_godai_settings",
		"set_godai_settings",
	} {
		if _, ok := names[name]; !ok {
			t.Errorf("tools/list is missing local tool %q", name)
		}
	}

	// restart_editor and close_editor are local overrides of remote tools, so
	// they're still listed.
	_, hasRestart := names["restart_editor"]
	is.True(hasRestart)
	_, hasClose := names["close_editor"]
	is.True(hasClose)

	scene, hasScene := names["get_current_scene"]
	is.True(hasScene)
	props, _ := scene.InputSchema["properties"].(map[string]any)
	_, hasProjectPath := props["project_path"]
	is.True(hasProjectPath) // server injected project_path

	// get_current_project is marked doNotForward, so it must not be advertised.
	_, hasCurrentProject := names["get_current_project"]
	is.True(!hasCurrentProject)

	for _, tool := range tools {
		if err := tool.ValidateAnnotations(); err != nil {
			t.Error(err)
		}
	}

	is.Equal(scene.Annotations["title"], scene.Title)
	is.Equal(scene.Annotations["readOnlyHint"], true)
	is.Equal(scene.Annotations["openWorldHint"], false)

	setProps := names["set_node_properties"].Annotations
	is.Equal(setProps["readOnlyHint"], false)
	is.Equal(setProps["destructiveHint"], true)
	is.Equal(setProps["idempotentHint"], true)

	saveAs, hasSaveAs := names["save_scene_as"]
	is.True(hasSaveAs) // forwarded remote tool
	saveAsProps, _ := saveAs.InputSchema["properties"].(map[string]any)
	_, hasFilePath := saveAsProps["file_path"]
	is.True(hasFilePath)
	is.Equal(saveAs.Annotations["readOnlyHint"], false)
	is.Equal(saveAs.Annotations["destructiveHint"], false)
	is.Equal(saveAs.Annotations["openWorldHint"], false)

	is.True(len(names) > 5) // local + forwarded remote tools
}

func TestForwardRemoteTool(t *testing.T) {
	ensureProjectOpen(t)

	// get_project_settings is read-only and succeeds even with no scene open;
	// reaching it at all proves the server -> websocket -> editor round trip.
	result := callTool(t, "get_project_settings", map[string]any{
		"project_path": projectPath,
	})
	if result.IsError {
		t.Fatalf("forwarded get_project_settings returned an error: %s", result.Text())
	}
}
