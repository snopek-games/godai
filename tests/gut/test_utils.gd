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
