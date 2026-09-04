extends RefCounted

## Hidden properties that we should report anyway.
const HIDDEN_PROPERTIES_WORTH_REPORTING := ["name", "scene_file_path", "script"]

const UNSAVED_SCENE_NOTE := "the scene has unsaved changes - call save_scene once you're done editing to persist them"
const UNSAVED_SCRIPT_NOTE := "the script has unsaved changes - call save_script once you're done editing to persist them"


## Encodes a property value as a string in Godot variant syntax.
##
## The inverse of decode_property_value(): any value this returns can be passed
## back in, though the `Object(ClassName)` summary decodes to a brand new object
## with every property at its default.
static func encode_property_value(p_value: Variant) -> String:
	match typeof(p_value):
		TYPE_STRING, TYPE_STRING_NAME:
			# String properties are passed around raw, without quotes.
			return str(p_value)
		TYPE_OBJECT:
			if p_value == null or not is_instance_valid(p_value):
				return "null"
			var res := p_value as Resource
			if res and not res.is_built_in():
				return 'Resource("%s")' % res.resource_path
			# An embedded resource (or other object): just a summary, because
			# dumping every property would be huge. Sub-properties can be
			# accessed with a colon path (e.g. "mesh:radius").
			return "Object(%s)" % p_value.get_class()
		_:
			return var_to_str(p_value)


## Decodes a property value received from the AI into a Variant.
##
## Returns a Dictionary with either a 'value' key, or an 'error' key with a
## message that can be sent back to the AI.
static func decode_property_value(p_raw: Variant, p_expected_type: int) -> Dictionary:
	# Tolerate values that are already JSON-native (numbers, booleans, ...).
	if typeof(p_raw) != TYPE_STRING:
		return { value = p_raw }

	var string_value: String = p_raw

	# String properties take raw strings, but the tool descriptions promise
	# variant syntax, so a value parsing as a quoted string is unquoted.
	if p_expected_type == TYPE_STRING or p_expected_type == TYPE_STRING_NAME:
		var parsed_string: Variant = str_to_var(string_value)
		if typeof(parsed_string) in [TYPE_STRING, TYPE_STRING_NAME]:
			string_value = parsed_string
		if p_expected_type == TYPE_STRING_NAME:
			return { value = StringName(string_value) }
		return { value = string_value }

	var parsed: Variant = str_to_var(_add_object_comma(string_value))
	if parsed == null and not string_value.strip_edges() in ["null", "nil"]:
		# str_to_var() returns null on parse failure.
		if p_expected_type == TYPE_NIL:
			# Variant or unknown property type: fall back to the raw string.
			return { value = string_value }
		var prefix_error := _unknown_type_prefix_error(string_value)
		if not prefix_error.is_empty():
			return { error = prefix_error }
		return { error = 'Cannot parse "%s" as a Godot variant. Examples of valid values: 5, 2.5, true, Vector2(1, 2), Color(1, 0, 0, 1), Resource("res://path/to/file.tres"), Object(SphereMesh,"radius":2.0). Packed arrays take a flat list of components, so: PackedColorArray(0, 0, 0, 1, 1, 1, 1, 1) is two colors (r,g,b,a, r,g,b,a)' % string_value }

	var packed_error := _packed_array_error(string_value, parsed, p_expected_type)
	if not packed_error.is_empty():
		return { error = packed_error }

	return { value = parsed }


const _PACKED_ARRAY_INFO := {
	TYPE_PACKED_BYTE_ARRAY: { name = "PackedByteArray", components = 1 },
	TYPE_PACKED_INT32_ARRAY: { name = "PackedInt32Array", components = 1 },
	TYPE_PACKED_INT64_ARRAY: { name = "PackedInt64Array", components = 1 },
	TYPE_PACKED_FLOAT32_ARRAY: { name = "PackedFloat32Array", components = 1 },
	TYPE_PACKED_FLOAT64_ARRAY: { name = "PackedFloat64Array", components = 1 },
	TYPE_PACKED_STRING_ARRAY: { name = "PackedStringArray", components = 1 },
	TYPE_PACKED_VECTOR2_ARRAY: { name = "PackedVector2Array", components = 2, element = "x,y" },
	TYPE_PACKED_VECTOR3_ARRAY: { name = "PackedVector3Array", components = 3, element = "x,y,z" },
	TYPE_PACKED_COLOR_ARRAY: { name = "PackedColorArray", components = 4, element = "r,g,b,a" },
	TYPE_PACKED_VECTOR4_ARRAY: { name = "PackedVector4Array", components = 4, element = "x,y,z,w" },
}

