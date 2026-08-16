package addon

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// A script exercising every way a property set can go sideways: setters that
// clamp or reject values, dynamic properties (declared and not), a write-only
// property, and setters that print. @tool, so all of it actually runs in the
// editor.
const verifyFixtureScript = `@tool
extends Node

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

var noisy_value := 0.0:
	set(v):
		print("noisy setter was here")
		noisy_value = v

var offset_int := 0:
	set(v):
		offset_int = v + 7

var bool_value := false:
	set(v):
		bool_value = bool(v)

var _stash := {}


func _get_property_list():
	return [{name = "declared_dynamic", type = TYPE_INT, usage = PROPERTY_USAGE_DEFAULT}]


func _set(p_property: StringName, p_value) -> bool:
	if p_property in ["declared_dynamic", "undeclared_dynamic", "write_only"]:
		_stash[p_property] = p_value
		return true
	return false


func _get(p_property: StringName):
	if p_property in ["declared_dynamic", "undeclared_dynamic"]:
		return _stash.get(p_property, 0)
	return null
`

func setFixtureProps(t *testing.T, props map[string]any) map[string]any {
	t.Helper()
	return callToolOK(t, "set_node_properties", map[string]any{
		"action": "Verification test",
		"nodes":  map[string]any{"Fixture": props},
	})
}

func anyLineContains(lines []any, substr string) bool {
	for _, line := range asStrings(lines) {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

func TestSetNodePropertiesVerification(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/verify_props_test.tscn",
		"root_node_type": "Node",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node",
		"properties":  map[string]any{"name": "Fixture"},
	})
	writeProjectFileFromEditor(t, "res://scripts/verify_props_fixture.gd", verifyFixtureScript)
	callToolOK(t, "attach_script", map[string]any{
		"node_path":   "Fixture",
		"script_path": "res://scripts/verify_props_fixture.gd",
	})

	t.Run("plain_set", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"plain_value": "3.5"})
		is.Equal(structured["success"], true)
		_, hasErrors := structured["errors"]
		is.True(!hasErrors)
		_, hasWarnings := structured["warnings"]
		is.True(!hasWarnings)

		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "save_scene"))
	})

	t.Run("set_to_current_value", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"plain_value": "3.5"})
		is.Equal(structured["success"], true)
		_, hasErrors := structured["errors"]
		is.True(!hasErrors)
	})

	t.Run("clamping_setter_warns", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"clamped_value": "50.0"})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "Fixture / clamped_value"))
		is.True(strings.Contains(asStrings(warnings)[0], "only to 10.0"))
	})

	t.Run("rejecting_setter_errors_with_output", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"rejecting_value": "-5.0"})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "Fixture / rejecting_value"))
		is.True(strings.Contains(asStrings(errs)[0], "read back unchanged"))

		output, _ := structured["output"].([]any)
		is.True(anyLineContains(output, "rejecting_value cannot be negative"))
	})

	t.Run("declared_dynamic_property", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"declared_dynamic": "42"})
		is.Equal(structured["success"], true)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"Fixture:declared_dynamic"},
		})
		is.Equal(props["Fixture:declared_dynamic"], "42")
	})

	t.Run("undeclared_dynamic_property", func(t *testing.T) {
		is := is.New(t)

		// Not in the property list, but _set()/_get() handle it: the attempt
		// succeeds and verification confirms it.
		structured := setFixtureProps(t, map[string]any{"undeclared_dynamic": "7"})
		is.Equal(structured["success"], true)
		_, hasErrors := structured["errors"]
		is.True(!hasErrors)
	})

	t.Run("bool_set_from_numeric_string", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"bool_value": "1"})
		is.Equal(structured["success"], true)
		_, hasWarnings := structured["warnings"]
		is.True(!hasWarnings)
	})

	t.Run("write_only_property", func(t *testing.T) {
		is := is.New(t)

		// _set() stores it but _get() can't read it back, so this can't be
		// verified; the error must say why without claiming certainty.
		structured := setFixtureProps(t, map[string]any{"write_only": "9"})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "has no property named 'write_only'"))
		is.True(strings.Contains(asStrings(errs)[0], `get_node_properties with "include_defaults": true`))
	})

	t.Run("misspelled_property_suggests_the_real_one", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"plain_valu": "1.0"})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "has no property named 'plain_valu'"))
		is.True(strings.Contains(asStrings(errs)[0], "did you mean 'plain_value'"))
	})

	t.Run("indexing_into_non_object_hints_discovery", func(t *testing.T) {
		is := is.New(t)

		// "plain_value:0" walks into a float, so the path never resolves to a
		// property: the read is null both before and after the set.
		structured := setFixtureProps(t, map[string]any{"plain_value:0": "1.0"})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "the property may not exist"))
		is.True(strings.Contains(asStrings(errs)[0], `get_node_properties with "include_defaults": true`))
	})

	t.Run("noisy_setter_output", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"noisy_value": "2.0"})
		is.Equal(structured["success"], true)
		output, _ := structured["output"].([]any)
		is.True(anyLineContains(output, "noisy setter was here"))
	})

	t.Run("mixed_outcomes_in_one_call", func(t *testing.T) {
		is := is.New(t)

		// clamped_value sits at 10.0 from the earlier subtest, so -3.0 clamps
		// to 0.0: a change, but not to the requested value.
		structured := setFixtureProps(t, map[string]any{
			"plain_value":     "8.25",
			"clamped_value":   "-3.0",
			"rejecting_value": "-1.0",
		})
		is.Equal(structured["success"], false)

		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "rejecting_value"))

		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "clamped_value"))

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"Fixture:plain_value"},
		})
		is.Equal(props["Fixture:plain_value"], "8.25")
	})

	t.Run("large_int_off_by_a_little_warns", func(t *testing.T) {
		is := is.New(t)

		// Ints compare exactly: at this magnitude an approximate comparison
		// would consider 10000007 equal to the requested value.
		structured := setFixtureProps(t, map[string]any{"offset_int": "10000000"})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "only to 10000007"))
	})

	t.Run("float_round_trip_is_not_a_warning", func(t *testing.T) {
		is := is.New(t)

		structured := setFixtureProps(t, map[string]any{"plain_value": "0.1"})
		is.Equal(structured["success"], true)
		_, hasWarnings := structured["warnings"]
		is.True(!hasWarnings)
	})
}

