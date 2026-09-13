package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func screenshotResult(t *testing.T, data string) *core.ToolResult {
	t.Helper()
	raw := `{"content": [{"type": "text", "text": "{\"success\":true}"}, {"type": "image", "data": "` + data + `", "mimeType": "image/png"}], "structuredContent": {"success": true, "notes": ["captured"]}}`
	var result core.ToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	result.Raw = json.RawMessage(raw)
	return &result
}

func TestAttachmentToolResultWritesTheGivenFile(t *testing.T) {
	is := is.New(t)

	var outBuf, errBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, Err: &errBuf}
	path := filepath.Join(t.TempDir(), "shot.png")

	is.NoErr(printAttachmentToolResult(printer, "take_screenshot", screenshotResult(t, "aGVsbG8="), "image/png", path))

	written, err := os.ReadFile(path)
	is.NoErr(err)
	is.Equal(string(written), "hello")
	is.Equal(outBuf.String(), "") // the caller named the file, so nothing to report
	is.Equal(errBuf.String(), "note: captured\n")
}

func TestAttachmentToolResultFallsBackToATempFile(t *testing.T) {
	is := is.New(t)

	var outBuf, errBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, Err: &errBuf}

	is.NoErr(printAttachmentToolResult(printer, "take_screenshot", screenshotResult(t, "aGVsbG8="), "image/png", ""))

	path := strings.TrimSuffix(outBuf.String(), "\n")
	t.Cleanup(func() { os.Remove(path) })
	is.True(strings.HasSuffix(path, ".png"))
	written, err := os.ReadFile(path)
	is.NoErr(err)
	is.Equal(string(written), "hello")
}

func TestAttachmentToolResultJSONAddsAnOutputField(t *testing.T) {
	is := is.New(t)

	var outBuf, errBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, Err: &errBuf, JSON: true}
	result := screenshotResult(t, "aGVsbG8=")

	is.NoErr(printAttachmentToolResult(printer, "take_screenshot", result, "image/png", ""))
	is.Equal(outBuf.String(), `{"success": true, "notes": ["captured"],"output":{"data":"aGVsbG8=","mimeType":"image/png"}}`+"\n")

	outBuf.Reset()
	path := filepath.Join(t.TempDir(), "shot.png")
	is.NoErr(printAttachmentToolResult(printer, "take_screenshot", result, "image/png", path))
	is.Equal(outBuf.String(), `{"success": true, "notes": ["captured"]}`+"\n") // the file holds the image, so the field is left out
	_, err := os.Stat(path)
	is.NoErr(err)
}

func TestAttachmentToolResultJSONFallsBackToTheEnvelope(t *testing.T) {
	is := is.New(t)

	var outBuf bytes.Buffer
	printer := &output.Printer{Out: &outBuf, Err: &bytes.Buffer{}, JSON: true}
	raw := `{"content": [{"type": "image", "data": "aGVsbG8=", "mimeType": "image/png"}]}`
	var result core.ToolResult
	is.NoErr(json.Unmarshal([]byte(raw), &result))
	result.Raw = json.RawMessage(raw)

	is.NoErr(printAttachmentToolResult(printer, "take_screenshot", &result, "image/png", ""))
	is.Equal(outBuf.String(), raw+"\n") // no structured object to add the field to
}

func TestWithOutputFieldHandlesAnEmptyObject(t *testing.T) {
	is := is.New(t)

	payload, ok := withOutputField(json.RawMessage(" {} "), []byte("hello"), "image/png")
	is.True(ok)
	is.Equal(string(payload), `{"output":{"data":"aGVsbG8=","mimeType":"image/png"}}`)

	_, ok = withOutputField(json.RawMessage(`["not", "an", "object"]`), []byte("hello"), "image/png")
	is.True(!ok)
}

func TestAttachmentToolResultWithoutAttachmentFails(t *testing.T) {
	is := is.New(t)

	printer := &output.Printer{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	result := &core.ToolResult{Raw: json.RawMessage(`{"content": []}`)}

	err := printAttachmentToolResult(printer, "take_screenshot", result, "image/png", "")
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "no image/png attachment"))
}