const _NUMERIC_PACKED_ARRAY_TYPES := [TYPE_PACKED_BYTE_ARRAY, TYPE_PACKED_INT32_ARRAY, TYPE_PACKED_INT64_ARRAY, TYPE_PACKED_FLOAT32_ARRAY, TYPE_PACKED_FLOAT64_ARRAY]


## Catches two ways a packed array value would silently corrupt on set() instead of erroring.
static func _packed_array_error(p_string_value: String, p_parsed: Variant, p_expected_type: int) -> String:
	var parsed_type := typeof(p_parsed)
	if not parsed_type in _PACKED_ARRAY_INFO:
		return ""
	var info: Dictionary = _PACKED_ARRAY_INFO[parsed_type]

	if p_expected_type in _PACKED_ARRAY_INFO and p_expected_type != parsed_type:
		var both_numeric: bool = parsed_type in _NUMERIC_PACKED_ARRAY_TYPES and p_expected_type in _NUMERIC_PACKED_ARRAY_TYPES
		if not both_numeric:
			return "this property expects %s, not %s" % [_PACKED_ARRAY_INFO[p_expected_type]['name'], info['name']]

	var components: int = info['components']
	if components > 1:
		var trimmed := p_string_value.strip_edges()
		var inner := trimmed.substr(trimmed.find("(") + 1, trimmed.rfind(")") - trimmed.find("(") - 1)
		var value_count := 0
		for part in inner.split(","):
			if not part.strip_edges().is_empty():
				value_count += 1
		if value_count % components != 0:
			return "%s takes a flat list of %s components, so the number of values must be a multiple of %d - got %d, which would silently drop the leftover values" % [info['name'], info['element'], components, value_count]

	return ""


## Includes the Object/Resource forms decode_property_value adds on top of variant syntax.
const _VARIANT_TYPE_PREFIXES := [
	"Vector2", "Vector2i", "Vector3", "Vector3i", "Vector4", "Vector4i",
	"Rect2", "Rect2i", "Transform2D", "Plane", "Quaternion", "AABB",
	"Basis", "Transform3D", "Projection", "Color", "NodePath", "StringName",
	"PackedByteArray", "PackedInt32Array", "PackedInt64Array",
	"PackedFloat32Array", "PackedFloat64Array", "PackedStringArray",
	"PackedVector2Array", "PackedVector3Array", "PackedColorArray",
	"PackedVector4Array",
	"Object", "Resource",
]

## Returns "" when the prefix is a valid variant type (or there is none).
static func _unknown_type_prefix_error(p_value: String) -> String:
	# Compiled here rather than kept in a static var: a script reload resets
	# statics without re-running their initializers, leaving them null.
	var type_prefix_regex := RegEx.create_from_string("^([A-Za-z_][A-Za-z0-9_]*)\\s*\\(")
	var m := type_prefix_regex.search(p_value.strip_edges())
	if not m:
		return ""
	var prefix := m.get_string(1)
	if prefix in _VARIANT_TYPE_PREFIXES:
		return ""

	var suggestions := _closest_matches(prefix, PackedStringArray(_VARIANT_TYPE_PREFIXES))

	var msg := '"%s" is not a variant type.' % prefix
	if not suggestions.is_empty():
		msg += " Did you mean %s?" % " or ".join(suggestions)
	msg += " Valid types: %s" % ", ".join(_VARIANT_TYPE_PREFIXES)
	return msg


## Godot's variant parser wants a comma after the class name of an `Object(...)`,
## so `Object(SphereMesh)` - which is how a resource with nothing but default
## values reads, and what encode_property_value() returns for any embedded
## resource - fails to parse. This adds the comma it's missing.
static func _add_object_comma(p_value: String) -> String:
	var trimmed := p_value.strip_edges()
	if not trimmed.begins_with("Object(") or not trimmed.ends_with(")"):
		return p_value

	var object_class := trimmed.substr(7, trimmed.length() - 8).strip_edges()
	if not object_class.is_valid_ascii_identifier():
		return p_value

	return "Object(%s,)" % object_class


