extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(ResourceCreate.new(p_data["create_resource"]))
	p_tools.register_tool(ResourceOpen.new(p_data["open_resource"]))
	p_tools.register_tool(ResourceGetProperties.new(p_data["get_resource_properties"]))
	p_tools.register_tool(ResourceSetProperties.new(p_data["set_resource_properties"]))


class ResourceCreate extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var resource_type: String = p_input.get('resource_type', '')
		var props: Dictionary = p_input.get('properties', {})

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({error = "'file_path' must be inside the project (res://)"})
		if FileAccess.file_exists(file_path):
			return ToolResult.rejected({error = "'%s' already exists" % file_path})

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
				return ToolResult.rejected({error = "Unknown 'resource_type': %s" % resource_type})

			resource = ClassDB.instantiate(info['base'])
			resource.script = load(info['path'])

		if not resource:
			return ToolResult.rejected({error = "Failed to create '%s'" % resource_type})
		if not resource is Resource:
			if not resource is RefCounted:
				resource.free()
			return ToolResult.rejected({error = "'%s' is not a Resource type" % resource_type})

		var errors := PackedStringArray()
		for prop_name in props:
			var resolved := Utils.resolve_property_path(resource, prop_name)
			if resolved.has("error"):
				errors.append("%s: %s" % [prop_name, resolved['error']])
				continue

			var decoded := Utils.decode_property_value(props[prop_name], resolved['expected_type'])
			if decoded.has("error"):
				errors.append("%s: %s" % [prop_name, decoded['error']])
				continue

			if prop_name.contains(":"):
				resource.set_indexed(prop_name, decoded['value'])
			else:
				resource.set(prop_name, decoded['value'])

		if not errors.is_empty():
			return ToolResult.rejected({error = "Resource wasn't created, due to the following errors:\n" + "\n".join(errors)})

		var dir_path = file_path.get_base_dir()
		if not DirAccess.dir_exists_absolute(dir_path):
			var err = DirAccess.make_dir_recursive_absolute(dir_path)
			if err != OK:
				return ToolResult.rejected({error = "Failed to make parent directory '%s': %s" % [dir_path, error_string(err)]})

		var err = ResourceSaver.save(resource, file_path)
		if err != OK:
			return ToolResult.rejected({error = "Failed to save resource '%s': %s" % [file_path, error_string(err)]})

		return ToolResult.resolved({success = true})


class ResourceOpen extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({error = "'file_path' must be inside the project (res://)"})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({error = "'%s' doesn't exist" % file_path})

		var resource = load(file_path)
		if not resource:
			return ToolResult.rejected({error = "Failed to load resource: %s" % file_path})

		EditorInterface.edit_resource(resource)

		return ToolResult.resolved({success = true})


class ResourceGetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var properties: Array = p_input.get('properties', [])
		var modified_only: bool = not bool(p_input.get('include_defaults', false))

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({error = "'file_path' must be inside the project (res://)"})
		# Skip past the colon in "res://".
		if file_path.find(":", 6) != -1:
			return ToolResult.rejected({error = "'file_path' must point at just the file - request sub-properties via the 'properties' input (e.g. \"albedo_color:r\")"})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({error = "'%s' doesn't exist" % file_path})

		var resource = load(file_path)
		if not resource:
			return ToolResult.rejected({error = "Failed to load resource: %s" % file_path})

		if properties.is_empty():
			return ToolResult.resolved(Utils.get_property_map(resource, modified_only))

		var results := {}

		for property_path in properties:
			var resolved := Utils.resolve_property_path(resource, property_path)
			if resolved.has("error"):
				results[property_path] = { error = resolved['error'] }
			elif resolved['value'] is Object:
				results[property_path] = Utils.get_property_map(resolved['value'], modified_only)
			else:
				results[property_path] = Utils.encode_property_value(resolved['value'])

		return ToolResult.resolved(results)


class ResourceSetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var action: String = p_input.get('action', '')
		var file_path: String = p_input.get('file_path', '')
		var props: Dictionary = p_input.get('properties', {})

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({error = "'file_path' must be inside the project (res://)"})
		# Skip past the colon in "res://".
		if file_path.find(":", 6) != -1:
			return ToolResult.rejected({error = "'file_path' must point at just the file - set sub-properties via colon-separated keys in 'properties' (e.g. \"albedo_color:r\")"})

		# Only file types that ResourceSaver can write back.
		if not file_path.get_extension() in ["tres", "res"]:
			return ToolResult.rejected({error = "'%s' must be a .tres or .res file" % file_path})

		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({error = "'%s' doesn't exist" % file_path})

		var resource = load(file_path)
		if not resource:
			return ToolResult.rejected({error = "Failed to load resource: %s" % file_path})

		# Resolve and decode everything up front: if any property is invalid,
		# we reject the whole call without changing anything.
		var errors := PackedStringArray()
		var ops := []

		for prop_name in props:
			var resolved := Utils.resolve_property_path(resource, prop_name)
			if resolved.has("error"):
				errors.append("%s: %s" % [prop_name, resolved['error']])
				continue

			var decoded := Utils.decode_property_value(props[prop_name], resolved['expected_type'])
			if decoded.has("error"):
				errors.append("%s: %s" % [prop_name, decoded['error']])
				continue

			ops.append({
				path = prop_name,
				value = decoded['value'],
				old_value = resolved['value'],
			})

		if not errors.is_empty():
			return ToolResult.rejected({error = "No properties were changed, due to the following errors:\n" + "\n".join(errors)})

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

		return ToolResult.resolved({success = true})

	func _save_resource(p_resource: Resource) -> void:
		var err := ResourceSaver.save(p_resource)
		if err != OK:
			push_error("Failed to save resource '%s': %s" % [p_resource.resource_path, error_string(err)])
