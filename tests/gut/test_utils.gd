extends GutTest

const Utils = preload("res://addons/godai/utils.gd")


func test_encode_property_value() -> void:
	assert_eq(Utils.encode_property_value(Vector2(1, 2)), "Vector2(1, 2)")
	assert_eq(Utils.encode_property_value(0.5), "0.5")
	assert_eq(Utils.encode_property_value(true), "true")
	assert_eq(Utils.encode_property_value(null), "null")

	# Strings (and StringNames) are passed around raw, without quotes.
	assert_eq(Utils.encode_property_value("Hello"), "Hello")
	assert_eq(Utils.encode_property_value(&"Hello"), "Hello")

	# A saved resource is encoded as a reference to its file.
	var external := load("res://icon.png")
	assert_eq(Utils.encode_property_value(external), 'Resource("res://icon.png")')

	# An embedded resource is encoded as just a summary.
	var embedded := SphereMesh.new()
	embedded.radius = 2.0
	assert_eq(Utils.encode_property_value(embedded), "Object(SphereMesh)")

	# A null value in an Object-type property.
	var mesh_instance: MeshInstance3D = autofree(MeshInstance3D.new())
	assert_eq(Utils.encode_property_value(mesh_instance.get("mesh")), "null")


func test_decode_property_value() -> void:
	var decoded: Dictionary

	# Strings are taken raw for String properties...
	decoded = Utils.decode_property_value("Hello", TYPE_STRING)
	assert_eq(decoded.get("value"), "Hello")

	# ... and for StringName properties.
	decoded = Utils.decode_property_value("Hello", TYPE_STRING_NAME)
	assert_eq(decoded.get("value"), &"Hello")

	# Variant syntax works for them too, without the quotes becoming part of
	# the value...
	decoded = Utils.decode_property_value('"Hello World"', TYPE_STRING)
	assert_eq(decoded.get("value"), "Hello World")

	decoded = Utils.decode_property_value('"Hello"', TYPE_STRING_NAME)
	assert_eq(decoded.get("value"), &"Hello")

	# ... but a raw string that parses as some other type stays raw.
	decoded = Utils.decode_property_value("true", TYPE_STRING)
	assert_eq(decoded.get("value"), "true")

	# Other types are parsed with str_to_var().
	decoded = Utils.decode_property_value("Vector2(1, 2)", TYPE_VECTOR2)
	assert_eq(decoded.get("value"), Vector2(1, 2))

	decoded = Utils.decode_property_value("null", TYPE_OBJECT)
	assert_true(decoded.has("value"))
	assert_eq(decoded.get("value"), null)

	# JSON-native values are tolerated as-is.
	decoded = Utils.decode_property_value(2.5, TYPE_FLOAT)
	assert_eq(decoded.get("value"), 2.5)

	# A saved resource reference gets loaded.
	decoded = Utils.decode_property_value('Resource("res://icon.png")', TYPE_OBJECT)
	assert_true(decoded.get("value") is Texture2D)

	# An embedded resource gets created, with its properties set.
	decoded = Utils.decode_property_value('Object(SphereMesh,"radius":2.0)', TYPE_OBJECT)
	assert_true(decoded.get("value") is SphereMesh)
	assert_eq(decoded.get("value").radius, 2.0)

	# Godot's own parser needs a comma after the class name, so we add it: an
	# Object() with no properties comes out with all of them at their defaults.
	decoded = Utils.decode_property_value("Object(SphereMesh)", TYPE_OBJECT)
	assert_true(decoded.get("value") is SphereMesh)
	assert_eq(decoded.get("value").radius, SphereMesh.new().radius)

	decoded = Utils.decode_property_value(" Object( SphereMesh ) ", TYPE_OBJECT)
	assert_true(decoded.get("value") is SphereMesh)

	# Anything that isn't an Object() holding a bare class name goes to Godot's
	# parser untouched.
	assert_eq(Utils._add_object_comma('Object(SphereMesh,"radius":2.0)'), 'Object(SphereMesh,"radius":2.0)')
	assert_eq(Utils._add_object_comma("Object(Sphere Mesh)"), "Object(Sphere Mesh)")
	assert_eq(Utils._add_object_comma("Vector2(1, 2)"), "Vector2(1, 2)")

	# Parse failures are reported, rather than silently becoming null.
	decoded = Utils.decode_property_value("Vector2(1 2)", TYPE_VECTOR2)
	assert_true(decoded.has("error"))
	assert_string_contains(decoded.get("error", ""), "Cannot parse")

	# ... except for Variant (or unknown) properties, where the raw string is
	# used as a fallback.
	decoded = Utils.decode_property_value("not a variant", TYPE_NIL)
	assert_eq(decoded.get("value"), "not a variant")

	decoded = Utils.decode_property_value("PackedFloatArray(0, 1)", TYPE_PACKED_FLOAT32_ARRAY)
	assert_string_contains(decoded.get("error", ""), '"PackedFloatArray" is not a variant type')
	assert_string_contains(decoded.get("error", ""), "Did you mean PackedFloat32Array")

	decoded = Utils.decode_property_value("Frobnicate(1)", TYPE_VECTOR2)
	assert_string_contains(decoded.get("error", ""), '"Frobnicate" is not a variant type')
	assert_string_contains(decoded.get("error", ""), "Valid types:")

	decoded = Utils.decode_property_value("Vector2(1 2)", TYPE_VECTOR2)
	assert_string_contains(decoded.get("error", ""), "Cannot parse")

	decoded = Utils.decode_property_value("PackedFloatArray(0, 1)", TYPE_NIL)
	assert_eq(decoded.get("value"), "PackedFloatArray(0, 1)")

	# Godot's parser silently drops components that don't fill out a whole
	# element (2 floats make 0 colors), so that's an error here.
	decoded = Utils.decode_property_value("PackedColorArray(0.0, 1.0)", TYPE_PACKED_COLOR_ARRAY)
	assert_string_contains(decoded.get("error", ""), "multiple of 4")

	decoded = Utils.decode_property_value("PackedVector2Array(1, 2, 3)", TYPE_NIL)
	assert_string_contains(decoded.get("error", ""), "multiple of 2")

	decoded = Utils.decode_property_value("PackedColorArray(0.1, 0.2, 0.6, 1.0, 1.0, 0.7, 0.4, 1.0)", TYPE_PACKED_COLOR_ARRAY)
	assert_eq(decoded.get("value"), PackedColorArray([Color(0.1, 0.2, 0.6, 1.0), Color(1.0, 0.7, 0.4, 1.0)]))

	# A packed array of the wrong element type zero-fills when set(), so
	# a mismatch is an error...
	decoded = Utils.decode_property_value("PackedColorArray(1, 0, 0, 1)", TYPE_PACKED_FLOAT32_ARRAY)
	assert_string_contains(decoded.get("error", ""), "expects PackedFloat32Array, not PackedColorArray")

	# When the type is wrong AND the count is bad, the type mismatch wins:
	# fixing the type fixes both.
	decoded = Utils.decode_property_value("PackedColorArray(0.0, 1.0)", TYPE_PACKED_FLOAT32_ARRAY)
	assert_string_contains(decoded.get("error", ""), "expects PackedFloat32Array, not PackedColorArray")

	# ... except between the numeric packed arrays, which convert cleanly.
	decoded = Utils.decode_property_value("PackedFloat64Array(0.25, 0.75)", TYPE_PACKED_FLOAT32_ARRAY)
	assert_eq(decoded.get("value"), PackedFloat64Array([0.25, 0.75]))

	# A plain array converts on set(), so it stays accepted.
	decoded = Utils.decode_property_value("[0.0, 1.0]", TYPE_PACKED_FLOAT32_ARRAY)
	assert_eq(decoded.get("value"), [0.0, 1.0])


