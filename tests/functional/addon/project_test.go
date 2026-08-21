package addon

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/jsonrpc"
	"gitlab.com/snopek-games/godai/internal/mcp"

	"github.com/matryer/is"
)

func TestInitialize(t *testing.T) {
	is := is.New(t)

	result, err := client.Initialize(testContext(t))
	is.NoErr(err)
	is.Equal(result.ServerInfo.Name, mcp.GodaiMcpName)
	is.Equal(result.ServerInfo.Title, mcp.GodaiMcpTitle)
	is.True(result.ServerInfo.Version != "" && result.ServerInfo.Version != "unknown")
	is.Equal(result.ProtocolVersion, mcp.ProtocolVersion)
	_, ok := result.Capabilities["tools"]
	is.True(ok) // the tools capability is advertised
}

func TestInitializeVersionNegotiation(t *testing.T) {
	t.Run("supported_version_is_echoed", func(t *testing.T) {
		is := is.New(t)
		result, err := client.InitializeWithVersion(testContext(t), "2025-06-18")
		is.NoErr(err)
		is.Equal(result.ProtocolVersion, "2025-06-18")
	})

	t.Run("unsupported_version_falls_back_to_preferred", func(t *testing.T) {
		is := is.New(t)
		result, err := client.InitializeWithVersion(testContext(t), "1999-01-01")
		is.NoErr(err)
		is.Equal(result.ProtocolVersion, mcp.ProtocolVersion)
	})
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
		"save_scene_as",
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
		"clear_log_messages",
		"execute_editor_script",
		"get_editor_settings",
		"set_editor_settings",
		"restart_editor",
		"close_editor",
	}

	var got []string
	byName := make(map[string]ToolDef, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
		byName[tool.Name] = tool
		if tool.Title == "" || tool.Description == "" || tool.InputSchema == nil {
			t.Errorf("tool %s is missing its title, description or inputSchema", tool.Name)
		}
		if err := tool.ValidateAnnotations(); err != nil {
			t.Error(err)
		}
	}

	slices.Sort(got)
	slices.Sort(want)
	is.Equal(got, want)

	// Spot-check representative annotation values against default_tools.json.
	readOnly := byName["get_current_scene"].Annotations
	is.Equal(readOnly["readOnlyHint"], true)
	is.Equal(readOnly["openWorldHint"], false)

	setProps := byName["set_node_properties"].Annotations
	is.Equal(setProps["readOnlyHint"], false)
	is.Equal(setProps["destructiveHint"], true)
	is.Equal(setProps["idempotentHint"], true)
	is.Equal(setProps["openWorldHint"], false)

	addNode := byName["add_node"].Annotations
	is.Equal(addNode["destructiveHint"], false)
	is.Equal(addNode["idempotentHint"], false)

	// stop_project: not read-only, but non-destructive and idempotent.
	stop := byName["stop_project"].Annotations
	is.Equal(stop["readOnlyHint"], false)
	is.Equal(stop["destructiveHint"], false)
	is.Equal(stop["idempotentHint"], true)

	is.Equal(byName["execute_editor_script"].Annotations["openWorldHint"], true)
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
	is.Equal(len(resp.Result), 0) // an error response carries no result
}

func TestGetCurrentProject(t *testing.T) {
	is := is.New(t)

	structured := callToolOK(t, "get_current_project", nil)

	gotPath, _ := structured["project_path"].(string)
	is.True(gotPath != "")

	// Written the way Godot writes its own version, so it has at least a
	// number and a status: "4.5.stable.official".
	version, _ := structured["godot_version"].(string)
	is.True(strings.Count(version, ".") >= 2)
	is.True(strings.HasPrefix(version, "4."))

	if projectDir != "" {
		is.Equal(structured["project_name"], projectName)

		wantPath, err := filepath.EvalSymlinks(projectDir)
		is.NoErr(err)
		gotResolved, err := filepath.EvalSymlinks(gotPath)
		is.NoErr(err)
		is.Equal(gotResolved, wantPath)
	}
}

