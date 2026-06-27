package addon

import (
	"testing"

	"github.com/matryer/is"
)

func TestImportSettings(t *testing.T) {
	// These rely on the icon.svg fixture (and its generated .import file).
	requireManagedProject(t)

	const assetPath = "res://icon.svg"

	t.Run("get", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		importer, _ := structured["importer"].(string)
		is.True(importer != "")

		options, _ := structured["options"].(map[string]any)
		is.True(len(options) > 0)
		_, hasScale := options["svg/scale"]
		is.True(hasScale)
	})

	t.Run("get_nonexistent", func(t *testing.T) {
		callToolErr(t, "get_import_settings", map[string]any{
			"file_path": "res://no_such_asset.png",
		}, "doesn't exist")
	})

	t.Run("get_not_imported", func(t *testing.T) {
		callToolOK(t, "create_resource", map[string]any{
			"file_path":     "res://resources/imp_not_imported.tres",
			"resource_type": "LabelSettings",
			"properties":    map[string]any{},
		})
		callToolErr(t, "get_import_settings", map[string]any{
			"file_path": "res://resources/imp_not_imported.tres",
		}, "no import settings")
	})

	t.Run("set_and_reimport", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_import_settings", map[string]any{
			"file_path": assetPath,
			"options": map[string]any{
				"svg/scale": "2.0",
			},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		options, _ := after["options"].(map[string]any)
		is.Equal(options["svg/scale"], "2.0")
	})

	t.Run("set_nonexistent", func(t *testing.T) {
		callToolErr(t, "set_import_settings", map[string]any{
			"file_path": "res://no_such_asset.png",
			"options":   map[string]any{"svg/scale": "2.0"},
		}, "doesn't exist")
	})

	t.Run("reimport", func(t *testing.T) {
		is := is.New(t)
		structured := callToolOK(t, "reimport", map[string]any{
			"file_paths": []string{assetPath},
		})
		is.Equal(structured["success"], true)
	})

	t.Run("reimport_not_imported", func(t *testing.T) {
		callToolErr(t, "reimport", map[string]any{
			"file_paths": []string{"res://resources/imp_not_imported.tres"},
		}, "not an imported asset")
	})

	t.Run("reimport_nonexistent", func(t *testing.T) {
		callToolErr(t, "reimport", map[string]any{
			"file_paths": []string{"res://no_such_asset.png"},
		}, "doesn't exist")
	})
}