func test_values_equal_approx() -> void:
	# Numbers compare across int/float, with float32 round-trip noise tolerated.
	assert_true(Utils.values_equal_approx(1, 1.0))
	assert_true(Utils.values_equal_approx(0.1, 0.100000001490116))
	assert_false(Utils.values_equal_approx(0.1, 0.11))

	# Two ints compare exactly: the approximate comparison is relative, so at
	# large magnitudes it would call genuinely different ints equal.
	assert_true(Utils.values_equal_approx(10000000, 10000000))
	assert_false(Utils.values_equal_approx(10000000, 10000007))

	# Strings and StringNames compare by content; a string is never a number.
	assert_true(Utils.values_equal_approx("Hello", &"Hello"))
	assert_false(Utils.values_equal_approx("1", 1))

	assert_true(Utils.values_equal_approx(true, 1))
	assert_true(Utils.values_equal_approx(0, false))
	assert_true(Utils.values_equal_approx(true, 1.0))
	assert_false(Utils.values_equal_approx(true, 2))
	assert_false(Utils.values_equal_approx(false, 1))

	assert_true(Utils.values_equal_approx(Vector2(0.1, 0.2), Vector2(0.100000001, 0.200000003)))
	assert_false(Utils.values_equal_approx(Vector2(1, 2), Vector2(1, 3)))
	assert_true(Utils.values_equal_approx(Color(0.5, 0.25, 0.125), Color(0.5, 0.25, 0.125)))

	assert_true(Utils.values_equal_approx(PackedFloat32Array([0.1, 0.2]), [0.100000001490116, 0.2]))
	assert_false(Utils.values_equal_approx([1, 2], [1, 2, 3]))

	assert_true(Utils.values_equal_approx({a = 0.1}, {a = 0.100000001490116}))
	assert_false(Utils.values_equal_approx({a = 1}, {b = 1}))

	# Objects compare by reference.
	var mesh := SphereMesh.new()
	assert_true(Utils.values_equal_approx(mesh, mesh))
	assert_false(Utils.values_equal_approx(mesh, SphereMesh.new()))
	assert_false(Utils.values_equal_approx(mesh, null))
	assert_true(Utils.values_equal_approx(null, null))