const _ARRAY_TYPES := [TYPE_ARRAY, TYPE_PACKED_BYTE_ARRAY, TYPE_PACKED_INT32_ARRAY, TYPE_PACKED_INT64_ARRAY, TYPE_PACKED_FLOAT32_ARRAY, TYPE_PACKED_FLOAT64_ARRAY, TYPE_PACKED_STRING_ARRAY, TYPE_PACKED_VECTOR2_ARRAY, TYPE_PACKED_VECTOR3_ARRAY, TYPE_PACKED_COLOR_ARRAY, TYPE_PACKED_VECTOR4_ARRAY]


## Compares two property values, tolerating the float noise a value picks up on
## its way through variant syntax and float32 storage.
static func values_equal_approx(p_a: Variant, p_b: Variant) -> bool:
	var type_a := typeof(p_a)
	var type_b := typeof(p_b)

	if type_a == TYPE_INT and type_b == TYPE_INT:
		return p_a == p_b

	# set() coerces numbers into bool properties, so a correct set of a bool
	# property with "1" reads back as true.
	if TYPE_BOOL in [type_a, type_b] and type_a in [TYPE_BOOL, TYPE_INT, TYPE_FLOAT] and type_b in [TYPE_BOOL, TYPE_INT, TYPE_FLOAT]:
		return float(p_a) == float(p_b)

	if type_a in [TYPE_INT, TYPE_FLOAT] and type_b in [TYPE_INT, TYPE_FLOAT]:
		return is_equal_approx(p_a, p_b)

	if type_a in _ARRAY_TYPES and type_b in _ARRAY_TYPES:
		if p_a.size() != p_b.size():
			return false
		for i in range(p_a.size()):
			if not values_equal_approx(p_a[i], p_b[i]):
				return false
		return true

	if type_a != type_b:
		if type_a in [TYPE_STRING, TYPE_STRING_NAME] and type_b in [TYPE_STRING, TYPE_STRING_NAME]:
			return str(p_a) == str(p_b)
		return false

	match type_a:
		TYPE_VECTOR2, TYPE_VECTOR3, TYPE_VECTOR4, TYPE_QUATERNION, TYPE_COLOR, TYPE_RECT2, TYPE_PLANE, TYPE_AABB, TYPE_BASIS, TYPE_TRANSFORM2D, TYPE_TRANSFORM3D:
			return p_a.is_equal_approx(p_b)
		TYPE_DICTIONARY:
			if p_a.size() != p_b.size():
				return false
			for key in p_a:
				if not p_b.has(key) or not values_equal_approx(p_a[key], p_b[key]):
					return false
			return true
		_:
			return p_a == p_b


## Resolves a colon-separated property path (e.g. "mesh:radius") on an object,
## validating each segment along the way.
##
## Pass the same p_prop_cache Dictionary for every path in a request so each object's property list is fetched only once.
##
## Returns a Dictionary with 'value' (the current value at the path),
## 'expected_type' (the declared Variant.Type of the final property, or
## TYPE_NIL if unknown), 'hint' and 'hint_string' (the final property's
## declared PropertyHint), and 'usage' (its PropertyUsageFlags) keys, or an
## 'error' key with a message that can be sent back to the AI.
static func resolve_property_path(p_object: Object, p_path: String, p_prop_cache: Dictionary = {}) -> Dictionary:
	var segments := p_path.split(":")
	var walked := ""

	var current: Variant = p_object
	for i in range(segments.size()):
		var seg: String = segments[i]
		var is_last := (i == segments.size() - 1)

		if current == null:
			return { error = "'%s' is null, so cannot access '%s'" % [walked, seg] }

		if not (current is Object):
			# A built-in type (Vector2, Color, ...): we can't cheaply validate
			# its member names, so defer to get_indexed() for the value.
			return {
				value = p_object.get_indexed(p_path),
				expected_type = TYPE_NIL,
				hint = PROPERTY_HINT_NONE,
				hint_string = "",
				usage = PROPERTY_USAGE_NONE,
			}

		var props := _object_property_info(current, p_prop_cache)

		# Metadata properties only appear in the property list once set, so
		# allow setting new ones.
		if not props.has(seg) and not (is_last and seg.begins_with("metadata/")):
			return { error = "%s has no property named '%s'%s" % [current.get_class(), seg, _property_suggestion(seg, props)] }

		if is_last:
			var info: Dictionary = props.get(seg, {})
			return {
				value = current.get(seg),
				expected_type = info.get('type', TYPE_NIL),
				hint = info.get('hint', PROPERTY_HINT_NONE),
				hint_string = info.get('hint_string', ""),
				usage = info.get('usage', PROPERTY_USAGE_NONE),
			}

		current = current.get(seg)
		walked = seg if walked.is_empty() else walked + ":" + seg

	return { error = "Empty property path" }


