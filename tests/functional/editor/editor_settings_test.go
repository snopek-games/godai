package editor

import (
	"testing"

	"github.com/matryer/is"
)

// A stable, long-standing editor setting with an integer value, used as a
// well-known target for the get/set tests.
const editorSettingName = "interface/editor/code_font_size"

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

		// Including defaults yields many more settings than the modified-only set.
		is.True(len(allSettings) > len(modified))
		// The well-known setting exists in the full set.
		_, ok := allSettings[editorSettingName]
		is.True(ok)
	})

	t.Run("set_roundtrip", func(t *testing.T) {
		is := is.New(t)

		// Capture the original value so the editor's global settings are left
		// untouched (these tests can also run against an external editor).
		before := callToolOK(t, "get_editor_settings", map[string]any{
			"names": []string{editorSettingName},
		})
		original := before["settings"].(map[string]any)[editorSettingName].(string)
		t.Cleanup(func() {
			callToolOK(t, "set_editor_settings", map[string]any{
				"settings": map[string]any{editorSettingName: original},
			})
		})

		// Pick a new value distinct from the original.
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

		// Having changed it from its default, it now appears in the
		// modified-only listing.
		modified := callToolOK(t, "get_editor_settings", nil)
		_, ok := modified["settings"].(map[string]any)[editorSettingName]
		is.True(ok)
	})

	t.Run("set_invalid_typed_value", func(t *testing.T) {
		// The value can't be parsed as the existing setting's (int) type, so
		// it's rejected and nothing is changed.
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
