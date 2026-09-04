package addon

import (
	"fmt"
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

	t.Run("get_regenerates_stripped_options", func(t *testing.T) {
		is := is.New(t)

		stripImportOption(t, assetPath+".import", "svg/scale")

		structured := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		options, _ := structured["options"].(map[string]any)
		_, hasScale := options["svg/scale"]
		is.True(hasScale) // get regenerated the stripped option
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

	t.Run("set_regenerates_stripped_options", func(t *testing.T) {
		is := is.New(t)

		stripImportOption(t, assetPath+".import", "svg/scale")

		structured := callToolOK(t, "set_import_settings", map[string]any{
			"file_path": assetPath,
			"options": map[string]any{
				"svg/scale": "1.5",
			},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		options, _ := after["options"].(map[string]any)
		is.Equal(options["svg/scale"], "1.5")
	})

	t.Run("set_unknown_option", func(t *testing.T) {
		is := is.New(t)

		callToolErr(t, "set_import_settings", map[string]any{
			"file_path": assetPath,
			"options": map[string]any{
				"svg/scale":          "4.0",
				"svg/no_such_option": "1",
			},
		}, "unknown import setting")

		after := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		options, _ := after["options"].(map[string]any)
		is.Equal(options["svg/scale"], "1.5") // the valid option in a rejected call must not be written
	})

	t.Run("set_not_imported", func(t *testing.T) {
		callToolErr(t, "set_import_settings", map[string]any{
			"file_path": "res://resources/imp_not_imported.tres",
			"options":   map[string]any{"svg/scale": "2.0"},
		}, "no import settings")
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

// Options must be validated against the new importer's params, not the old one's.
func TestImportSettingsChangeImporter(t *testing.T) {
	requireManagedProject(t)

	// The 'texture' importer has svg/scale but no slices/horizontal; the
	// '2d_array_texture' importer is the reverse.
	const assetPath = "res://fixtures/importer_change.svg"
	createImportedSVG(t, assetPath)

	t.Run("new_importer_option_accepted", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_import_settings", map[string]any{
			"file_path": assetPath,
			"importer":  "2d_array_texture",
			"options": map[string]any{
				"slices/horizontal": "4",
			},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		is.Equal(after["importer"], "2d_array_texture")
		options, _ := after["options"].(map[string]any)
		is.Equal(options["slices/horizontal"], "4")
		_, hasScale := options["svg/scale"]
		is.True(!hasScale) // the old importer's options must be gone
	})

	t.Run("switch_back_regenerates_options", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_import_settings", map[string]any{
			"file_path": assetPath,
			"importer":  "texture",
			"options":   map[string]any{},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		is.Equal(after["importer"], "texture")
		options, _ := after["options"].(map[string]any)
		_, hasScale := options["svg/scale"]
		is.True(hasScale) // the texture importer's options were regenerated
		_, hasSlices := options["slices/horizontal"]
		is.True(!hasSlices) // the array importer's options are gone
	})

	t.Run("old_importer_option_rejected", func(t *testing.T) {
		is := is.New(t)

		callToolErr(t, "set_import_settings", map[string]any{
			"file_path": assetPath,
			"importer":  "2d_array_texture",
			"options": map[string]any{
				"svg/scale": "2.0",
			},
		}, "unknown import setting")

		after := callToolOK(t, "get_import_settings", map[string]any{
			"file_path": assetPath,
		})
		is.Equal(after["importer"], "texture") // a rejected call must not change the importer
	})
}

// Writes a fresh SVG into the project so the test can modify it without affecting tests that share icon.svg.
func createImportedSVG(t *testing.T, path string) {
	t.Helper()
	writeProjectFileFromEditor(t, path, `<svg width="128" height="128" xmlns="http://www.w3.org/2000/svg"><rect width="128" height="128" fill="#478cbf"/></svg>`)
	runEditorScript(t, fmt.Sprintf(`var fs := EditorInterface.get_resource_filesystem()
fs.reimport_files(PackedStringArray([%q]))
for i in range(300):
	if FileAccess.file_exists(%q + ".import"):
		return OK
	await Engine.get_main_loop().process_frame
push_error("no .import file was generated for %%s" %% %q)
return FAILED`, path, path, path))
}

// Removes an option from a .import file on disk, leaving it in the incomplete
// state the tools are expected to repair by reimporting before reading/writing.
func stripImportOption(t *testing.T, importPath, key string) {
	t.Helper()
	runEditorScript(t, fmt.Sprintf(`var cfg := ConfigFile.new()
var err := cfg.load(%q)
if err != OK:
	push_error("loading %%s: %%s" %% [%q, error_string(err)])
	return FAILED
if not cfg.has_section_key("params", %q):
	push_error("expected %%s to have option %%s" %% [%q, %q])
	return FAILED
cfg.erase_section_key("params", %q)
err = cfg.save(%q)
if err != OK:
	push_error("saving %%s: %%s" %% [%q, error_string(err)])
	return FAILED
return OK`, importPath, importPath, key, importPath, key, key, importPath, importPath))
}
