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

	t.Run("set_empty", func(t *testing.T) {
		callToolErr(t, "set_project_settings", map[string]any{
			"settings": map[string]any{},
		}, "'settings' is required")
	})
}
