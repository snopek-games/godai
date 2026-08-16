package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"

	"github.com/matryer/is"
)

// Editor settings holding the tools the user has approved or rejected for good.
const (
	allowedToolsSetting = "godai/tools/allowed"
	deniedToolsSetting  = "godai/tools/denied"
)

// A tool that needs approval but is harmless to actually run: stopping a project
// that isn't playing does nothing.
const approvalTool = "stop_project"

// A tool that never needs approval, because it's annotated read-only, and that
// answers whether or not a scene is open.
const readOnlyTool = "get_current_project"

func denialMessage(tool string) string {
	return "denied permission to use the '" + tool + "' tool"
}

// The rest of the suite runs against an editor with GODAI_AUTO_APPROVE_TOOLS
// set, since a headless editor has no one to answer the approval dialog. This
// brings up a second editor without it, which is the only way to reach the
// denial path from a test.
func startEditorWithoutAutoApproval(t *testing.T) *harness.MCPClient {
	t.Helper()

	godotBin, err := harness.FindGodot()
	if err != nil {
		t.Skipf("functional tests: %v", err)
	}

	dir, err := os.MkdirTemp("", "godai-approval-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}

	if err := harness.CreateTestProject(dir, harness.ProjectOptions{
		Name:            "Godai Approval Test",
		InstallAddon:    true,
		SkipSecretCheck: true,
	}); err != nil {
		t.Fatalf("creating test project: %v", err)
	}

	cmd, logPath, err := harness.LaunchEditor(godotBin, dir, harness.EditorOptions{
		Transport:          "http",
		Verbose:            os.Getenv("GODAI_TEST_VERBOSE") != "",
		NoAutoApproveTools: true,
		DisableShutdown:    true,
	})
	if err != nil {
		t.Fatalf("launching editor: %v", err)
	}
	t.Cleanup(func() {
		harness.StopEditor(cmd)
		if t.Failed() {
			t.Logf("test project kept at %s (editor log: %s)", dir, logPath)
			return
		}
		os.RemoveAll(dir)
	})

	// The editor advertises the port it bound once the addon is up, and the
	// first launch has to import the whole project first, which can be slow.
	instancesDir := filepath.Join(dir, ".xdg", "XDG_CACHE_HOME", "godai", "instances")
	port, err := harness.WaitForInstancePort(instancesDir, harness.OpenTimeout(180*time.Second))
	if err != nil {
		t.Fatalf("%v (editor log: %s)", err, logPath)
	}

	c := harness.NewHTTPClient("http://127.0.0.1:" + strconv.Itoa(port))

	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		reqCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, lastErr = c.Initialize(reqCtx)
		cancel()
		if lastErr == nil {
			return c
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("editor never became ready: %v", lastErr)
	return nil
}

func callToolOn(t *testing.T, c *harness.MCPClient, name string, args map[string]any) *ToolCallResult {
	t.Helper()
	result, err := c.CallTool(testContext(t), name, args)
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return result
}

// A denied tool comes back as a tool error rather than a JSON-RPC error, so the
// model sees it and can carry on with something else. It's shaped like any other
// tool failure, structured content included.
func requireDenied(t *testing.T, result *ToolCallResult, tool string) {
	t.Helper()
	if !result.IsError {
		t.Fatalf("tools/call %s should have been denied, got: %s", tool, result.Text())
	}
	structured, err := result.Structured()
	if err != nil {
		t.Fatalf("tools/call %s: %v", tool, err)
	}
	errs, _ := structured["errors"].([]any)
	errMsg := strings.Join(asStrings(errs), "\n")
	if !strings.Contains(errMsg, denialMessage(tool)) {
		t.Fatalf("tools/call %s errors %q do not report a denial", tool, errMsg)
	}
}

// Without auto-approval there's no way to approve anything headless, so every
// tool that needs approval has to be denied - and the read-only ones still have
// to work, since they never ask in the first place.
func TestToolApprovalWithoutAutoApprove(t *testing.T) {
	c := startEditorWithoutAutoApproval(t)

	t.Run("read_only_tool_runs", func(t *testing.T) {
		result := callToolOn(t, c, readOnlyTool, nil)
		if result.IsError {
			t.Fatalf("tools/call %s should not need approval, got: %s", readOnlyTool, result.Text())
		}
	})

	t.Run("tool_needing_approval_is_denied", func(t *testing.T) {
		requireDenied(t, callToolOn(t, c, approvalTool, nil), approvalTool)
	})

	// Whoever launched a headless editor has to be able to shut it down again,
	// even with nothing to approve the lifecycle tools. The harness sets
	// GODAI_DISABLE_CLOSE, so they stop short of really doing it.
	t.Run("editor_lifecycle_tools_are_allowed", func(t *testing.T) {
		for _, tool := range []string{"restart_editor", "close_editor"} {
			result := callToolOn(t, c, tool, map[string]any{"skip_save": true})
			if result.IsError {
				t.Fatalf("tools/call %s should not need approval headless, got: %s", tool, result.Text())
			}
		}
	})

	t.Run("denied_tool_does_not_run", func(t *testing.T) {
		is := is.New(t)

		const scriptPath = "res://never_created.gd"
		requireDenied(t, callToolOn(t, c, "create_script", map[string]any{
			"file_path": scriptPath,
		}), "create_script")

		// read_script is read-only, so it still answers.
		result := callToolOn(t, c, "read_script", map[string]any{"file_path": scriptPath})
		is.True(result.IsError)
		is.True(strings.Contains(result.Text(), "doesn't exist"))
	})
}

// The persisted allow/deny lists are the one part of the approval system a test
// can drive, since they live in editor settings rather than the dialog.
func TestPersistedToolDecisions(t *testing.T) {
	// Restore whatever the editor had, so the tools stay usable for other tests.
	restoreToolSetting(t, deniedToolsSetting)
	restoreToolSetting(t, allowedToolsSetting)

	t.Run("denied_list_beats_auto_approve", func(t *testing.T) {
		is := is.New(t)

		// Auto-approval is on for this editor, so the tool runs to begin with.
		is.Equal(callToolOK(t, approvalTool, nil)["success"], true)

		setToolSetting(t, deniedToolsSetting, approvalTool)
		requireDenied(t, callTool(t, approvalTool, nil), approvalTool)

		setToolSetting(t, deniedToolsSetting, "")
		is.Equal(callToolOK(t, approvalTool, nil)["success"], true)
	})

	t.Run("denied_list_is_one_tool_per_line", func(t *testing.T) {
		setToolSetting(t, deniedToolsSetting, "some_other_tool\n"+approvalTool+"\n")
		requireDenied(t, callTool(t, approvalTool, nil), approvalTool)

		setToolSetting(t, deniedToolsSetting, "")
	})

	t.Run("read_only_tools_ignore_the_denied_list", func(t *testing.T) {
		// Read-only tools never ask for approval, so a decision about them has
		// nothing to apply to.
		setToolSetting(t, deniedToolsSetting, readOnlyTool)
		callToolOK(t, readOnlyTool, nil)

		setToolSetting(t, deniedToolsSetting, "")
	})

	t.Run("denied_beats_allowed_for_the_same_tool", func(t *testing.T) {
		// A tool named in both lists is denied: the user had to go out of their
		// way to reject it.
		setToolSetting(t, allowedToolsSetting, approvalTool)
		setToolSetting(t, deniedToolsSetting, approvalTool)
		requireDenied(t, callTool(t, approvalTool, nil), approvalTool)

		setToolSetting(t, allowedToolsSetting, "")
		setToolSetting(t, deniedToolsSetting, "")
	})
}

// The editor settings tools deliberately refuse to touch anything under
// 'godai/', so these go through execute_editor_script instead.
func setToolSetting(t *testing.T, name, value string) {
	t.Helper()
	runEditorScript(t, fmt.Sprintf(
		"EditorInterface.get_editor_settings().set_setting(%q, %q)", name, value))
}

func getToolSetting(t *testing.T, name string) string {
	t.Helper()

	result := runEditorScript(t, fmt.Sprintf(
		"print(EditorInterface.get_editor_settings().get_setting(%q))", name))

	output, _ := result["output"].([]any)
	// The value is a newline-separated list, so print() spreads it over several
	// output lines.
	lines := make([]string, 0, len(output))
	for _, line := range output {
		s, _ := line.(string)
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}

func restoreToolSetting(t *testing.T, name string) {
	t.Helper()

	original := getToolSetting(t, name)
	t.Cleanup(func() {
		setToolSetting(t, name, original)
	})
}