func test_resolve_property_path() -> void:
	var resolved: Dictionary

	var node: Node2D = autofree(Node2D.new())
	node.position = Vector2(1, 2)

	# A plain property.
	resolved = Utils.resolve_property_path(node, "position")
	assert_eq(resolved.get("value"), Vector2(1, 2))
	assert_eq(resolved.get("expected_type"), TYPE_VECTOR2)

	# A member of a built-in type can't be validated, so the type is unknown.
	resolved = Utils.resolve_property_path(node, "position:x")
	assert_eq(resolved.get("value"), 1.0)
	assert_eq(resolved.get("expected_type"), TYPE_NIL)

	# An unknown property is an error.
	resolved = Utils.resolve_property_path(node, "no_such_prop")
	assert_string_contains(resolved.get("error", ""), "no property named 'no_such_prop'")

	resolved = Utils.resolve_property_path(node, "positon")
	assert_string_contains(resolved.get("error", ""), "no property named 'positon'")
	assert_string_contains(resolved.get("error", ""), "did you mean 'position'")

	var cache := {}
	Utils.resolve_property_path(node, "position", cache)
	assert_eq(cache.size(), 1)
	Utils.resolve_property_path(node, "rotation", cache)
	assert_eq(cache.size(), 1)

	# ... including in the middle of a path.
	resolved = Utils.resolve_property_path(node, "no_such_prop:x")
	assert_string_contains(resolved.get("error", ""), "no property named 'no_such_prop'")

	# Metadata properties can be set before they exist.
	resolved = Utils.resolve_property_path(node, "metadata/my_meta")
	assert_true(resolved.has("value"))
	assert_eq(resolved.get("expected_type"), TYPE_NIL)

	# A path through a resource property.
	var mesh_instance: MeshInstance3D = autofree(MeshInstance3D.new())

	resolved = Utils.resolve_property_path(mesh_instance, "mesh:radius")
	assert_string_contains(resolved.get("error", ""), "'mesh' is null")

	var mesh := SphereMesh.new()
	mesh.radius = 2.0
	mesh_instance.mesh = mesh

	resolved = Utils.resolve_property_path(mesh_instance, "mesh:radius")
	assert_eq(resolved.get("value"), 2.0)
	assert_eq(resolved.get("expected_type"), TYPE_FLOAT)

	resolved = Utils.resolve_property_path(mesh_instance, "mesh:no_such_prop")
	assert_string_contains(resolved.get("error", ""), "SphereMesh has no property named 'no_such_prop'")

	# The declared hint and usage come along, e.g. for enum-hinted,
	# restart-if-changed project settings.
	resolved = Utils.resolve_property_path(ProjectSettings, "rendering/renderer/rendering_method")
	assert_eq(resolved.get("expected_type"), TYPE_STRING)
	assert_eq(resolved.get("hint"), PROPERTY_HINT_ENUM)
	assert_string_contains(resolved.get("hint_string", ""), "gl_compatibility")
	assert_true(resolved.get("usage", 0) & PROPERTY_USAGE_RESTART_IF_CHANGED != 0)


