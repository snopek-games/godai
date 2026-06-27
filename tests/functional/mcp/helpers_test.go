package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"godai/tests/functional/internal/harness"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	// Generous: opening a project spawns and imports a real editor.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// callTool calls a tool on the main server.
func callTool(t *testing.T, name string, args map[string]any) *ToolCallResult {
	t.Helper()
	return callToolWith(t, client, name, args)
}

// callToolWith calls a tool on a specific server.
func callToolWith(t *testing.T, c *harness.MCPClient, name string, args map[string]any) *ToolCallResult {
	t.Helper()
	result, err := c.CallTool(testContext(t), name, args)
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return result
}

// callToolOK calls a tool on the main server, asserts it succeeded, and returns
// the structured content.
func callToolOK(t *testing.T, name string, args map[string]any) map[string]any {
	t.Helper()
	return callToolOKWith(t, client, name, args)
}

// callToolOKWith is callToolOK against a specific server.
func callToolOKWith(t *testing.T, c *harness.MCPClient, name string, args map[string]any) map[string]any {
	t.Helper()
	result := callToolWith(t, c, name, args)
	if result.IsError {
		t.Fatalf("tools/call %s returned an error: %s", name, result.Text())
	}
	structured, err := result.Structured()
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return structured
}

// callToolErr calls a tool, asserts it failed, and that the error text contains
// wantSubstr.
func callToolErr(t *testing.T, name string, args map[string]any, wantSubstr string) {
	t.Helper()
	result := callTool(t, name, args)
	if !result.IsError {
		t.Fatalf("tools/call %s should have returned an error, got: %s", name, result.Text())
	}
	if !strings.Contains(result.Text(), wantSubstr) {
		t.Fatalf("tools/call %s error %q does not contain %q", name, result.Text(), wantSubstr)
	}
}

// ensureProjectOpen makes sure the editor for the test project is running and
// connected, by calling open_godot_project. The first call spawns the editor
// (the real spawn path); later calls short-circuit in the server because the
// editor is already connected.
func ensureProjectOpen(t *testing.T) {
	t.Helper()
	result := callTool(t, "open_godot_project", map[string]any{
		"project_path": projectPath,
	})
	if result.IsError {
		t.Fatalf("open_godot_project: %s", result.Text())
	}
}
