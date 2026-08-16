## Base for the tools that set arbitrary properties: prepares one set operation
## per property, then verifies each by reading the value back after setting.
@abstract
extends "res://addons/godai/tools/tool_manager.gd".DefaultTool

const Utils = preload("res://addons/godai/utils.gd")
const CustomLogger = preload("res://addons/godai/custom_logger.gd")

var logger: CustomLogger


func _init(p_data: Dictionary) -> void:
	super._init(p_data)
	logger = CustomLogger.new()
	OS.add_logger(logger)


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_PREDELETE:
			OS.remove_logger(logger)


## Returns {op = ...}, or {error = ...} when the set can't even be attempted.
## The result can also carry a 'note' and/or 'warning' about the value (e.g.
## the option name matching an int enum value).
## Pass the same p_prop_cache for every property in the request so each object's property list is fetched only once.
func prepare_property_op(p_object: Object, p_path: String, p_raw_value, p_prop_cache: Dictionary = {}) -> Dictionary:
	var resolve_error := ""
	var expected_type := TYPE_NIL
	var hint := PROPERTY_HINT_NONE
	var hint_string := ""
	var usage := PROPERTY_USAGE_NONE
	var old_value: Variant = null

	var resolved := Utils.resolve_property_path(p_object, p_path, p_prop_cache)
	if resolved.has("error"):
		# The property may still be settable (e.g. handled dynamically by
		# the object's script), so attempt it and let verification decide.
		resolve_error = resolved['error']
		old_value = p_object.get_indexed(p_path)
	else:
		expected_type = resolved['expected_type']
		hint = resolved['hint']
		hint_string = resolved['hint_string']
		usage = resolved['usage']
		old_value = resolved['value']

	var int_enum := Utils.is_int_enum(expected_type, hint)
	var note := ""
	var warning := ""
	var decoded := {}

	# An int enum can be set by option name, which isn't variant syntax, so the
	# name is translated before decoding. A string that parses as a number is
	# taken as the enum's int value instead.
	if int_enum and typeof(p_raw_value) == TYPE_STRING:
		var translated := Utils.int_enum_name_to_value(p_raw_value, hint_string)
		if translated.has("value"):
			decoded = { value = translated['value'] }
			if translated.has("matched_name"):
				note = "'%s' was taken as the enum option '%s' (%d)" % [p_raw_value, translated['matched_name'], translated['value']]
		elif typeof(str_to_var(p_raw_value)) not in [TYPE_INT, TYPE_FLOAT]:
			return { error = translated['error'] }

	var value_from_name := not decoded.is_empty()
	if not value_from_name:
		decoded = Utils.decode_property_value(p_raw_value, expected_type)
		if decoded.has("error"):
			return { error = decoded['error'] }

	if int_enum and not value_from_name and typeof(decoded['value']) in [TYPE_INT, TYPE_FLOAT] and float(int(decoded['value'])) == float(decoded['value']):
		var enum_name := Utils.enum_value_to_name(int(decoded['value']), hint_string)
		if enum_name.is_empty():
			warning = "%d does not match any option of this enum (valid values: %s)" % [int(decoded['value']), Utils.describe_int_enum_options(hint_string)]
		else:
			note = "%d is the enum option '%s'" % [int(decoded['value']), enum_name]

	# String enums store the name itself, so the value is checked against the
	# options directly.
	if hint == PROPERTY_HINT_ENUM and expected_type in [TYPE_STRING, TYPE_STRING_NAME]:
		var enum_error := Utils.check_enum_value(str(decoded['value']), hint_string)
		if not enum_error.is_empty():
			return { error = enum_error }

	# Setting 'script' is another way to attach one, so it gets the
	# same check attach_script makes.
	if p_path == "script" and p_object is Node:
		var script_error := Utils.check_script_for_node(p_object, decoded['value'])
		if not script_error.is_empty():
			return { error = script_error }

	var result := { op = {
		path = p_path,
		value = decoded['value'],
		old_value = old_value,
		resolve_error = resolve_error,
		usage = usage,
		hint = hint,
		hint_string = hint_string,
	}}
	if not note.is_empty():
		result['note'] = note
	if not warning.is_empty():
		result['warning'] = warning
	return result