## Encodes the value of a resolve_property_path() result, translating an int
## enum value to its option name unless p_enums_as_ints. Returns {value,
## translated}.
static func encode_resolved_value(p_resolved: Dictionary, p_enums_as_ints: bool) -> Dictionary:
	if not p_enums_as_ints and is_int_enum(p_resolved['expected_type'], p_resolved['hint']) and typeof(p_resolved['value']) == TYPE_INT:
		var enum_name := enum_value_to_name(p_resolved['value'], p_resolved['hint_string'])
		if not enum_name.is_empty():
			return { value = enum_name, translated = true }
	return { value = encode_property_value(p_resolved['value']), translated = false }


## Maps the object's property names to their declared info ('type', 'hint',
## 'hint_string', 'usage'), skipping the grouping pseudo-properties. Cached in
## p_cache by instance ID.
static func _object_property_info(p_object: Object, p_cache: Dictionary) -> Dictionary:
	var id := p_object.get_instance_id()
	if p_cache.has(id):
		return p_cache[id]

	var props := {}
	for prop in p_object.get_property_list():
		if prop['usage'] & (PROPERTY_USAGE_GROUP | PROPERTY_USAGE_SUBGROUP | PROPERTY_USAGE_CATEGORY):
			continue
		if not props.has(prop['name']):
			props[prop['name']] = {
				type = prop['type'],
				hint = prop['hint'],
				hint_string = prop['hint_string'],
				usage = prop['usage'],
			}

	p_cache[id] = props
	return props


## A " - did you mean ...?" suffix with the object's property names most
## similar to p_name, or "" when nothing comes close.
static func property_suggestion(p_object: Object, p_name: String, p_prop_cache: Dictionary = {}) -> String:
	return _property_suggestion(p_name, _object_property_info(p_object, p_prop_cache))


## A " - did you mean ...?" suffix with the property names most similar to
## p_name, or "" when nothing comes close.
static func _property_suggestion(p_name: String, p_props: Dictionary) -> String:
	var candidates := PackedStringArray()
	for candidate in p_props:
		if not candidate.begins_with("_"):
			candidates.append(candidate)

	var suggestions := _quote_all(_closest_matches(p_name, candidates))
	if suggestions.is_empty():
		return ""
	return " - did you mean %s?" % " or ".join(suggestions)


## The candidates most similar to p_target (best first, at most 3), for
## "did you mean ...?" suggestions.
static func _closest_matches(p_target: String, p_candidates: PackedStringArray) -> PackedStringArray:
	# Negated score, so the plain ascending sort is best-first with ties
	# broken by declaration order.
	var scored := []
	for i in range(p_candidates.size()):
		scored.append([-p_candidates[i].similarity(p_target), i, p_candidates[i]])
	scored.sort()

	var matches := PackedStringArray()
	for s in scored:
		if -s[0] >= 0.5 and matches.size() < 3:
			matches.append(s[2])
	return matches


static func _quote_all(p_values: PackedStringArray) -> PackedStringArray:
	var quoted := PackedStringArray()
	for value in p_values:
		quoted.append("'%s'" % value)
	return quoted


## Validates a string value against a PROPERTY_HINT_ENUM hint string. Returns
## "" when the value is one of the options, or an error message with the
## closest options suggested.
static func check_enum_value(p_value: String, p_hint_string: String) -> String:
	var options := enum_option_names(p_hint_string)
	if p_value in options:
		return ""

	var suggestions := _quote_all(_closest_matches(p_value, options))

	var msg := "'%s' is not one of the valid values" % p_value
	if not suggestions.is_empty():
		msg += " - did you mean %s?" % " or ".join(suggestions)
	msg += " Valid values: %s" % ", ".join(options)
	return msg


## Whether a property declares an int enum: a PROPERTY_HINT_ENUM whose value
## holds the option's int, unlike a string enum which stores the name itself.
static func is_int_enum(p_type: int, p_hint: int) -> bool:
	return p_hint == PROPERTY_HINT_ENUM and p_type == TYPE_INT


