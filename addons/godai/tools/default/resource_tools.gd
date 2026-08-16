extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")
const VerifiedPropertyTool = preload("res://addons/godai/tools/default/verified_property_tool.gd")


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(ResourceCreate.new(p_data["create_resource"]))
	p_tools.register_tool(ResourceOpen.new(p_data["open_resource"]))
	p_tools.register_tool(ResourceGetProperties.new(p_data["get_resource_properties"]))
	p_tools.register_tool(ResourceSetProperties.new(p_data["set_resource_properties"]))


class ResourceCreate extends VerifiedPropertyTool:
	func get_properties_tool_name() -> String:
		return "get_resource_properties"

	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var resource_type: String = p_input.get('resource_type', '')
		var props: Dictionary = p_input.get('properties', {})

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' already exists" % file_path]})

		var resource

		if ClassDB.class_exists(resource_type):
			resource = ClassDB.instantiate(resource_type)
		else:
			var global_script_classes := ProjectSettings.get_global_class_list()
			var info: Dictionary
			for gsc in global_script_classes:
				if gsc['class'] == resource_type:
					info = gsc
					break

			if info.is_empty():
				return ToolResult.rejected({errors = ["Unknown 'resource_type': %s" % resource_type]})

			resource = ClassDB.instantiate(info['base'])
			resource.script = load(info['path'])

		if not resource:
			return ToolResult.rejected({errors = ["Failed to create '%s'" % resource_type]})
		if not resource is Resource:
			if not resource is RefCounted:
				resource.free()
			return ToolResult.rejected({errors = ["'%s' is not a Resource type" % resource_type]})

		logger.start()

		# The resource is still created when a property fails: each one is
		# prepared and set independently. Property problems are warnings, not
		# errors, so 'success' reflects the resource's creation.
		var ops := []
		var warnings := PackedStringArray()
		var notes := PackedStringArray()
		var prop_cache := {}
		for prop_name in props:
			var prepared := prepare_property_op(resource, prop_name, props[prop_name], prop_cache)
			if prepared.has("error"):
				warnings.append("%s: %s" % [prop_name, prepared['error']])
				continue
			collect_prepared_notices(prepared, prop_name, notes, warnings)

			var op: Dictionary = prepared['op']
			op['object'] = resource
			op['label'] = prop_name
			ops.append(op)

		for op in ops:
			if op['path'].contains(":"):
				resource.set_indexed(op['path'], op['value'])
			else:
				resource.set(op['path'], op['value'])

		var dir_path = file_path.get_base_dir()
		if not DirAccess.dir_exists_absolute(dir_path):
			var err = DirAccess.make_dir_recursive_absolute(dir_path)
			if err != OK:
				return ToolResult.rejected(build_rejection(PackedStringArray(["Failed to make parent directory '%s': %s" % [dir_path, error_string(err)]])))

		var err = ResourceSaver.save(resource, file_path)
		if err != OK:
			return ToolResult.rejected(build_rejection(PackedStringArray(["Failed to save resource '%s': %s" % [file_path, error_string(err)]])))

		verify_property_ops(ops, warnings, warnings)

		return ToolResult.resolved(build_result({}, PackedStringArray(), warnings, notes))


class ResourceOpen extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		var resource = load(file_path)
		if not resource:
			return ToolResult.rejected({errors = ["Failed to load resource: %s" % file_path]})

		EditorInterface.edit_resource(resource)

		return ToolResult.resolved({success = true})


class ResourceGetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var properties: Array = p_input.get('properties', [])
		var modified_only: bool = not bool(p_input.get('include_defaults', false))
		var enums_as_ints: bool = bool(p_input.get('enums_as_ints', false))

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		# Skip past the colon in "res://".
		if file_path.find(":", 6) != -1:
			return ToolResult.rejected({errors = ["'file_path' must point at just the file - request sub-properties via the 'properties' input (e.g. \"albedo_color:r\")"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		var resource = load(file_path)
		if not resource:
			return ToolResult.rejected({errors = ["Failed to load resource: %s" % file_path]})

		var results := {}
		var translated := PackedStringArray()
		var prop_cache := {}

		if properties.is_empty():
			results = Utils.get_property_map(resource, modified_only, enums_as_ints, translated)
		else:
			for property_path in properties:
				var resolved := Utils.resolve_property_path(resource, property_path, prop_cache)
				if resolved.has("error"):
					results[property_path] = { error = resolved['error'] }
				elif resolved['value'] is Object:
					var local := PackedStringArray()
					results[property_path] = Utils.get_property_map(resolved['value'], modified_only, enums_as_ints, local)
					for prop_name in local:
						translated.append("%s:%s" % [property_path, prop_name])
				else:
					var encoded := Utils.encode_resolved_value(resolved, enums_as_ints)
					results[property_path] = encoded['value']
					if encoded['translated']:
						translated.append(property_path)

		var result := { properties = results }
		if not translated.is_empty():
			result['notes'] = [Utils.enum_translation_note(translated)]
		return ToolResult.resolved(result)


class ResourceSetProperties extends VerifiedPropertyTool:
	func get_properties_tool_name() -> String:
		return "get_resource_properties"

	func execute(p_input) -> ToolResult:
		var action: String = p_input.get('action', '')
		var file_path: String = p_input.get('file_path', '')
		var props: Dictionary = p_input.get('properties', {})

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		# Skip past the colon in "res://".
		if file_path.find(":", 6) != -1:
			return ToolResult.rejected({errors = ["'file_path' must point at just the file - set sub-properties via colon-separated keys in 'properties' (e.g. \"albedo_color:r\")"]})

		# Only file types that ResourceSaver can write back.
		if not file_path.get_extension() in ["tres", "res"]:
			return ToolResult.rejected({errors = ["'%s' must be a .tres or .res file" % file_path]})

		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		var resource = load(file_path)
		if not resource:
			return ToolResult.rejected({errors = ["Failed to load resource: %s" % file_path]})

		var errors := PackedStringArray()
		var warnings := PackedStringArray()
		var notes := PackedStringArray()
		var ops := []
		var prop_cache := {}

		logger.start()

		# Each property is prepared and set independently: one bad property
		# doesn't stop the others from being attempted.
		for prop_name in props:
			var prepared := prepare_property_op(resource, prop_name, props[prop_name], prop_cache)
			if prepared.has("error"):
				errors.append("%s: %s" % [prop_name, prepared['error']])
				continue
			collect_prepared_notices(prepared, prop_name, notes, warnings)

			var op: Dictionary = prepared['op']
			op['object'] = resource
			op['label'] = prop_name
			ops.append(op)

		if ops.is_empty() and not errors.is_empty():
			return ToolResult.rejected(build_rejection(errors))

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("%s (Godai)" % action)

		for op in ops:
			if op['path'].contains(":"):
				# A colon path: UndoRedo's do/undo properties use plain set(),
				# so go through set_indexed() instead.
				undo_redo.add_do_method(resource, "set_indexed", op['path'], op['value'])
				undo_redo.add_undo_method(resource, "set_indexed", op['path'], op['old_value'])
			else:
				undo_redo.add_do_property(resource, op['path'], op['value'])
				undo_redo.add_undo_property(resource, op['path'], op['old_value'])

		# Save the resource back to its file. Operations run in the order
		# they were added, so on both do and undo this happens after the
		# property changes - the file always matches the in-memory state.
		undo_redo.add_do_method(self, "_save_resource", resource)
		undo_redo.add_undo_method(self, "_save_resource", resource)
		# Keep the resource alive while the action is in the undo history.
		undo_redo.add_do_reference(resource)

		undo_redo.commit_action()

		verify_property_ops(ops, errors, warnings)

		return ToolResult.resolved(build_result({}, errors, warnings, notes))

	func _save_resource(p_resource: Resource) -> void:
		var err := ResourceSaver.save(p_resource)
		if err != OK:
			push_error("Failed to save resource '%s': %s" % [p_resource.resource_path, error_string(err)])
