package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestCreateResource(t *testing.T) {
	t.Run("missing_file_path", func(t *testing.T) {
		callToolErr(t, "create_resource", map[string]any{
			"resource_type": "Resource",
			"properties":    map[string]any{},
		}, "'file_path' is required")
	})

	t.Run("missing_resource_type", func(t *testing.T) {
		callToolErr(t, "create_resource", map[string]any{
			"file_path":  "res://resources/nope.tres",
			"properties": map[string]any{},
		}, "'resource_type' is required")
	})

	t.Run("unknown_resource_type", func(t *testing.T) {
		callToolErr(t, "create_resource", map[string]any{
			"file_path":     "res://resources/nope.tres",
			"resource_type": "NoSuchResourceType",
			"properties":    map[string]any{},
		}, "Unknown 'resource_type'")
	})

	t.Run("not_a_resource_type", func(t *testing.T) {
		callToolErr(t, "create_resource", map[string]any{
			"file_path":     "res://resources/nope.tres",
			"resource_type": "Node",
			"properties":    map[string]any{},
		}, "'Node' is not a Resource type")
	})

	t.Run("builtin_type_with_properties", func(t *testing.T) {
		requireManagedProject(t)
		is := is.New(t)

		// In a new directory, to also cover the make_dir code path.
		structured := callToolOK(t, "create_resource", map[string]any{
			"file_path":     "resources/label_settings.tres",
			"resource_type": "LabelSettings",
			"properties": map[string]any{
				"font_size": "32",
			},
		})
		is.Equal(structured["success"], true)

		data, err := os.ReadFile(filepath.Join(projectDir, "resources", "label_settings.tres"))
		is.NoErr(err)
		is.True(strings.Contains(string(data), `type="LabelSettings"`))
		is.True(strings.Contains(string(data), "font_size = 32"))
	})

	t.Run("already_exists", func(t *testing.T) {
		requireManagedProject(t)

		callToolErr(t, "create_resource", map[string]any{
			"file_path":     "res://resources/label_settings.tres",
			"resource_type": "LabelSettings",
			"properties":    map[string]any{},
		}, "already exists")
	})

	t.Run("invalid_property_still_creates_resource", func(t *testing.T) {
		requireManagedProject(t)
		is := is.New(t)

		// The resource is created and saved anyway, so a property problem is a
		// warning: an error (success = false) could push an agent into retrying
		// the whole call, which would fail on the already-existing file.
		structured := callToolOK(t, "create_resource", map[string]any{
			"file_path":     "res://resources/still_created.tres",
			"resource_type": "LabelSettings",
			"properties": map[string]any{
				"font_size": "garbage(",
			},
		})
		is.Equal(structured["success"], true)
		_, hasErrors := structured["errors"]
		is.True(!hasErrors) // the bad property is a warning, not an error
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], `font_size: "garbage" is not a variant type`))

		_, err := os.Stat(filepath.Join(projectDir, "resources", "still_created.tres"))
		is.NoErr(err)
	})

	t.Run("global_script_class", func(t *testing.T) {
		requireManagedProject(t)
		is := is.New(t)

		structured := callToolOK(t, "create_resource", map[string]any{
			"file_path":     "res://resources/custom.tres",
			"resource_type": "GodaiTestCustomResource",
			"properties": map[string]any{
				"title":  "Hello",
				"amount": "5",
			},
		})
		is.Equal(structured["success"], true)

		data, err := os.ReadFile(filepath.Join(projectDir, "resources", "custom.tres"))
		is.NoErr(err)
		content := string(data)
		is.True(strings.Contains(content, "custom_resource.gd"))
		is.True(strings.Contains(content, `title = "Hello"`))
		is.True(strings.Contains(content, "amount = 5"))
	})
}

