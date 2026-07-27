package addon

import "testing"

// TestPathOutsideProject checks that every tool taking a file path rejects a
// path that resolves outside the project (e.g. via ".." segments), which
// begins_with("res://") alone fails to catch.
func TestPathOutsideProject(t *testing.T) {
	const outside = "res://../outside.txt"
	const wantErr = "must be inside the project"

	// Several tools below require a scene open before they reach the path
	// check (attach_script, instantiate_scene, save_scene_as).
	setupSceneWithChild(t, "res://scenes/path_check_setup.tscn")

	cases := []struct {
		tool string
		args map[string]any
	}{
		{"create_script", map[string]any{"file_path": outside}},
		{"open_script", map[string]any{"file_path": outside}},
		{"read_script", map[string]any{"file_path": outside}},
		{"write_script", map[string]any{"file_path": outside, "content": "extends Node\n"}},
		{"save_script", map[string]any{"file_path": outside}},

		{"attach_script", map[string]any{"node_path": ".", "script_path": outside}},

		{"run_project", map[string]any{"scene": outside}},

		{"reimport", map[string]any{"file_paths": []string{outside}}},
		{"get_import_settings", map[string]any{"file_path": outside}},
		{"set_import_settings", map[string]any{"file_path": outside}},

		{"create_resource", map[string]any{"file_path": outside, "resource_type": "LabelSettings"}},
		{"open_resource", map[string]any{"file_path": outside}},
		{"get_resource_properties", map[string]any{"file_path": outside}},
		{"set_resource_properties", map[string]any{"action": "Test", "file_path": outside}},

		{"create_scene", map[string]any{"file_path": outside, "root_node_type": "Node2D"}},
		{"open_scene", map[string]any{"file_path": outside}},
		{"instantiate_scene", map[string]any{"parent_path": ".", "scene_path": outside}},
		{"save_scene_as", map[string]any{"file_path": outside}},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			callToolErr(t, tc.tool, tc.args, wantErr)
		})
	}
}

// TestPathEscapeVariants checks a range of escape forms are all caught, using
// create_script as a representative tool.
func TestPathEscapeVariants(t *testing.T) {
	const wantErr = "must be inside the project"

	for _, path := range []string{
		"res://../outside.gd",
		"res://../../outside.gd",
		"res://scripts/../../outside.gd",
		"res://./../outside.gd",
	} {
		t.Run(path, func(t *testing.T) {
			callToolErr(t, "create_script", map[string]any{"file_path": path}, wantErr)
		})
	}
}