## Parses a PROPERTY_HINT_ENUM hint string into {name, value} options. Each
## option can pair the name with an explicit value ("Name:value"); one without
## takes the previous option's value plus one, starting at 0 (the same rule
## the editor uses).
static func parse_enum_hint(p_hint_string: String) -> Array:
	var options := []
	if p_hint_string.is_empty():
		return options

	var next_value := 0
	for item in p_hint_string.split(","):
		var value := next_value
		if item.contains(":"):
			value = int(item.get_slice(":", 1))
		options.append({ name = item.get_slice(":", 0).strip_edges(), value = value })
		next_value = value + 1
	return options


static func enum_option_names(p_hint_string: String) -> PackedStringArray:
	var names := PackedStringArray()
	for option in parse_enum_hint(p_hint_string):
		names.append(option['name'])
	return names


## The option name for an int enum value, or "" when no option has that value.
static func enum_value_to_name(p_value: int, p_hint_string: String) -> String:
	for option in parse_enum_hint(p_hint_string):
		if option['value'] == p_value:
			return option['name']
	return ""


## The options of an int enum with their values, e.g. "'Nearest' (0), 'Linear' (1)".
static func describe_int_enum_options(p_hint_string: String) -> String:
	var parts := PackedStringArray()
	for option in parse_enum_hint(p_hint_string):
		parts.append("'%s' (%d)" % [option['name'], option['value']])
	return ", ".join(parts)


## Translates an option name of an int enum to its value. Returns {value}
## (plus 'matched_name' when the match ignored case), or {error} with the
## closest options suggested.
static func int_enum_name_to_value(p_name: String, p_hint_string: String) -> Dictionary:
	var options := parse_enum_hint(p_hint_string)
	for option in options:
		if option['name'] == p_name:
			return { value = option['value'] }
	for option in options:
		if option['name'].nocasecmp_to(p_name) == 0:
			return { value = option['value'], matched_name = option['name'] }

	var suggestions := _quote_all(_closest_matches(p_name, enum_option_names(p_hint_string)))

	var msg := "'%s' is not one of the valid values" % p_name
	if not suggestions.is_empty():
		msg += " - did you mean %s?" % " or ".join(suggestions)
	msg += " Valid values: %s" % describe_int_enum_options(p_hint_string)
	return { error = msg }


## Encodes a value for an error/warning message: an int enum value reads as
## its option name when it has one.
static func encode_property_value_for_hint(p_value: Variant, p_hint: int, p_hint_string: String) -> String:
	if p_hint == PROPERTY_HINT_ENUM and typeof(p_value) == TYPE_INT:
		var enum_name := enum_value_to_name(p_value, p_hint_string)
		if not enum_name.is_empty():
			return enum_name
	return encode_property_value(p_value)


## The note attached to get results when int enum values were translated to
## their option names. A long list of translated properties is summarized
## rather than spelled out.
static func enum_translation_note(p_translated: PackedStringArray) -> String:
	if p_translated.size() >= 5:
		return 'Some integer enum values are shown as their option name rather than the underlying int; pass "enums_as_ints": true for the raw values'
	return 'Integer enum values are shown as their option name rather than the underlying int (%s); pass "enums_as_ints": true for the raw values' % ", ".join(p_translated)


## Builds a map of property name to encoded value for the given object,
## skipping internal properties (and optionally, properties at their default
## value).
##
## Unless p_enums_as_ints, int enum values are translated to their option
## names, with the translated property names appended to r_translated.
static func get_property_map(p_object: Object, p_modified_only: bool, p_enums_as_ints := false, r_translated := PackedStringArray()) -> Dictionary:
	var props := {}

	for prop in p_object.get_property_list():
		var prop_name: String = prop['name']
		var prop_usage: int = prop['usage']

		if prop_name.begins_with("_"):
			continue
		if prop_usage & PROPERTY_USAGE_GROUP or prop_usage & PROPERTY_USAGE_CATEGORY or prop_usage & PROPERTY_USAGE_SUBGROUP:
			continue
		if ((prop_usage & PROPERTY_USAGE_INTERNAL) or not (prop_usage & (PROPERTY_USAGE_STORAGE | PROPERTY_USAGE_EDITOR))) and not prop_name in HIDDEN_PROPERTIES_WORTH_REPORTING:
			continue

		var value: Variant = p_object.get(prop_name)
		if p_modified_only and value == get_default_property_value(p_object, prop_name, prop['type']):
			continue

		if not p_enums_as_ints and is_int_enum(prop['type'], prop['hint']) and typeof(value) == TYPE_INT:
			var enum_name := enum_value_to_name(value, prop['hint_string'])
			if not enum_name.is_empty():
				props[prop_name] = enum_name
				r_translated.append(prop_name)
				continue

		props[prop_name] = encode_property_value(value)

	return props


