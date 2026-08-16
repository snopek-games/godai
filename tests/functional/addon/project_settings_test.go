package addon

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestProjectSettings(t *testing.T) {
	// These assertions depend on the test project's known project.godot and read it back from disk.
	requireManagedProject(t)

	t.Run("get_specific", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"application/config/name"},
		})
		settings, _ := structured["settings"].(map[string]any)
		is.Equal(len(settings), 1)
		is.Equal(settings["application/config/name"], projectName)
	})

	t.Run("get_specific_at_default", func(t *testing.T) {
		is := is.New(t)

		// Requesting a setting by name returns it even at its default value (unlike get-all).
		structured := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"display/window/size/viewport_height"},
		})
		settings, _ := structured["settings"].(map[string]any)
		_, ok := settings["display/window/size/viewport_height"]
		is.True(ok)
	})

	t.Run("get_nonexistent", func(t *testing.T) {
		callToolErr(t, "get_project_settings", map[string]any{
			"names": []string{"no/such/setting"},
		}, "No such setting")
	})

	t.Run("get_all_modified_only", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_project_settings", nil)
		settings, _ := structured["settings"].(map[string]any)

		is.Equal(settings["application/config/name"], projectName)

		_, hasDefault := settings["display/window/size/viewport_height"]
		is.True(!hasDefault)
	})

	t.Run("get_all_include_defaults", func(t *testing.T) {
		is := is.New(t)

		modifiedOnly := callToolOK(t, "get_project_settings", nil)
		modified, _ := modifiedOnly["settings"].(map[string]any)

		all := callToolOK(t, "get_project_settings", map[string]any{
			"include_defaults": true,
		})
		allSettings, _ := all["settings"].(map[string]any)

		is.True(len(allSettings) > len(modified))

		_, hasDefault := allSettings["display/window/size/viewport_height"]
		is.True(hasDefault)
	})

	t.Run("set_and_persist", func(t *testing.T) {
		is := is.New(t)

		// A value that isn't valid variant syntax is stored as a raw string; an int-looking value as an int.
		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"godai_test/example": "hello",
				"godai_test/number":  "42",
			},
			"create_missing": true,
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"godai_test/example", "godai_test/number"},
		})
		settings, _ := after["settings"].(map[string]any)
		is.Equal(settings["godai_test/example"], "hello")
		is.Equal(settings["godai_test/number"], "42")

		content := readProjectFile(t, "project.godot")
		is.True(strings.Contains(content, "[godai_test]"))
		is.True(strings.Contains(content, `example="hello"`))
		is.True(strings.Contains(content, "number=42"))
	})

	t.Run("set_existing_typed", func(t *testing.T) {
		is := is.New(t)

		// For an existing setting, the string value is decoded using its current type.
		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"display/window/size/viewport_width": "1280",
			},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"display/window/size/viewport_width"},
		})
		settings, _ := after["settings"].(map[string]any)
		is.Equal(settings["display/window/size/viewport_width"], "1280")
	})

	t.Run("set_invalid_typed_value", func(t *testing.T) {
		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"display/window/size/viewport_width": "not_a_number",
			},
		}, "Cannot parse")
	})

	t.Run("set_unknown_requires_create_missing", func(t *testing.T) {
		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"godai_test/never_created": "nope",
			},
		}, `pass "create_missing": true`)

		callToolErr(t, "get_project_settings", map[string]any{
			"names": []string{"godai_test/never_created"},
		}, "No such setting")
	})

	t.Run("set_misspelled_setting_suggests", func(t *testing.T) {
		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"display/window/size/viewport_widht": "1280",
			},
		}, "did you mean 'display/window/size/viewport_width'")
	})

	t.Run("set_feature_override", func(t *testing.T) {
		is := is.New(t)

		// An override of an existing setting doesn't need create_missing.
		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"application/config/name.web": projectName + " Web",
			},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"application/config/name.web"},
		})
		is.Equal(after["settings"].(map[string]any)["application/config/name.web"], projectName+" Web")
	})

	t.Run("set_feature_override_checks_base_enum", func(t *testing.T) {
		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"rendering/renderer/rendering_method.web": "compatibility",
			},
		}, "did you mean 'gl_compatibility'")
	})

	t.Run("set_enum_rejects_invalid_value", func(t *testing.T) {
		is := is.New(t)

		before := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"rendering/renderer/rendering_method"},
		})
		original := before["settings"].(map[string]any)["rendering/renderer/rendering_method"]

		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"rendering/renderer/rendering_method": "compatibility",
			},
		}, "did you mean 'gl_compatibility'")

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"rendering/renderer/rendering_method"},
		})
		is.Equal(after["settings"].(map[string]any)["rendering/renderer/rendering_method"], original)
	})

	t.Run("set_enum_valid_value", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"rendering/renderer/rendering_method": "gl_compatibility",
			},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"rendering/renderer/rendering_method"},
		})
		is.Equal(after["settings"].(map[string]any)["rendering/renderer/rendering_method"], "gl_compatibility")
	})

	t.Run("set_restart_setting_warns", func(t *testing.T) {
		is := is.New(t)

		// rendering/renderer/rendering_method is flagged restart-if-changed.
		before := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"rendering/renderer/rendering_method"},
		})
		newValue := "mobile"
		if before["settings"].(map[string]any)["rendering/renderer/rendering_method"] == "mobile" {
			newValue = "forward_plus"
		}

		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{"rendering/renderer/rendering_method": newValue},
		})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.True(anyLineContains(warnings, "restart_editor"))

		// Setting it to the value it already has doesn't need a restart.
		structured = callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{"rendering/renderer/rendering_method": newValue},
		})
		is.Equal(structured["success"], true)
		_, hasWarnings := structured["warnings"]
		is.True(!hasWarnings)
	})

	t.Run("set_independently", func(t *testing.T) {
		is := is.New(t)

		// One bad setting doesn't stop the others from being applied.
		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{
				"display/window/size/viewport_width": "not_a_number",
				"godai_test/independent":             "still_set",
			},
			"create_missing": true,
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "Cannot parse"))

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{"godai_test/independent"},
		})
		is.Equal(after["settings"].(map[string]any)["godai_test/independent"], "still_set")
	})

	t.Run("set_empty", func(t *testing.T) {
		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{},
		}, "'settings' is required")
	})
}