func TestResourceProperties(t *testing.T) {
	is := is.New(t)

	const materialPath = "res://resources/rp_material.tres"
	callToolOK(t, "create_resource", map[string]any{
		"file_path":     materialPath,
		"resource_type": "StandardMaterial3D",
		"properties": map[string]any{
			"albedo_color": "Color(1, 0, 0, 1)",
		},
	})

	readMaterialFile := func(t *testing.T) string {
		t.Helper()
		if projectDir == "" {
			return ""
		}
		data, err := os.ReadFile(filepath.Join(projectDir, "resources", "rp_material.tres"))
		is.NoErr(err)
		return string(data)
	}

	t.Run("get", func(t *testing.T) {
		is := is.New(t)

		props := getResourceProps(t, map[string]any{
			"file_path": materialPath,
		})
		is.Equal(props["albedo_color"], "Color(1, 0, 0, 1)")

		// "roughness" is still at its default, so it's left out by default.
		_, hasRoughness := props["roughness"]
		is.True(!hasRoughness)
	})

	t.Run("get_include_defaults", func(t *testing.T) {
		is := is.New(t)

		props := getResourceProps(t, map[string]any{
			"file_path":        materialPath,
			"include_defaults": true,
		})
		_, hasRoughness := props["roughness"]
		is.True(hasRoughness)
	})

	t.Run("get_specific_properties", func(t *testing.T) {
		is := is.New(t)

		structured := getResourceProps(t, map[string]any{
			"file_path":  materialPath,
			"properties": []string{"albedo_color", "albedo_color:r"},
		})
		is.Equal(len(structured), 2)
		is.Equal(structured["albedo_color"], "Color(1, 0, 0, 1)")
		is.Equal(structured["albedo_color:r"], "1.0")
	})

	t.Run("get_nonexistent", func(t *testing.T) {
		callToolErr(t, "get_resource_properties", map[string]any{
			"file_path": "res://no_such_resource.tres",
		}, "doesn't exist")
	})

	t.Run("get_colon_in_file_path", func(t *testing.T) {
		callToolErr(t, "get_resource_properties", map[string]any{
			"file_path": materialPath + ":albedo_color",
		}, "must point at just the file")
	})

	t.Run("get_bad_property_path", func(t *testing.T) {
		is := is.New(t)

		structured := getResourceProps(t, map[string]any{
			"file_path":  materialPath,
			"properties": []string{"no_such_prop"},
		})
		errProps, _ := structured["no_such_prop"].(map[string]any)
		errMsg, _ := errProps["error"].(string)
		is.True(strings.Contains(errMsg, "has no property named 'no_such_prop'"))
	})

	t.Run("set_and_persist", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_resource_properties", map[string]any{
			"action":    "Change material",
			"file_path": materialPath,
			"properties": map[string]any{
				"albedo_color": "Color(0, 1, 0, 1)",
				"metallic":     "0.5",
			},
		})
		is.Equal(structured["success"], true)

		props := getResourceProps(t, map[string]any{
			"file_path": materialPath,
		})
		is.Equal(props["albedo_color"], "Color(0, 1, 0, 1)")

		if content := readMaterialFile(t); content != "" {
			is.True(strings.Contains(content, "albedo_color = Color(0, 1, 0, 1)"))
			is.True(strings.Contains(content, "metallic = 0.5"))
		}
	})

	t.Run("set_sub_property", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "set_resource_properties", map[string]any{
			"action":    "Tweak color",
			"file_path": materialPath,
			"properties": map[string]any{
				"albedo_color:b": "1.0",
			},
		})

		structured := getResourceProps(t, map[string]any{
			"file_path":  materialPath,
			"properties": []string{"albedo_color"},
		})
		is.Equal(structured["albedo_color"], "Color(0, 1, 1, 1)")
	})

	t.Run("undo_restores_and_saves", func(t *testing.T) {
		is := is.New(t)

		// Edits to a resource that isn't part of the edited scene land in the global history.
		runEditorScript(t, `EditorInterface.get_editor_undo_redo().get_history_undo_redo(EditorUndoRedoManager.GLOBAL_HISTORY).undo()
return OK`)

		structured := getResourceProps(t, map[string]any{
			"file_path":  materialPath,
			"properties": []string{"albedo_color"},
		})
		is.Equal(structured["albedo_color"], "Color(0, 1, 0, 1)") // the sub-property edit was undone

		if content := readMaterialFile(t); content != "" {
			is.True(strings.Contains(content, "albedo_color = Color(0, 1, 0, 1)"))
		}
	})

	t.Run("get_embedded_resource_expansion", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "set_resource_properties", map[string]any{
			"action":    "Add albedo texture",
			"file_path": materialPath,
			"properties": map[string]any{
				"albedo_texture": `Object(GradientTexture1D,"width":128)`,
			},
		})

		props := getResourceProps(t, map[string]any{
			"file_path": materialPath,
		})
		is.Equal(props["albedo_texture"], "Object(GradientTexture1D)") // embedded resources are summarized by type

		structured := getResourceProps(t, map[string]any{
			"file_path":  materialPath,
			"properties": []string{"albedo_texture", "albedo_texture:width"},
		})
		texProps, _ := structured["albedo_texture"].(map[string]any)
		is.Equal(texProps["width"], "128")
		is.Equal(structured["albedo_texture:width"], "128")
	})

	t.Run("set_nonexistent", func(t *testing.T) {
		callToolErr(t, "set_resource_properties", map[string]any{
			"action":     "Set on missing resource",
			"file_path":  "res://no_such_resource.tres",
			"properties": map[string]any{"metallic": "0.5"},
		}, "doesn't exist")
	})

	t.Run("set_colon_in_file_path", func(t *testing.T) {
		callToolErr(t, "set_resource_properties", map[string]any{
			"action":     "Set via property path",
			"file_path":  materialPath + ":albedo_color",
			"properties": map[string]any{"b": "1.0"},
		}, "must point at just the file")
	})

	t.Run("set_one_bad_value_still_sets_others", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_resource_properties", map[string]any{
			"action":    "Change material",
			"file_path": materialPath,
			"properties": map[string]any{
				"metallic":  "0.75",
				"roughness": "garbage(",
			},
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], `roughness: "garbage" is not a variant type`))

		props := getResourceProps(t, map[string]any{
			"file_path":  materialPath,
			"properties": []string{"metallic"},
		})
		is.Equal(props["metallic"], "0.75")
	})

	t.Run("set_unknown_property", func(t *testing.T) {
		is := is.New(t)

		// An unknown property is attempted anyway (a script could handle it
		// dynamically), so the failure comes from verification.
		structured := callToolOK(t, "set_resource_properties", map[string]any{
			"action":     "Set unknown property",
			"file_path":  materialPath,
			"properties": map[string]any{"no_such_prop": "1"},
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "has no property named 'no_such_prop'"))
	})

	t.Run("set_not_a_saveable_file", func(t *testing.T) {
		callToolErr(t, "set_resource_properties", map[string]any{
			"action":     "Set on non-tres file",
			"file_path":  "res://icon.svg",
			"properties": map[string]any{"resource_name": "nope"},
		}, "must be a .tres or .res file")
	})

	t.Run("set_missing_action", func(t *testing.T) {
		callToolErr(t, "set_resource_properties", map[string]any{
			"file_path":  materialPath,
			"properties": map[string]any{"metallic": "0.5"},
		}, "'action' is required")
	})
}