func test_check_enum_value() -> void:
	assert_eq(Utils.check_enum_value("gl_compatibility", "forward_plus,mobile,gl_compatibility"), "")

	var error := Utils.check_enum_value("compatibility", "forward_plus,mobile,gl_compatibility")
	assert_string_contains(error, "'compatibility' is not one of the valid values")
	assert_string_contains(error, "did you mean 'gl_compatibility'")
	assert_string_contains(error, "Valid values: forward_plus, mobile, gl_compatibility")

	# Hint strings can pair each name with an explicit value.
	assert_eq(Utils.check_enum_value("Two", "One:1,Two:2"), "")
	assert_string_contains(Utils.check_enum_value("Three", "One:1,Two:2"), "Valid values: One, Two")

	# Nothing close enough: no suggestion, just the valid values.
	error = Utils.check_enum_value("xyz", "forward_plus,mobile,gl_compatibility")
	assert_false(error.contains("did you mean"))
	assert_string_contains(error, "Valid values:")


func test_parse_enum_hint() -> void:
	assert_eq(Utils.parse_enum_hint(""), [])
	assert_eq(Utils.parse_enum_hint("Slow,Fast"), [
		{ name = "Slow", value = 0 },
		{ name = "Fast", value = 1 },
	])
	# An option without an explicit value takes the previous value plus one.
	assert_eq(Utils.parse_enum_hint("Low:1,High:10,Higher"), [
		{ name = "Low", value = 1 },
		{ name = "High", value = 10 },
		{ name = "Higher", value = 11 },
	])


func test_enum_value_to_name() -> void:
	assert_eq(Utils.enum_value_to_name(1, "Slow,Fast"), "Fast")
	assert_eq(Utils.enum_value_to_name(10, "Low:1,High:10"), "High")
	assert_eq(Utils.enum_value_to_name(2, "Low:1,High:10"), "")


func test_int_enum_name_to_value() -> void:
	assert_eq(Utils.int_enum_name_to_value("Fast", "Slow,Fast"), { value = 1 })
	assert_eq(Utils.int_enum_name_to_value("High", "Low:1,High:10"), { value = 10 })

	# A match ignoring case is accepted, reporting the canonical name.
	assert_eq(Utils.int_enum_name_to_value("fast", "Slow,Fast"), { value = 1, matched_name = "Fast" })

	var result := Utils.int_enum_name_to_value("Fastt", "Slow,Fast")
	assert_string_contains(result.get("error", ""), "'Fastt' is not one of the valid values")
	assert_string_contains(result.get("error", ""), "did you mean 'Fast'")
	assert_string_contains(result.get("error", ""), "Valid values: 'Slow' (0), 'Fast' (1)")


func test_enum_translation_note() -> void:
	var note := Utils.enum_translation_note(PackedStringArray(["a", "b"]))
	assert_string_contains(note, "(a, b)")
	assert_string_contains(note, '"enums_as_ints": true')

	# A long list is summarized rather than spelled out.
	note = Utils.enum_translation_note(PackedStringArray(["a", "b", "c", "d", "e"]))
	assert_false(note.contains("a, b"))
	assert_string_contains(note, "Some integer enum values")
	assert_string_contains(note, '"enums_as_ints": true')


