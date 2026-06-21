package editor

import (
	"encoding/json"
	"fmt"
	"godai/mcp/jsonrpc"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestInitialize(t *testing.T) {
	is := is.New(t)

	result, err := client.Initialize(testContext(t))
	is.NoErr(err)
	is.Equal(result.ServerInfo.Name, "Godai")
	is.True(result.ServerInfo.Version != "" && result.ServerInfo.Version != "unknown")
	is.Equal(result.ProtocolVersion, "2025-06-18")
	_, ok := result.Capabilities["tools"]
	is.True(ok)
}

func TestListTools(t *testing.T) {
	is := is.New(t)

	tools, err := client.ListTools(testContext(t))
	is.NoErr(err)

	want := []string{
		"get_current_project",
		"get_project_settings",
		"set_project_settings",
		"get_current_scene",
		"get_current_scene_tree",
		"create_scene",
		"open_scene",
		"get_node_properties",
		"set_node_properties",
		"add_node",
		"remove_node",
		"create_resource",
		"open_resource",
		"get_resource_properties",
		"set_resource_properties",
		"get_selected_nodes",
		"instantiate_scene",
		"save_scene",
		"add_to_group",
		"remove_from_group",
		"get_node_groups",
		"connect_signal",
		"disconnect_signal",
		"attach_script",
		"detach_script",
		"create_script",
		"open_script",
		"save_script",
		"read_script",
		"write_script",
		"run_project",
		"stop_project",
		"reimport",
		"get_import_settings",
		"set_import_settings",
		"get_log_messages",
		"execute_editor_script",
		"get_editor_settings",
		"set_editor_settings",
		"restart_editor",
	}

	var got []string
	for _, tool := range tools {
		got = append(got, tool.Name)
		if tool.Title == "" || tool.Description == "" || tool.InputSchema == nil {
			t.Errorf("tool %s is missing its title, description or inputSchema", tool.Name)
		}
	}

	slices.Sort(got)
	slices.Sort(want)
	is.Equal(got, want)
}

func TestCallUnknownTool(t *testing.T) {
	is := is.New(t)

	resp, err := client.Call(testContext(t), "tools/call", map[string]any{
		"name":      "no_such_tool",
		"arguments": map[string]any{},
	})
	is.True(err != nil)
	is.True(resp != nil && resp.Error != nil)
	is.Equal(resp.Error.Code, jsonrpc.ErrorCode(jsonrpc.InvalidParamsErrorCode))
	is.True(strings.Contains(resp.Error.Message, "Unknown tool: no_such_tool"))
	is.Equal(len(resp.Result), 0)
}

func TestGetCurrentProject(t *testing.T) {
	is := is.New(t)

	structured := callToolOK(t, "get_current_project", nil)

	gotPath, _ := structured["project_path"].(string)
	is.True(gotPath != "")

	if projectDir != "" {
		is.Equal(structured["project_name"], projectName)

		wantPath, err := filepath.EvalSymlinks(projectDir)
		is.NoErr(err)
		gotResolved, err := filepath.EvalSymlinks(gotPath)
		is.NoErr(err)
		is.Equal(gotResolved, wantPath)
	}
}

// TestNoSceneOpen covers the code paths that require no scene to be open in
// the editor.

func TestExecuteEditorScript(t *testing.T) {
	t.Run("empty_code", func(t *testing.T) {
		callToolErr(t, "execute_editor_script", map[string]any{
			"code": "",
		}, "'code' is required")
	})

	t.Run("parse_error", func(t *testing.T) {
		is := is.New(t)
		structured := callToolErr(t, "execute_editor_script", map[string]any{
			"code": "this is not valid gdscript ((",
		}, "Script failed to parse")
		_, hasLog := structured["log"]
		is.True(hasLog)
	})

	t.Run("success_with_print", func(t *testing.T) {
		is := is.New(t)
		structured := callToolOK(t, "execute_editor_script", map[string]any{
			"code": "print(\"hello from functional test\")",
		})
		is.Equal(structured["success"], true)
		output, _ := json.Marshal(structured["output"])
		is.True(strings.Contains(string(output), "hello from functional test"))
	})

	t.Run("spaces_converted_to_tabs", func(t *testing.T) {
		is := is.New(t)
		// Space-based indentation gets converted to tabs by _process_user_code.
		code := "for i in range(2):\n    print(\"iteration \", i)\n    if i == 1:\n        print(\"last one\")"
		structured := callToolOK(t, "execute_editor_script", map[string]any{
			"code": code,
		})
		is.Equal(structured["success"], true)
		output, _ := json.Marshal(structured["output"])
		is.True(strings.Contains(string(output), "last one"))
	})

	t.Run("ansi_escapes_stripped_from_output", func(t *testing.T) {
		is := is.New(t)
		// Control characters in the output would break the JSON encoding of
		// the response, since Godot's JSON.stringify() doesn't escape them.
		structured := callToolOK(t, "execute_editor_script", map[string]any{
			"code": "print(\"\x1b[31mred\x1b[0m plain\")",
		})
		is.Equal(structured["success"], true)
		output, _ := json.Marshal(structured["output"])
		is.True(strings.Contains(string(output), "red plain"))
		is.True(!strings.Contains(string(output), `\u001b`))
	})

	t.Run("user_code_returns_error", func(t *testing.T) {
		callToolErr(t, "execute_editor_script", map[string]any{
			"code": "return FAILED",
		}, "Failed to execute script")
	})

	t.Run("script_can_access_editor", func(t *testing.T) {
		is := is.New(t)
		structured := callToolOK(t, "execute_editor_script", map[string]any{
			"code": "print(EditorInterface.get_edited_scene_root() != null)",
		})
		is.Equal(structured["success"], true)
	})
}

// settleEditor lets the editor process a couple of frames, so deferred state
// (selection updates, newly-opened script tabs, etc) has a chance to settle.

func TestRunProject(t *testing.T) {
	// Running the project spawns a separate game process, so only exercise it
	// against a project we control.
	requireManagedProject(t)

	setupSceneWithChild(t, "res://scenes/run_test.tscn")
	callToolOK(t, "save_scene", nil)

	t.Run("run_current_and_stop", func(t *testing.T) {
		is := is.New(t)

		// Make sure we always stop, even if an assertion fails.
		t.Cleanup(func() {
			client.CallTool(testContext(t), "stop_project", map[string]any{})
		})

		structured := callToolOK(t, "run_project", map[string]any{
			"scene": "current",
		})
		is.Equal(structured["success"], true)

		stopped := callToolOK(t, "stop_project", nil)
		is.Equal(stopped["success"], true)

		// After stopping, the editor is no longer playing.
		runEditorScript(t, `if EditorInterface.is_playing_scene():
	return FAILED
return OK`)
	})

	t.Run("no_main_scene", func(t *testing.T) {
		// The test project has no main scene configured.
		callToolErr(t, "run_project", map[string]any{
			"scene": "main",
		}, "No main scene")
	})

	t.Run("nonexistent_scene", func(t *testing.T) {
		callToolErr(t, "run_project", map[string]any{
			"scene": "res://scenes/no_such_scene.tscn",
		}, "doesn't exist")
	})
}

func TestGetLogMessages(t *testing.T) {
	is := is.New(t)

	marker := "godai-log-marker-d5ed93b"
	runEditorScript(t, fmt.Sprintf("print(%q)\nreturn OK", marker))

	structured := callToolOK(t, "get_log_messages", map[string]any{
		"count": float64(500),
	})
	messages, _ := structured["messages"].([]any)
	is.True(len(messages) > 0)

	found := false
	for _, raw := range messages {
		if line, ok := raw.(string); ok && strings.Contains(line, marker) {
			found = true
			break
		}
	}
	is.True(found)
}

// asStrings converts a decoded JSON array into a []string.
