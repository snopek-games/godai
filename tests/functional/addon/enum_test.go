package addon

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// A script with int enum properties in both hint-string forms: implicit
// values (0, 1, 2), explicit values, and a setter that clamps.
const enumFixtureScript = `@tool
extends Node

@export_enum("Slow", "Medium", "Fast") var speed_mode := 0
@export_enum("Low:1", "High:10") var power := 1

@export_enum("A", "B", "C") var clamped_mode := 0:
	set(v):
		clamped_mode = clampi(v, 0, 1)
`

func TestEnumNodeProperties(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/enum_props_test.tscn",
		"root_node_type": "Node",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node",
		"properties":  map[string]any{"name": "Fixture"},
	})
	writeProjectFileFromEditor(t, "res://scripts/enum_props_fixture.gd", enumFixtureScript)
	callToolOK(t, "attach_script", map[string]any{
		"node_path":   "Fixture",
		"script_path": "res://scripts/enum_props_fixture.gd",
	})

	setProps := func(t *testing.T, props map[string]any) map[string]any {
		t.Helper()
		return callToolOK(t, "set_node_properties", map[string]any{
			"action": "Enum test",
			"nodes":  map[string]any{"Fixture": props},
		})
	}

	t.Run("set_by_option_name", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"speed_mode": "Fast"})
		is.Equal(structured["success"], true)
		_, hasWarnings := structured["warnings"]
		is.True(!hasWarnings) // an exact name match must not warn

		props := getNodeProps(t, map[string]any{
			"node_paths":    []string{"Fixture:speed_mode"},
			"enums_as_ints": true,
		})
		is.Equal(props["Fixture:speed_mode"], "2") // "Fast" is enum value 2
	})

	t.Run("get_translates_with_note", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"Fixture"},
		})
		nodeProps, _ := structured["nodes"].(map[string]any)["Fixture"].(map[string]any)
		is.Equal(nodeProps["speed_mode"], "Fast")

		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "Fixture:speed_mode"))
		is.True(anyLineContains(notes, `"enums_as_ints": true`))
	})

	t.Run("get_enums_as_ints_has_no_note", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths":    []string{"Fixture"},
			"enums_as_ints": true,
		})
		nodeProps, _ := structured["nodes"].(map[string]any)["Fixture"].(map[string]any)
		is.Equal(nodeProps["speed_mode"], "2")

		_, hasNotes := structured["notes"]
		is.True(!hasNotes)
	})

	t.Run("set_by_int_notes_option_name", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"speed_mode": "1"})
		is.Equal(structured["success"], true)
		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "Fixture / speed_mode: 1 is the enum option 'Medium'"))
	})

	t.Run("set_by_name_ignoring_case", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"speed_mode": "fast"})
		is.Equal(structured["success"], true)
		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "'fast' was taken as the enum option 'Fast' (2)"))
	})

	t.Run("set_bad_name_suggests", func(t *testing.T) {
		is := is.New(t)

		structured := callToolErr(t, "set_node_properties", map[string]any{
			"action": "Enum test",
			"nodes":  map[string]any{"Fixture": map[string]any{"speed_mode": "Fastt"}},
		}, "did you mean 'Fast'")
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "'Slow' (0)")) // the error lists the valid options
	})

	t.Run("set_unmatched_int_warns_but_applies", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"speed_mode": "9"})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.True(anyLineContains(warnings, "9 does not match any option of this enum"))
		is.True(anyLineContains(warnings, "'Slow' (0)")) // the warning lists the valid options

		// No option name to show, so the value stays an int on get too.
		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"Fixture:speed_mode"},
		})
		is.Equal(props["Fixture:speed_mode"], "9")
	})

	t.Run("explicit_option_values", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"power": "High"})
		is.Equal(structured["success"], true)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"Fixture:power"},
		})
		is.Equal(props["Fixture:power"], "High")

		props = getNodeProps(t, map[string]any{
			"node_paths":    []string{"Fixture:power"},
			"enums_as_ints": true,
		})
		is.Equal(props["Fixture:power"], "10") // "High" has the explicit value 10
	})

	t.Run("clamping_setter_warns_with_option_name", func(t *testing.T) {
		is := is.New(t)

		structured := setProps(t, map[string]any{"clamped_mode": "C"})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.True(anyLineContains(warnings, "only to B")) // the warning names the option it clamped to
	})
}