## The name of the matching read tool (e.g. "get_node_properties"), so error
## messages can point at it for discovering the valid properties.
@abstract
func get_properties_tool_name() -> String


func _discovery_hint() -> String:
	return ' (%s with "include_defaults": true lists the valid properties)' % get_properties_tool_name()


## Returns {} on success, or {warning = ...} / {error = ...}.
func verify_property_op(p_object: Object, p_op: Dictionary) -> Dictionary:
	var after: Variant = p_object.get_indexed(p_op['path'])
	if Utils.values_equal_approx(after, p_op['value']):
		return {}

	var after_str := Utils.encode_property_value_for_hint(after, p_op['hint'], p_op['hint_string'])
	if Utils.values_equal_approx(after, p_op['old_value']):
		if not p_op['resolve_error'].is_empty():
			if after == null:
				return { error = "%s%s%s" % [p_op['resolve_error'], _non_tool_script_hint(p_object, p_op['path']), _discovery_hint()] }
			return { error = "the value read back unchanged after setting it (still %s), and %s%s%s" % [after_str, p_op['resolve_error'], _non_tool_script_hint(p_object, p_op['path']), _discovery_hint()] }
		if after == null:
			return { error = "the value read back null both before and after setting it - the setter may have rejected the value (is it the right type?), or the property may not exist%s" % _discovery_hint() }
		return { error = "the value read back unchanged after setting it (still %s) - the property may be read-only, the setter may have rejected the value, or it may have clamped the value to what it already was; retrying the same value will not change it" % after_str }

	return { warning = "the value was changed, but only to %s" % after_str }


## The most common reason an unrecognized property is really there in the
## script: the editor only sees @export variables of a non-@tool script.
func _non_tool_script_hint(p_object: Object, p_path: String) -> String:
	var script = p_object.get_script()
	if not script is Script or script.is_tool():
		return ""

	var first_segment: String = p_path.split(":")[0]
	for prop in script.get_script_property_list():
		if prop['name'] == first_segment:
			return " (the node's script defines '%s', but a non-@tool script only exposes @export variables to the editor)" % first_segment
	return ""


## Collects the note/warning a prepare_property_op() result may carry,
## prefixed with the op's label.
func collect_prepared_notices(p_prepared: Dictionary, p_label: String, p_notes: PackedStringArray, p_warnings: PackedStringArray) -> void:
	if p_prepared.has("note"):
		p_notes.append("%s: %s" % [p_label, p_prepared['note']])
	if p_prepared.has("warning"):
		p_warnings.append("%s: %s" % [p_label, p_prepared['warning']])


func verify_property_ops(p_ops: Array, p_errors: PackedStringArray, p_warnings: PackedStringArray) -> void:
	for op in p_ops:
		var verdict := verify_property_op(op['object'], op)
		if verdict.has("error"):
			p_errors.append("%s: %s" % [op['label'], verdict['error']])
		elif verdict.has("warning"):
			p_warnings.append("%s: %s" % [op['label'], verdict['warning']])


## Builds a rejection payload, attaching whatever the logger captured.
func build_rejection(p_errors: PackedStringArray) -> Dictionary:
	var rejection := {errors = p_errors}
	var output := logger.stop()
	if not output.is_empty():
		rejection['output'] = output
	return rejection


## Builds the resolved result content, leaving out the slots with nothing
## to say.
func build_result(p_result: Dictionary, p_errors: PackedStringArray, p_warnings: PackedStringArray, p_notes := PackedStringArray()) -> Dictionary:
	p_result['success'] = p_errors.is_empty()
	if not p_errors.is_empty():
		p_result['errors'] = p_errors
	if not p_warnings.is_empty():
		p_result['warnings'] = p_warnings
	if not p_notes.is_empty():
		var notes: Array = p_result.get_or_add('notes', [])
		notes.append_array(p_notes)
	var output := logger.stop()
	if not output.is_empty():
		p_result['output'] = output
	return p_result
