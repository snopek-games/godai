package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func callTool(t *testing.T, name string, args map[string]any) *ToolCallResult {
	t.Helper()
	result, err := client.CallTool(testContext(t), name, args)
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return result
}

func callToolOK(t *testing.T, name string, args map[string]any) map[string]any {
	t.Helper()
	result := callTool(t, name, args)
	if result.IsError {
		t.Fatalf("tools/call %s returned an error: %s", name, result.Text())
	}
	structured, err := result.Structured()
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return structured
}

func callToolErr(t *testing.T, name string, args map[string]any, wantSubstr string) map[string]any {
	t.Helper()
	result := callTool(t, name, args)
	if !result.IsError {
		t.Fatalf("tools/call %s should have returned an error, got: %s", name, result.Text())
	}
	structured, err := result.Structured()
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	errMsg, _ := structured["error"].(string)
	if !strings.Contains(errMsg, wantSubstr) {
		t.Fatalf("tools/call %s error %q does not contain %q", name, errMsg, wantSubstr)
	}
	return structured
}

func requireManagedProject(t *testing.T) {
	t.Helper()
	if projectDir == "" {
		t.Skip("requires a test-managed project directory (not GODAI_TEST_PORT mode)")
	}
}

func runEditorScript(t *testing.T, code string) map[string]any {
	t.Helper()
	return callToolOK(t, "execute_editor_script", map[string]any{
		"code": code,
	})
}

func closeAllScenes(t *testing.T) {
	t.Helper()
	runEditorScript(t, `EditorInterface.save_all_scenes()
for i in range(100):
	if EditorInterface.get_edited_scene_root() == null:
		return OK
	EditorInterface.close_scene()
	await Engine.get_main_loop().process_frame
return FAILED`)
}

func setupSceneWithChild(t *testing.T, scenePath string) {
	t.Helper()
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      scenePath,
		"root_node_type": "Node2D",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node2D",
		"properties": map[string]any{
			"name": "MyChild",
		},
	})
}

func settleEditor(t *testing.T) {
	t.Helper()
	runEditorScript(t, `await Engine.get_main_loop().process_frame
await Engine.get_main_loop().process_frame
return OK`)
}

// Opens a file that isn't a Script in the script editor, where it gets a tab of
// its own alongside the open scripts. Only works for a file some ResourceLoader
// recognizes (a .json, say): a .txt has no loader, so there is no resource to
// hand to edit_resource(), and the script editor's own "File > Open..." is the
// only way in.
func openNonScriptInScriptEditor(t *testing.T, path string) {
	t.Helper()
	runEditorScript(t, fmt.Sprintf(`var res := ResourceLoader.load(%q)
if not res:
	push_error("nothing to edit: " + %q + " didn't load as a resource")
	return FAILED
EditorInterface.edit_resource(res)
await Engine.get_main_loop().process_frame
return OK`, path, path))
}

func closeScriptEditorFiles(t *testing.T, paths ...string) {
	t.Helper()
	quoted := make([]string, 0, len(paths))
	for _, path := range paths {
		quoted = append(quoted, fmt.Sprintf("%q", path))
	}
	runEditorScript(t, fmt.Sprintf(`var script_editor := EditorInterface.get_script_editor()
script_editor.save_all_scripts()
for path in [%s]:
	script_editor.close_file(path)
	await Engine.get_main_loop().process_frame
return OK`, strings.Join(quoted, ", ")))
}

// Returns the live text of every buffer open in the script editor, so a test
// can check which file a tool actually wrote to.
func scriptEditorBuffers(t *testing.T) []string {
	t.Helper()
	out := runEditorScript(t, `for editor in EditorInterface.get_script_editor().get_open_script_editors():
	var base = editor.get_base_editor()
	if base is TextEdit:
		print("BUFFER:", JSON.stringify(base.text))
return OK`)

	lines, _ := out["output"].([]any)
	var buffers []string
	for _, line := range asStrings(lines) {
		encoded, found := strings.CutPrefix(line, "BUFFER:")
		if !found {
			continue
		}
		var text string
		if err := json.Unmarshal([]byte(encoded), &text); err != nil {
			t.Fatalf("decoding editor buffer %q: %v", encoded, err)
		}
		buffers = append(buffers, text)
	}
	return buffers
}

func writeProjectFileFromEditor(t *testing.T, path, content string) {
	t.Helper()
	runEditorScript(t, fmt.Sprintf(`DirAccess.make_dir_recursive_absolute(%q.get_base_dir())
var f = FileAccess.open(%q, FileAccess.WRITE)
if not f:
	push_error("could not write " + %q)
	return FAILED
f.store_string(%q)
f.close()
EditorInterface.get_resource_filesystem().update_file(%q)
return OK`, path, path, path, content, path))
}

func readProjectFile(t *testing.T, relPath string) string {
	t.Helper()
	if projectDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("reading %s: %v", relPath, err)
	}
	return string(data)
}

func asStrings(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}