## Reads settings from a settings object (the ProjectSettings or EditorSettings
## singleton) into a map of setting name to value, encoded in Godot variant
## syntax.
##
## When p_names is non-empty, only those settings are returned (regardless of
## whether they match their default), and an 'error' is returned if any don't
## exist. Otherwise all settings are returned, skipping those at their default
## value unless p_include_defaults is true.
##
## Unless p_enums_as_ints, int enum values are translated to their option
## names; 'translated' lists the setting names that were.
##
## Returns a Dictionary with either 'settings' and 'translated' keys, or an
## 'error' key with a message that can be sent back to the AI.
static func get_settings_map(p_settings: Object, p_names: Array, p_include_defaults: bool, p_enums_as_ints := false) -> Dictionary:
	var settings := {}
	var translated := PackedStringArray()

	if not p_names.is_empty():
		var missing := PackedStringArray()
		for name in p_names:
			if not p_settings.has_setting(name):
				missing.append(str(name))
		if not missing.is_empty():
			return { error = "No such setting(s): %s" % ", ".join(missing) }
		var info := _object_property_info(p_settings, {})
		for name in p_names:
			settings[name] = _encode_setting_value(p_settings.get_setting(name), info.get(name, {}), p_enums_as_ints, name, translated)
		return { settings = settings, translated = translated }

	for prop in p_settings.get_property_list():
		var name: String = prop['name']
		var usage: int = prop['usage']

		if usage & (PROPERTY_USAGE_GROUP | PROPERTY_USAGE_SUBGROUP | PROPERTY_USAGE_CATEGORY):
			continue
		# Filters out the settings object's own properties (e.g. 'script'),
		# leaving only actual settings.
		if not p_settings.has_setting(name):
			continue
		if not p_include_defaults and not is_setting_modified(p_settings, name):
			continue

		settings[name] = _encode_setting_value(p_settings.get_setting(name), prop, p_enums_as_ints, name, translated)

	return { settings = settings, translated = translated }


static func _encode_setting_value(p_value: Variant, p_info: Dictionary, p_enums_as_ints: bool, p_name: String, r_translated: PackedStringArray) -> String:
	if not p_enums_as_ints and is_int_enum(p_info.get('type', TYPE_NIL), p_info.get('hint', PROPERTY_HINT_NONE)) and typeof(p_value) == TYPE_INT:
		var enum_name := enum_value_to_name(p_value, p_info.get('hint_string', ""))
		if not enum_name.is_empty():
			r_translated.append(p_name)
			return enum_name
	return encode_property_value(p_value)


## Whether a setting's current value differs from its default. Settings with no
## known default (e.g. custom ones) are always considered modified.
static func is_setting_modified(p_settings: Object, p_name: String) -> bool:
	if not p_settings.property_can_revert(p_name):
		return true
	# values_equal_approx, because Godot may hand back the current value and
	# the default as different types (e.g. a bool setting with an int default),
	# and comparing those directly is a script error.
	return not values_equal_approx(p_settings.get_setting(p_name), p_settings.property_get_revert(p_name))


## Gets the default value of a property, for both native and script properties.
##
## Godot only tracks defaults for the properties it saves or shows in the
## inspector, so for anything else we fall back to the zero value of p_type
## (e.g. "" for a String), which is what such a property holds when nothing set
## it.
static func get_default_property_value(p_object: Object, p_prop_name: String, p_type := TYPE_NIL) -> Variant:
	var script: Script = p_object.get_script()
	if script:
		for prop in script.get_script_property_list():
			if prop['name'] == p_prop_name:
				return script.get_property_default_value(p_prop_name)

	var default: Variant = ClassDB.class_get_property_default_value(p_object.get_class(), p_prop_name)
	if default == null and p_type != TYPE_NIL and p_type != TYPE_OBJECT:
		# Godot has no API for "the zero value of this type", but resizing a
		# typed array fills the new slots with exactly that.
		var typed := Array([], p_type, "", null)
		typed.resize(1)
		return typed[0]

	return default


