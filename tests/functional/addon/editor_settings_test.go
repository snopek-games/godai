package addon

import (
	"testing"

	"github.com/matryer/is"
)

// A stable editor setting with an integer value, used as the get/set target.
const editorSettingName = "text_editor/behavior/indent/size"

func TestEditorSettings(t *testing.T) {
	t.Run("get_specific", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_editor_settings", map[string]any{
			"names": []string{editorSettingName},
		})
		settings, _ := structured["settings"].(map[string]any)
		is.Equal(len(settings), 1)
		value, ok := settings[editorSettingName].(string)
		is.True(ok)
		is.True(value != "")
	})

	t.Run("get_nonexistent", func(t *testing.T) {
		callToolErr(t, "get_editor_settings", map[string]any{
			"names": []string{"no/such/editor/setting"},
		}, "No such setting")
	})

	t.Run("get_all_include_defaults", func(t *testing.T) {
		is := is.New(t)

		modifiedOnly := callToolOK(t, "get_editor_settings", nil)
		modified, _ := modifiedOnly["settings"].(map[string]any)

		all := callToolOK(t, "get_editor_settings", map[string]any{
			"include_defaults": true,
		})
		allSettings, _ := all["settings"].(map[string]any)

		is.True(len(allSettings) > len(modified))
		_, ok := allSettings[editorSettingName]
		is.True(ok)
	})

	t.Run("set_roundtrip", func(t *testing.T) {
		is := is.New(t)

		// Restore the original so an external editor's global settings aren't polluted.
		before := callToolOK(t, "get_editor_settings", map[string]any{
			"names": []string{editorSettingName},
		})
		original := before["settings"].(map[string]any)[editorSettingName].(string)
		t.Cleanup(func() {
			callToolOK(t, "set_editor_settings", map[string]any{
				"settings": map[string]any{editorSettingName: original},
			})
		})

		newValue := "20"
		if original == newValue {
			newValue = "18"
		}

		structured := callToolOK(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{editorSettingName: newValue},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_editor_settings", map[string]any{
			"names": []string{editorSettingName},
		})
		is.Equal(after["settings"].(map[string]any)[editorSettingName], newValue)

		modified := callToolOK(t, "get_editor_settings", nil)
		_, ok := modified["settings"].(map[string]any)[editorSettingName]
		is.True(ok)
	})

	t.Run("set_invalid_typed_value", func(t *testing.T) {
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{
				editorSettingName: "not_a_number",
			},
		}, "Nothing was changed")
	})

	t.Run("set_empty", func(t *testing.T) {
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{},
		}, "'settings' is required")
	})
}
