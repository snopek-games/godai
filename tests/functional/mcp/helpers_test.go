package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	// Generous: opening a project spawns and imports a real editor.
	ctx, cancel := context.WithTimeout(context.Background(), harness.OpenTimeout(60*time.Second))
	t.Cleanup(cancel)
	return ctx
}

func callTool(t *testing.T, name string, args map[string]any) *ToolCallResult {
	t.Helper()
	return callToolWith(t, client, name, args)
}

func callToolWith(t *testing.T, c *harness.MCPClient, name string, args map[string]any) *ToolCallResult {
	t.Helper()
	result, err := c.CallTool(testContext(t), name, args)
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return result
}

func callToolOK(t *testing.T, name string, args map[string]any) map[string]any {
	t.Helper()
	return callToolOKWith(t, client, name, args)
}

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

func ensureProjectOpen(t *testing.T) {
	t.Helper()
	result := callTool(t, "open_godot_project", map[string]any{
		"project_path": projectPath,
	})
	if result.IsError {
		t.Fatalf("open_godot_project: %s", result.Text())
	}
}