## Whether a value can be a node's script: it has to be a Script (or null, to
## detach the one it has), and its base type has to be one the node actually is.
##
## Returns an error message, or "" when it fits.
static func check_script_for_node(p_node: Node, p_script: Variant) -> String:
	if p_script == null:
		return ""
	if not p_script is Script:
		return "%s is not a script" % encode_property_value(p_script)

	var base_type: String = p_script.get_instance_base_type()
	if base_type != "" and not ClassDB.is_parent_class(p_node.get_class(), base_type):
		return "Script extends '%s', which is not compatible with a node of type '%s'" % [base_type, p_node.get_class()]

	return ""


static func get_project_path() -> String:
	return ProjectSettings.globalize_path("res://").simplify_path()


static func is_pid_running(p_pid: int) -> bool:
	if p_pid <= 0:
		return false

	if OS.get_name() == "Windows":
		var output := []
		OS.execute("tasklist", ["/FI", "PID eq %d" % p_pid, "/NH", "/FO", "CSV"], output)
		if output.is_empty():
			return false
		# A match is one CSV row: "image.exe","1234","Console","1","12,345 K"
		# PID is always the 2nd column. Split on the quote-comma-quote delimiter.
		var fields: PackedStringArray = output[0].strip_edges().trim_prefix("\"").split("\",\"")
		return fields.size() > 1 and fields[1] == str(p_pid)

	if OS.get_name() == "Linux":
		var status := FileAccess.open("/proc/%d/status" % p_pid, FileAccess.READ)
		if not status:
			return false
		while not status.eof_reached():
			var line := status.get_line()
			if line.begins_with("State:"):
				# We don't count zombie processes as running.
				return not line.trim_prefix("State:").strip_edges().begins_with("Z")
		return false

	# MacOS or other UNIX-y systems.
	var output := []
	OS.execute("ps", ["-p", str(p_pid), "-o", "stat="], output)
	if output.is_empty():
		return false
	var stat := str(output[0]).strip_edges()
	return not stat.is_empty() and not stat.begins_with("Z")


## Normalizes a path into a `res://` path, adding the prefix if missing.
## Returns "" if the path escapes the project (e.g. via ".." segments).
static func to_res_path(p_path: String) -> String:
	if not p_path.begins_with("res://"):
		p_path = "res://" + p_path.lstrip("/")
	var root := ProjectSettings.globalize_path("res://").simplify_path()
	var resolved := ProjectSettings.globalize_path(p_path).simplify_path()
	if resolved != root and not resolved.begins_with(root + "/"):
		return ""
	return p_path.simplify_path()


## Finds the live text editor for a script that is open in the script editor,
## or null if the script isn't currently open. The returned control is a
## TextEdit (CodeEdit) whose `text` is the live, possibly-unsaved buffer.
static func get_open_script_editor(p_script_path: String) -> TextEdit:
	if not Engine.is_editor_hint():
		return null

	var script_editor := EditorInterface.get_script_editor()
	if not script_editor:
		return null

	# get_open_script_editors() has an entry for every tab in the script editor,
	# including files that aren't scripts (a .txt open in a tab of its own, for
	# example), while get_open_scripts() only has the scripts. The two only line
	# up once the tabs editing something other than a script are skipped.
	var scripts := script_editor.get_open_scripts()
	var editors := script_editor.get_open_script_editors()

	var index := 0
	for editor in editors:
		if not editor.is_class("ScriptTextEditor"):
			continue
		if index >= scripts.size():
			break
		var script: Script = scripts[index]
		index += 1
		if script and script.resource_path == p_script_path:
			var base: Control = editor.get_base_editor()
			if base is TextEdit:
				return base
			return null

	return null


## Tracks the hash of each script's content as the AI last saw it (via
## read_script, or after it created/wrote the script), so write_script can
## refuse to overwrite changes the AI hasn't seen.
static var _script_read_hashes := {}


## Returns the current content of a script as the AI would read it: the live
## editor buffer when it's open, otherwise the file on disk. Returns a
## Dictionary with 'content' and 'open_in_editor' keys, or an 'error' key.
static func read_script_content(p_path: String) -> Dictionary:
	var editor := get_open_script_editor(p_path)
	if editor:
		return { content = editor.text, open_in_editor = true }

	var fa := FileAccess.open(p_path, FileAccess.READ)
	if not fa:
		return { error = "Failed to read '%s': %s" % [p_path, error_string(FileAccess.get_open_error())] }
	var text := fa.get_as_text()
	fa.close()
	return { content = text, open_in_editor = false }


