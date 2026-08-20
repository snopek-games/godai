package addon

import (
	"fmt"
	"strings"
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

		for name := range allSettings {
			if strings.HasPrefix(name, "_") {
				t.Errorf("listing includes the editor-internal setting %q", name)
			}
		}
	})

	t.Run("get_underscored_by_name", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_editor_settings", map[string]any{
			"names": []string{"_editor_settings_advanced_mode"},
		})
		settings, _ := structured["settings"].(map[string]any)
		is.Equal(len(settings), 1)
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
		}, "Cannot parse")
	})

	t.Run("set_enum_rejects_invalid_value", func(t *testing.T) {
		// "text_editor/theme/color_theme" is a string setting with an enum
		// hint that always includes "Default".
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{
				"text_editor/theme/color_theme": "Defaul",
			},
		}, "did you mean 'Default'")
	})

	t.Run("set_unknown_requires_create_missing", func(t *testing.T) {
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{
				"no/such/editor/setting": "1",
			},
		}, `pass "create_missing": true`)

		// Unlike project settings, a dotted name is not a feature-tag
		// override of the base setting.
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{
				editorSettingName + ".web": "4",
			},
		}, `pass "create_missing": true`)
	})

	t.Run("set_restart_setting_warns", func(t *testing.T) {
		is := is.New(t)

		// Which settings are flagged restart-if-changed varies by build and
		// platform, so find a bool one to toggle.
		out := runEditorScript(t, `var es := EditorInterface.get_editor_settings()
for prop in es.get_property_list():
	if prop['usage'] & PROPERTY_USAGE_RESTART_IF_CHANGED and prop['type'] == TYPE_BOOL and es.has_setting(prop['name']):
		print("RESTART_SETTING:", prop['name'], "=", es.get_setting(prop['name']))
		return OK
return OK`)

		var name, original string
		lines, _ := out["output"].([]any)
		for _, line := range asStrings(lines) {
			if rest, found := strings.CutPrefix(line, "RESTART_SETTING:"); found {
				name, original, _ = strings.Cut(rest, "=")
			}
		}
		if name == "" {
			t.Skip("no restart-flagged bool editor settings in this build")
		}
		t.Cleanup(func() {
			callToolOK(t, "set_editor_settings", map[string]any{
				"settings": map[string]any{name: original},
			})
		})

		newValue := "true"
		if original == "true" {
			newValue = "false"
		}

		structured := callToolOK(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{name: newValue},
		})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.True(anyLineContains(warnings, "restart_editor"))
	})

	t.Run("set_create_missing", func(t *testing.T) {
		is := is.New(t)

		const createdSetting = "godai_test/created_by_test"
		t.Cleanup(func() {
			runEditorScript(t, fmt.Sprintf(`EditorInterface.get_editor_settings().erase(%q)
return OK`, createdSetting))
		})

		structured := callToolOK(t, "set_editor_settings", map[string]any{
			"settings":       map[string]any{createdSetting: "hello"},
			"create_missing": true,
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_editor_settings", map[string]any{
			"names": []string{createdSetting},
		})
		is.Equal(after["settings"].(map[string]any)[createdSetting], "hello")
	})

	t.Run("set_empty", func(t *testing.T) {
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{},
		}, "'settings' is required")
	})
}

// Godai's own settings hold the Anthropic API key and the persisted tool
// approvals, so the AI can neither read the key nor grant itself permissions the
// user never approved.
func TestGodaiEditorSettingsAreHidden(t *testing.T) {
	const apiKeySetting = "godai/api/anthropic_key"

	t.Run("get_by_name_is_rejected", func(t *testing.T) {
		callToolErr(t, "get_editor_settings", map[string]any{
			"names": []string{apiKeySetting},
		}, "is a Godai setting")
	})

	t.Run("set_is_rejected", func(t *testing.T) {
		is := is.New(t)

		setToolSetting(t, allowedToolsSetting, "")
		callToolErr(t, "set_editor_settings", map[string]any{
			"settings": map[string]any{allowedToolsSetting: approvalTool},
		}, "is a Godai setting")

		// Rejected before anything was written.
		is.Equal(getToolSetting(t, allowedToolsSetting), "")
	})

	t.Run("omitted_when_listing_everything", func(t *testing.T) {
		is := is.New(t)

		// Make sure at least one Godai setting differs from its default, so it
		// would show up in both listings if it weren't filtered out.
		setToolSetting(t, allowedToolsSetting, approvalTool)
		t.Cleanup(func() { setToolSetting(t, allowedToolsSetting, "") })

		for _, args := range []map[string]any{nil, {"include_defaults": true}} {
			structured := callToolOK(t, "get_editor_settings", args)
			settings, _ := structured["settings"].(map[string]any)
			for name := range settings {
				is.True(!strings.HasPrefix(name, "godai/"))
			}
		}

		// The modified-only listing can legitimately be empty once the Godai
		// settings are filtered out, so prove the filter isn't just hiding an
		// empty result.
		all := callToolOK(t, "get_editor_settings", map[string]any{"include_defaults": true})
		is.True(len(all["settings"].(map[string]any)) > 0)
	})
}
