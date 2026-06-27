package addon

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// The harness sets GODAI_DISABLE_CLOSE, so restart_editor runs end-to-end
// (scenes are saved) but stops short of relaunching the editor it manages.
func TestRestartEditor(t *testing.T) {
	// Without GODAI_DISABLE_CLOSE (GODAI_TEST_PORT mode) this would really restart the user's editor.
	requireManagedProject(t)

	t.Run("save_and_restart", func(t *testing.T) {
		is := is.New(t)

		setupSceneWithChild(t, "res://scenes/restart_save_test.tscn")

		// 'skip_save' defaults to false, so changes are saved.
		structured := callToolOK(t, "restart_editor", nil)
		is.Equal(structured["success"], true)

		// Let the deferred restart step (which saves all scenes) run.
		settleEditor(t)

		content := readProjectFile(t, "scenes/restart_save_test.tscn")
		is.True(strings.Contains(content, "MyChild"))

		callToolOK(t, "get_current_project", nil)
	})

	t.Run("restart_without_saving", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "restart_editor", map[string]any{
			"skip_save": true,
		})
		is.Equal(structured["success"], true)

		callToolOK(t, "get_current_project", nil)
	})
}
