package addon

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// The harness sets GODAI_DISABLE_CLOSE, so close_editor runs end-to-end
// (scenes are saved) but stops short of shutting down the editor it manages.
func TestCloseEditor(t *testing.T) {
	// Without GODAI_DISABLE_CLOSE (GODAI_TEST_PORT mode) this would really close the user's editor.
	requireManagedProject(t)

	t.Run("save_and_close", func(t *testing.T) {
		is := is.New(t)

		setupSceneWithChild(t, "res://scenes/close_save_test.tscn")

		// 'skip_save' defaults to false, so changes are saved.
		structured := callToolOK(t, "close_editor", nil)
		is.Equal(structured["success"], true)

		// Let the deferred close step (which saves all scenes) run.
		settleEditor(t)

		content := readProjectFile(t, "scenes/close_save_test.tscn")
		is.True(strings.Contains(content, "MyChild"))

		callToolOK(t, "get_current_project", nil)
	})

	t.Run("close_without_saving", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "close_editor", map[string]any{
			"skip_save": true,
		})
		is.Equal(structured["success"], true)

		callToolOK(t, "get_current_project", nil)
	})

	t.Run("close_stops_running_game", func(t *testing.T) {
		is := is.New(t)

		startGameScene(t, "res://scenes/close_running_game.tscn")

		structured := callToolOK(t, "close_editor", map[string]any{
			"skip_save": true,
		})
		is.Equal(structured["success"], true)

		settleEditor(t)
		is.True(!gameIsPlaying(t))
	})
}