func test_encode_property_value_for_hint() -> void:
	assert_eq(Utils.encode_property_value_for_hint(1, PROPERTY_HINT_ENUM, "Slow,Fast"), "Fast")
	# No matching option, or not an int enum at all: plain encoding.
	assert_eq(Utils.encode_property_value_for_hint(7, PROPERTY_HINT_ENUM, "Slow,Fast"), "7")
	assert_eq(Utils.encode_property_value_for_hint(1, PROPERTY_HINT_NONE, ""), "1")
	assert_eq(Utils.encode_property_value_for_hint("Fast", PROPERTY_HINT_ENUM, "Slow,Fast"), "Fast")


func test_get_property_default_value() -> void:
	assert_eq(Utils.get_default_property_value(autofree(Node2D.new()), "position"), Vector2(0, 0))

	var with_script: Node = autofree(Node.new())
	with_script.set_script(load("res://tests/gut/fixtures/script_with_default.gd"))
	assert_eq(Utils.get_default_property_value(with_script, "my_value"), 42)

	# Godot tracks no default for 'scene_file_path', so it falls back to the
	# zero value of the given type.
	var node: Node = autofree(Node.new())
	assert_null(Utils.get_default_property_value(node, "scene_file_path"))
	assert_eq(Utils.get_default_property_value(node, "scene_file_path", TYPE_STRING), "")


func test_get_property_map() -> void:
	var node: Node3D = autofree(Node3D.new())
	node.name = "MyNode"
	node.position = Vector3(1, 2, 3)

	var modified := Utils.get_property_map(node, true)
	assert_eq(modified.get("position"), "Vector3(1, 2, 3)")
	assert_eq(modified.get("name"), "MyNode")
	# At its default, so left out...
	assert_false(modified.has("visible"))
	# ... as is an unset property Godot tracks no default for.
	assert_false(modified.has("scene_file_path"))

	var all := Utils.get_property_map(node, false)
	assert_true(all.has("visible"))

	# Properties Godot neither saves nor shows in the inspector are left out of
	# both: they're derived from other properties, or unrelated to the node's
	# own state.
	for prop_name in ["global_position", "global_transform", "basis", "quaternion", "rotation_degrees", "multiplayer", "owner"]:
		assert_false(modified.has(prop_name), "'%s' should be left out" % prop_name)
		assert_false(all.has(prop_name), "'%s' should be left out" % prop_name)


func test_get_property_map_translates_int_enums() -> void:
	var node: Node = autofree(Node.new())
	node.set_script(load("res://tests/gut/fixtures/script_with_enum.gd"))
	node.speed_mode = 2
	node.power = 10

	var translated := PackedStringArray()
	var props := Utils.get_property_map(node, true, false, translated)
	assert_eq(props.get("speed_mode"), "Fast")
	assert_eq(props.get("power"), "High")
	assert_true("speed_mode" in translated)
	assert_true("power" in translated)

	var raw := Utils.get_property_map(node, true, true)
	assert_eq(raw.get("speed_mode"), "2")
	assert_eq(raw.get("power"), "10")

	# A value no option covers stays an int.
	node.power = 3
	translated = PackedStringArray()
	props = Utils.get_property_map(node, true, false, translated)
	assert_eq(props.get("power"), "3")
	assert_false("power" in translated)


func test_script_read_tracking() -> void:
	var path := "res://some/script.gd"
	Utils.clear_script_reads()

	# Without a prior read, writing is refused.
	var result := Utils.check_script_writable(path, "extends Node\n")
	assert_true(result.has("error"))
	assert_string_contains(result.get("error", ""), "must read")

	# After recording a read, the same content is writable...
	Utils.record_script_read(path, "extends Node\n")
	assert_false(Utils.check_script_writable(path, "extends Node\n").has("error"))

	# ... but changed content is refused.
	result = Utils.check_script_writable(path, "extends Node\n\nvar changed := true\n")
	assert_true(result.has("error"))
	assert_string_contains(result.get("error", ""), "has changed")

	# Clearing forgets the read (as on client disconnect), so it requires a
	# fresh read again.
	Utils.clear_script_reads()
	result = Utils.check_script_writable(path, "extends Node\n")
	assert_true(result.has("error"))
	assert_string_contains(result.get("error", ""), "must read")
