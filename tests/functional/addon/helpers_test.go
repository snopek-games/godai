package addon

import (
	"context"
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

// callToolOK calls a tool, asserts it succeeded, and returns the structured content.

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

// callToolErr calls a tool, asserts it failed, and that the 'error' field
// contains wantSubstr.

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

// requireManagedProject skips the test when running against an external
// editor (GODAI_TEST_PORT), where we don't control the project directory.

func requireManagedProject(t *testing.T) {
	t.Helper()
	if projectDir == "" {
		t.Skip("requires a test-managed project directory (not GODAI_TEST_PORT mode)")
	}
}

// runEditorScript runs GDScript code in the editor via the
// execute_editor_script tool, asserting that it succeeds.

func runEditorScript(t *testing.T, code string) map[string]any {
	t.Helper()
	return callToolOK(t, "execute_editor_script", map[string]any{
		"code": code,
	})
}

// closeAllScenes closes any scenes that are open in the editor.

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

// setupSceneWithChild creates and opens a fresh scene containing a single
// "MyChild" node, for tests that operate on existing nodes.

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

// readProjectFile reads a file from the test project directory, or returns ""
// when the project directory isn't available (GODAI_TEST_PORT mode).

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