// A Resource script with setters that clamp, reject, and print, so
// set_resource_properties verification can be exercised the same way as the
// node tools.
const verifyResourceFixtureScript = `@tool
extends Resource

var plain_value := 0.0

var clamped_value := 0.0:
	set(v):
		clamped_value = clampf(v, 0.0, 10.0)

var rejecting_value := 1.0:
	set(v):
		if v < 0.0:
			push_error("rejecting_value cannot be negative")
			return
		rejecting_value = v
`

func TestSetResourcePropertiesVerification(t *testing.T) {
	writeProjectFileFromEditor(t, "res://scripts/verify_res_fixture.gd", verifyResourceFixtureScript)
	writeProjectFileFromEditor(t, "res://resources/verify_res_fixture.tres", `[gd_resource type="Resource" load_steps=2 format=3]

[ext_resource type="Script" path="res://scripts/verify_res_fixture.gd" id="1_fix"]

[resource]
script = ExtResource("1_fix")
`)
	fixturePath := "res://resources/verify_res_fixture.tres"

	setProps := func(t *testing.T, props map[string]any) map[string]any {
		t.Helper()
		return callToolOK(t, "set_resource_properties", map[string]any{
			"action":     "Verification test",
			"file_path":  fixturePath,
			"properties": props,
		})
	}

	t.Run("plain_set", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"plain_value": "3.5"})
		is.Equal(structured["success"], true)
		_, hasErrors := structured["errors"]
		is.True(!hasErrors)
		_, hasWarnings := structured["warnings"]
		is.True(!hasWarnings)
	})

	t.Run("clamping_setter_warns", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"clamped_value": "50.0"})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "clamped_value"))
		is.True(strings.Contains(asStrings(warnings)[0], "only to 10.0")) // the warning shows what it clamped to
	})

	t.Run("rejecting_setter_errors_with_output", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"rejecting_value": "-5.0"})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "rejecting_value"))
		is.True(strings.Contains(asStrings(errs)[0], "read back unchanged"))

		output, _ := structured["output"].([]any)
		is.True(anyLineContains(output, "rejecting_value cannot be negative")) // the setter's push_error is surfaced
	})

	t.Run("indexing_into_non_object_hints_discovery", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"plain_value:0": "1.0"})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "the property may not exist"))
		is.True(strings.Contains(asStrings(errs)[0], `get_resource_properties with "include_defaults": true`))
	})
}

func TestOpenResource(t *testing.T) {
	t.Run("missing_file_path", func(t *testing.T) {
		callToolErr(t, "open_resource", map[string]any{}, "'file_path' is required")
	})

	t.Run("nonexistent", func(t *testing.T) {
		callToolErr(t, "open_resource", map[string]any{
			"file_path": "res://no_such_resource.tres",
		}, "doesn't exist")
	})

	t.Run("invalid_resource", func(t *testing.T) {
		runEditorScript(t, `var f := FileAccess.open("res://invalid_resource_test.tres", FileAccess.WRITE)
f.store_string("this is not a valid resource file")
f.close()
return OK`)

		callToolErr(t, "open_resource", map[string]any{
			"file_path": "res://invalid_resource_test.tres",
		}, "Failed to load resource")
	})

	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_resource", map[string]any{
			"file_path":     "res://resources/open_resource_test.tres",
			"resource_type": "LabelSettings",
			"properties":    map[string]any{},
		})

		// No res:// prefix, to also cover the prefixing code path.
		structured := callToolOK(t, "open_resource", map[string]any{
			"file_path": "resources/open_resource_test.tres",
		})
		is.Equal(structured["success"], true)

		runEditorScript(t, `var edited = EditorInterface.get_inspector().get_edited_object()
if edited is Resource and edited.resource_path == "res://resources/open_resource_test.tres":
	return OK
return FAILED`)
	})
}