## Records the content the AI has just seen (or written) for a script, so a
## later write can tell whether it's still up to date.
static func record_script_read(p_path: String, p_content: String) -> void:
	_script_read_hashes[p_path] = p_content.sha256_text()


## Checks whether it's safe to overwrite a script: the AI must have read it,
## and it must not have changed since. Returns an empty Dictionary when it's
## safe, or one with an 'error' key explaining why not.
static func check_script_writable(p_path: String, p_current_content: String) -> Dictionary:
	if not _script_read_hashes.has(p_path):
		return { error = "You must read '%s' with read_script before writing to it, so you don't overwrite changes you haven't seen." % p_path }
	if _script_read_hashes[p_path] != p_current_content.sha256_text():
		return { error = "'%s' has changed since you last read it; read it again with read_script before writing, so you don't overwrite those changes." % p_path }
	return {}


## Clear our tracking of script reads.
static func clear_script_reads() -> void:
	_script_read_hashes.clear()


## Gets the scene tree.
static func get_scene_tree() -> SceneTree:
	var main_loop: MainLoop = Engine.get_main_loop()
	if main_loop is SceneTree:
		return main_loop
	return null


## Gets the root of the scene tree.
static func get_scene_root() -> Node:
	var scene_tree: SceneTree = get_scene_tree()
	if not scene_tree:
		return null

	return scene_tree.root


## Gets the `EditorDebuggerNode`.
static func get_editor_debugger_node() -> Node:
	var root: Node = get_scene_root()
	if not root:
		return null

	var results = root.find_children("*", "EditorDebuggerNode", true, false)
	if len(results) == 0:
		return null

	return results[0]


## Set the name of a child node that is being added to the given parent.
static func set_child_node_name(p_parent: Node, p_child: Node) -> void:
	var name: String = p_child.name
	if name == "":
		name = p_child.get_class()

	if p_parent.has_node(name):
		var base_name := name.rstrip("0123456789")
		var num := 2

		while true:
			var test_name: String = base_name + str(num)
			if not p_parent.has_node(test_name):
				name = test_name
				break
			num += 1

	p_child.name = name


## Configures the EditorUndoRedoManager to create and add a node.
static func editor_undo_redo_create_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
	if not edited_scene_root:
		return

	p_undo_redo.add_do_method(p_parent, "add_child", p_child, true)
	p_undo_redo.add_do_method(p_child, "set_owner", edited_scene_root)
	p_undo_redo.add_do_method(EditorInterface.get_selection(), "add_node", p_child)
	p_undo_redo.add_do_reference(p_child)
	p_undo_redo.add_undo_method(p_parent, "remove_child", p_child)

	# If there is a debugger connection, then do this "live" as well.
	var editor_debugger_node := get_editor_debugger_node()
	if editor_debugger_node:
		set_child_node_name(p_parent, p_child)
		p_undo_redo.add_do_method(editor_debugger_node, "live_debug_create_node", edited_scene_root.get_path_to(p_parent), p_child.get_class(), p_child.name)
		p_undo_redo.add_undo_method(editor_debugger_node, "live_debug_remove_node", NodePath(str(edited_scene_root.get_path_to(p_parent)) + "/" + p_child.name))


## Configures the EditorUndoRedoManager to remove a node.
static func editor_undo_redo_remove_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
	if not edited_scene_root:
		return

	p_undo_redo.add_do_method(p_parent, "remove_child", p_child)
	p_undo_redo.add_undo_method(p_parent, "add_child", p_child, true)
	p_undo_redo.add_undo_method(p_parent, "move_child", p_child, p_child.get_index(false))
	p_undo_redo.add_undo_method(p_child, "set_owner", edited_scene_root)
	p_undo_redo.add_undo_reference(p_child)

	# If there is a debugger connection, then do this "live" as well.
	var editor_debugger_node := get_editor_debugger_node()
	if editor_debugger_node:
		p_undo_redo.add_do_method(editor_debugger_node, "live_debug_remove_and_keep_node", edited_scene_root.get_path_to(p_child), p_child.get_instance_id());
		p_undo_redo.add_undo_method(editor_debugger_node, "live_debug_restore_node", p_child.get_instance_id(), edited_scene_root.get_path_to(p_parent), p_child.get_index(false))