// Without @tool, the editor gives the script a placeholder instance: @export
// variables can be set, but plain members silently can't.
const nonToolFixtureScript = `extends Node

@export var exported_value := 0.0

var member_value := 0.0
`

func TestSetNodePropertiesNonToolScript(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/verify_non_tool_test.tscn",
		"root_node_type": "Node",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node",
		"properties":  map[string]any{"name": "PlainScript"},
	})
	writeProjectFileFromEditor(t, "res://scripts/verify_non_tool_fixture.gd", nonToolFixtureScript)
	callToolOK(t, "attach_script", map[string]any{
		"node_path":   "PlainScript",
		"script_path": "res://scripts/verify_non_tool_fixture.gd",
	})

	t.Run("exported_var", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Set exported var",
			"nodes":  map[string]any{"PlainScript": map[string]any{"exported_value": "2.5"}},
		})
		is.Equal(structured["success"], true)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"PlainScript:exported_value"},
		})
		is.Equal(props["PlainScript:exported_value"], "2.5")
	})

	t.Run("plain_member_var_errors_with_hint", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Set member var",
			"nodes":  map[string]any{"PlainScript": map[string]any{"member_value": "2.5"}},
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "non-@tool script only exposes @export variables"))
	})
}

func TestAddNodeVerification(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/verify_add_node_test.tscn",
		"root_node_type": "Node2D",
	})

	t.Run("unknown_property_still_creates_node", func(t *testing.T) {
		is := is.New(t)

		// The node is created, so a property problem is a warning: an error
		// (success = false) could push an agent into retrying the whole call,
		// duplicating the node.
		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties": map[string]any{
				"name":         "Survivor",
				"no_such_prop": "1",
			},
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["node_path"], "Survivor")
		_, hasErrors := structured["errors"]
		is.True(!hasErrors)
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "has no property named 'no_such_prop'"))
	})

	t.Run("clamped_property_warns", func(t *testing.T) {
		is := is.New(t)

		// Range.value is clamped to [min_value, max_value] by its setter.
		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "HSlider",
			"properties":  map[string]any{"value": "500.0"},
		})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "value"))
	})
}
