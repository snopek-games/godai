package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func renderToolResult(t *testing.T, toolName string, structured string) (stdout, stderr string) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, Err: &errBuf}
	result := &core.ToolResult{
		Raw:               json.RawMessage(`{"structuredContent":` + structured + `}`),
		StructuredContent: json.RawMessage(structured),
	}
	if err := printToolResult(printer, toolName, result); err != nil {
		t.Fatal(err)
	}
	return outBuf.String(), errBuf.String()
}

func TestToolResultPlainSuccessIsSilent(t *testing.T) {
	is := is.New(t)

	stdout, stderr := renderToolResult(t, "create_scene", `{"success": true}`)
	is.Equal(stdout, "")
	is.Equal(stderr, "")
}

func TestToolResultMessageLists(t *testing.T) {
	is := is.New(t)

	stdout, stderr := renderToolResult(t, "set_project_settings",
		`{"success": false, "errors": ["bad setting"], "warnings": ["deprecated"], "notes": ["fyi"], "output": ["Godot said this"]}`)
	is.Equal(stdout, "Godot said this\n")
	is.Equal(stderr, "note: fyi\nwarning: deprecated\nerror: bad setting\n")
}

func TestToolResultScalarFields(t *testing.T) {
	is := is.New(t)

	stdout, stderr := renderToolResult(t, "save_scene",
		`{"success": true, "scene_path": "res://test.tscn"}`)
	is.Equal(stdout, "scene_path: res://test.tscn\n")
	is.Equal(stderr, "")

	stdout, _ = renderToolResult(t, "get_current_project",
		`{"project_path": "/games/demo", "project_name": "Demo", "headless": false, "godot_version": "4.5.stable.official"}`)
	is.Equal(stdout, "project_path: /games/demo\nproject_name: Demo\nheadless: false\ngodot_version: 4.5.stable.official\n")
}

func TestToolResultSoleListPrintsBareLines(t *testing.T) {
	is := is.New(t)

	stdout, _ := renderToolResult(t, "get_log_messages",
		`{"messages": ["first line", "second line"]}`)
	is.Equal(stdout, "first line\nsecond line\n")
}

func TestToolResultSoleMapPrintsEntries(t *testing.T) {
	is := is.New(t)

	stdout, _ := renderToolResult(t, "get_project_settings",
		`{"settings": {"application/config/name": "\"Demo\"", "application/run/main_scene": "\"res://main.tscn\""}}`)
	is.Equal(stdout, "application/config/name = \"Demo\"\napplication/run/main_scene = \"res://main.tscn\"\n")
}

func TestToolResultNestedMapPrintsSections(t *testing.T) {
	is := is.New(t)

	stdout, _ := renderToolResult(t, "get_node_properties",
		`{"nodes": {"Sphere": {"speed": "0.25"}, "Sphere:mesh": {"radius": "1.0"}}}`)
	is.Equal(stdout, "Sphere:\n  speed = 0.25\nSphere:mesh:\n  radius = 1.0\n")
}

func TestToolResultSceneTree(t *testing.T) {
	is := is.New(t)

	stdout, stderr := renderToolResult(t, "get_current_scene_tree",
		`{"name": "Test", "type": "Node3D", "path": ".", "children": [{"name": "Sphere", "type": "MeshInstance3D", "path": "Sphere", "script": "res://spin.gd"}]}`)
	is.Equal(stdout, "Test (Node3D)\n  Sphere (MeshInstance3D) res://spin.gd\n")
	is.Equal(stderr, "")
}

func TestToolResultReadScriptPrintsBareContent(t *testing.T) {
	is := is.New(t)

	stdout, stderr := renderToolResult(t, "read_script",
		`{"content": "extends Node\n", "open_in_editor": true}`)
	is.Equal(stdout, "extends Node\n")
	is.Equal(stderr, "note: the script is open in the editor, so this is the live buffer\n")
}

func TestToolResultErrorsAloneOnFailure(t *testing.T) {
	is := is.New(t)

	stdout, stderr := renderToolResult(t, "get_current_scene_tree",
		`{"errors": ["No scene open"]}`)
	is.Equal(stdout, "")
	is.Equal(stderr, "error: No scene open\n")
}

func TestToolResultJSONPrintsStructuredContentOnly(t *testing.T) {
	is := is.New(t)

	var outBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, JSON: true}
	structured := `{"success":true}`
	err := printToolResult(printer, "create_scene", &core.ToolResult{
		Raw:               json.RawMessage(`{"structuredContent":` + structured + `}`),
		StructuredContent: json.RawMessage(structured),
	})
	is.NoErr(err)
	is.Equal(outBuf.String(), structured+"\n")
}

func TestToolResultJSONFallsBackToRaw(t *testing.T) {
	is := is.New(t)

	var outBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, JSON: true}
	raw := `{"content":[{"type":"text","text":"hi"}]}`
	err := printToolResult(printer, "some_custom_tool", &core.ToolResult{Raw: json.RawMessage(raw)})
	is.NoErr(err)
	is.Equal(outBuf.String(), raw+"\n")
}

func TestToolResultFallsBackToPrettyJSON(t *testing.T) {
	is := is.New(t)

	stdout, _ := renderToolResult(t, "some_custom_tool", `["not", "an", "object"]`)
	is.Equal(stdout, "[\n  \"not\",\n  \"an\",\n  \"object\"\n]\n")
}