// A project setting under 'editor_overrides/' overrides the editor setting of
// the same name for this project alone, so it's a second route to Godai's own
// settings - including the tool approvals the AI must not be able to grant
// itself.
func TestGodaiProjectSettingOverridesAreHidden(t *testing.T) {
	const autoApproveOverride = "editor_overrides/godai/tools/auto_approve"
	// Written into project.godot by the harness, so it's really there to find.
	const skipSecretCheckOverride = "editor_overrides/godai/mcp/skip_secret_check"

	t.Run("get_by_name_is_rejected", func(t *testing.T) {
		callToolErr(t, "get_project_settings", map[string]any{
			"names": []string{skipSecretCheckOverride},
		}, "overrides a Godai setting")
	})

	t.Run("set_is_rejected", func(t *testing.T) {
		requireManagedProject(t)

		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{autoApproveOverride: "true"},
		}, "overrides a Godai setting")

		// Rejected before anything was written.
		runEditorScript(t, fmt.Sprintf(`if ProjectSettings.has_setting(%q):
	return FAILED
return OK`, autoApproveOverride))
	})

	t.Run("omitted_when_listing_everything", func(t *testing.T) {
		requireManagedProject(t)
		is := is.New(t)

		// Prove the filter has something to hide, since the listing goes through
		// ProjectSettings rather than the file.
		runEditorScript(t, fmt.Sprintf(`if not ProjectSettings.has_setting(%q):
	return FAILED
return OK`, skipSecretCheckOverride))

		for _, args := range []map[string]any{nil, {"include_defaults": true}} {
			structured := callToolOK(t, "get_project_settings", args)
			settings, _ := structured["settings"].(map[string]any)
			is.True(len(settings) > 0)
			for name := range settings {
				is.True(!strings.HasPrefix(name, "editor_overrides/godai/"))
			}
		}
	})
}

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
		_, hasOutput := structured["output"]
		is.True(hasOutput) // the parser's output is included
	})

	t.Run("success_with_print", func(t *testing.T) {
		is := is.New(t)
		structured := callToolOK(t, "execute_editor_script", map[string]any{
			"code": "print(\"hello from functional test\")",
		})
		is.Equal(structured["success"], true)
		output, _ := json.Marshal(structured["output"])
		is.True(strings.Contains(string(output), "hello from functional test"))

		// Script output is returned verbatim, without log timestamps.
		lines, _ := structured["output"].([]any)
		for _, line := range asStrings(lines) {
			is.True(!logTimestampRegexp.MatchString(line))
		}
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

func TestRunProject(t *testing.T) {
	// Running the project spawns a separate game process; only exercise it on a project we control.
	requireManagedProject(t)

	setupSceneWithChild(t, "res://scenes/run_test.tscn")
	callToolOK(t, "save_scene", nil)

	t.Run("run_current_and_stop", func(t *testing.T) {
		is := is.New(t)

		t.Cleanup(func() {
			client.CallTool(testContext(t), "stop_project", map[string]any{})
		})

		structured := callToolOK(t, "run_project", map[string]any{
			"scene": "current",
		})
		is.Equal(structured["success"], true)

		stopped := callToolOK(t, "stop_project", nil)
		is.Equal(stopped["success"], true)

		runEditorScript(t, `if EditorInterface.is_playing_scene():
	return FAILED
return OK`)
	})

	t.Run("no_main_scene", func(t *testing.T) {
		callToolErr(t, "run_project", map[string]any{
			"scene": "main",
		}, "No main scene")
	})

	t.Run("nonexistent_scene", func(t *testing.T) {
		callToolErr(t, "run_project", map[string]any{
			"scene": "res://scenes/no_such_scene.tscn",
		}, "doesn't exist")
	})

	t.Run("clear_log_messages", func(t *testing.T) {
		is := is.New(t)

		// Otherwise the game opens a window on a developer's machine.
		callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{"editor/run/main_run_args": "--headless"},
		})

		t.Cleanup(func() {
			client.CallTool(testContext(t), "stop_project", map[string]any{})
		})

		marker := "godai-run-clear-marker-91c4e7"
		runEditorScript(t, fmt.Sprintf("print(%q)\nreturn OK", marker))
		is.True(logContainsMarker(t, marker))

		// A rejected run must leave the captured log alone.
		callToolErr(t, "run_project", map[string]any{
			"scene":              "res://scenes/no_such_scene.tscn",
			"clear_log_messages": true,
		}, "doesn't exist")
		is.True(logContainsMarker(t, marker))

		structured := callToolOK(t, "run_project", map[string]any{
			"scene":              "current",
			"clear_log_messages": true,
		})
		is.Equal(structured["success"], true)
		is.True(!logContainsMarker(t, marker))

		stopped := callToolOK(t, "stop_project", nil)
		is.Equal(stopped["success"], true)
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
			is.True(logTimestampRegexp.MatchString(line)) // captured log lines get timestamps
		}
	}
	is.True(found) // the marker printed by the script was captured
}

var logTimestampRegexp = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\.\d{3}\] `)

func TestClearLogMessages(t *testing.T) {
	is := is.New(t)

	marker := "godai-clear-marker-a7f31c"
	runEditorScript(t, fmt.Sprintf("print(%q)\nreturn OK", marker))
	is.True(logContainsMarker(t, marker))

	structured := callToolOK(t, "clear_log_messages", nil)
	is.Equal(structured["success"], true)
	is.True(!logContainsMarker(t, marker))

	// Capture must keep working after a clear.
	after := "godai-post-clear-marker-a7f31c"
	runEditorScript(t, fmt.Sprintf("print(%q)\nreturn OK", after))
	is.True(logContainsMarker(t, after))
}

func logContainsMarker(t *testing.T, marker string) bool {
	t.Helper()
	structured := callToolOK(t, "get_log_messages", map[string]any{"count": float64(0)})
	raw, _ := structured["messages"].([]any)
	for _, line := range asStrings(raw) {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}
