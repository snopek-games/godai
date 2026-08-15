package mcp

import "testing"

func TestComputeEnabledTools(t *testing.T) {
	byDefault := computeEnabledTools(nil)
	if byDefault["list_installed_godot_versions"] {
		t.Error("default enables the engine toolset")
	}
	for _, name := range []string{"open_godot_project", "pin_project_to_godot_version", "get_current_scene", "restart_editor", "get_godai_settings", "get_current_project"} {
		if !byDefault[name] {
			t.Errorf("default doesn't enable %s", name)
		}
	}

	withEngine := computeEnabledTools([]string{"default", "engine"})
	if !withEngine["install_godot_version"] || !withEngine["open_godot_project"] {
		t.Error("default,engine doesn't enable both default and engine tools")
	}

	sceneOnly := computeEnabledTools([]string{"scene"})
	if !sceneOnly["add_node"] {
		t.Error("scene doesn't enable add_node")
	}
	if sceneOnly["list_projects"] {
		t.Error("scene enables the project toolset")
	}
}

func TestLocalToolsAndDefinitionsMatch(t *testing.T) {
	server := &Server{localTools: map[string]*Tool{}}
	server.setupLocalTools()

	for name, tool := range server.localTools {
		if tool.Definition == nil {
			t.Errorf("%s has no definition", name)
		}
	}

	for name := range GetLocalToolDefinitions() {
		if _, ok := server.localTools[name]; !ok {
			t.Errorf("%s is defined in local_tools.json but never registered", name)
		}
	}
}
