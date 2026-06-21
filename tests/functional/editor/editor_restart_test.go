package editor

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// TestRestartEditor exercises the restart_editor tool. The harness sets
// GODAI_DISABLE_RESTART, so the tool runs end-to-end (prompt is skipped while
// headless, scenes are saved) but stops short of actually relaunching the
// editor this harness manages.
func TestRestartEditor(t *testing.T) {
	// In GODAI_TEST_PORT mode the env var isn't set, so this would really
	// restart the user's editor.
	requireManagedProject(t)

	t.Run("save_and_restart", func(t *testing.T) {
		is := is.New(t)

		// An open scene with an unsaved change.
		setupSceneWithChild(t, "res://scenes/restart_save_test.tscn")

		// 'skip_save' defaults to false, so changes are saved.
		structured := callToolOK(t, "restart_editor", nil)
		is.Equal(structured["success"], true)

		// Let the deferred restart step (which saves all scenes) run.
		settleEditor(t)

		// The unsaved child should have been written to disk by the save step.
		content := readProjectFile(t, "scenes/restart_save_test.tscn")
		is.True(strings.Contains(content, "MyChild"))

		// The editor is still alive (the real restart was disabled).
		callToolOK(t, "get_current_project", nil)
	})

	t.Run("restart_without_saving", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "restart_editor", map[string]any{
			"skip_save": true,
		})
		is.Equal(structured["success"], true)

		// Still responsive afterwards.
		callToolOK(t, "get_current_project", nil)
	})
}