func TestEnumResourceProperties(t *testing.T) {
	is := is.New(t)

	const materialPath = "res://resources/enum_material.tres"
	structured := callToolOK(t, "create_resource", map[string]any{
		"file_path":     materialPath,
		"resource_type": "StandardMaterial3D",
		"properties": map[string]any{
			"texture_filter": "Nearest",
		},
	})
	is.Equal(structured["success"], true)

	t.Run("get_translates_with_note", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_resource_properties", map[string]any{
			"file_path": materialPath,
		})
		props, _ := structured["properties"].(map[string]any)
		is.Equal(props["texture_filter"], "Nearest")

		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "texture_filter"))
		is.True(anyLineContains(notes, `"enums_as_ints": true`))
	})

	t.Run("get_enums_as_ints", func(t *testing.T) {
		is := is.New(t)

		props := getResourceProps(t, map[string]any{
			"file_path":     materialPath,
			"properties":    []string{"texture_filter"},
			"enums_as_ints": true,
		})
		is.Equal(props["texture_filter"], "0") // "Nearest" is enum value 0
	})

	t.Run("set_by_int_notes_option_name", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_resource_properties", map[string]any{
			"action":     "Change filtering",
			"file_path":  materialPath,
			"properties": map[string]any{"texture_filter": "2"},
		})
		is.Equal(structured["success"], true)
		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "texture_filter: 2 is the enum option"))
	})
}

func TestEnumProjectSettings(t *testing.T) {
	is := is.New(t)

	// An int enum project setting present in every Godot 4 build.
	const settingName = "rendering/textures/canvas_textures/default_texture_filter"

	before := callToolOK(t, "get_project_settings", map[string]any{
		"names":         []string{settingName},
		"enums_as_ints": true,
	})
	original, _ := before["settings"].(map[string]any)[settingName].(string)
	is.True(original != "")
	t.Cleanup(func() {
		callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{settingName: original},
		})
	})

	t.Run("get_translates_with_note", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "get_project_settings", map[string]any{
			"names": []string{settingName},
		})
		value, _ := structured["settings"].(map[string]any)[settingName].(string)
		is.True(value != original) // translated to the option name, not the int

		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, settingName))
		is.True(anyLineContains(notes, `"enums_as_ints": true`))
	})

	t.Run("many_translations_summarized", func(t *testing.T) {
		is := is.New(t)

		// Getting everything translates far more than the listing cutoff, so
		// the note doesn't spell out the settings.
		structured := callToolOK(t, "get_project_settings", map[string]any{
			"include_defaults": true,
		})
		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "Some integer enum values"))
		is.True(anyLineContains(notes, `"enums_as_ints": true`))
		is.True(!anyLineContains(notes, settingName))
	})

	t.Run("set_by_option_name", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{settingName: "Nearest"},
		})
		is.Equal(structured["success"], true)

		after := callToolOK(t, "get_project_settings", map[string]any{
			"names":         []string{settingName},
			"enums_as_ints": true,
		})
		is.Equal(after["settings"].(map[string]any)[settingName], "0") // "Nearest" is enum value 0
	})

	t.Run("set_by_int_notes_option_name", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_project_settings", map[string]any{
			"settings": map[string]any{settingName: "1"},
		})
		is.Equal(structured["success"], true)
		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "1 is the enum option"))
	})
}

func TestEnumEditorSettings(t *testing.T) {
	// Which editor settings exist varies by build, so find an int enum one
	// whose current value maps to an option name.
	out := runEditorScript(t, `var es := EditorInterface.get_editor_settings()
for prop in es.get_property_list():
	if prop['type'] != TYPE_INT or prop['hint'] != PROPERTY_HINT_ENUM:
		continue
	if not es.has_setting(prop['name']) or str(prop['name']).begins_with("godai/"):
		continue
	if prop['hint_string'].contains(":"):
		continue
	var options = prop['hint_string'].split(",")
	var value = es.get_setting(prop['name'])
	if typeof(value) == TYPE_INT and value >= 0 and value < options.size():
		print("ENUM_SETTING:", prop['name'], "|", options[value])
		return OK
return OK`)

	var name, optionName string
	lines, _ := out["output"].([]any)
	for _, line := range asStrings(lines) {
		if rest, found := strings.CutPrefix(line, "ENUM_SETTING:"); found {
			name, optionName, _ = strings.Cut(rest, "|")
		}
	}
	if name == "" {
		t.Skip("no int enum editor settings in this build")
	}

	is := is.New(t)

	structured := callToolOK(t, "get_editor_settings", map[string]any{
		"names": []string{name},
	})
	is.Equal(structured["settings"].(map[string]any)[name], optionName)
	notes, _ := structured["notes"].([]any)
	is.True(anyLineContains(notes, name))

	// Setting it back by name is a no-op that exercises the translation.
	set := callToolOK(t, "set_editor_settings", map[string]any{
		"settings": map[string]any{name: optionName},
	})
	is.Equal(set["success"], true)
}
