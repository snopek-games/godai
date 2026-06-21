extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(NodeGetProperties.new(p_data["get_node_properties"]))
	p_tools.register_tool(NodeSetProperties.new(p_data["set_node_properties"]))
	p_tools.register_tool(NodeAdd.new(p_data["add_node"]))
	p_tools.register_tool(NodeRemove.new(p_data["remove_node"]))
	p_tools.register_tool(NodeAddToGroup.new(p_data["add_to_group"]))
	p_tools.register_tool(NodeRemoveFromGroup.new(p_data["remove_from_group"]))
	p_tools.register_tool(NodeGetGroups.new(p_data["get_node_groups"]))
	p_tools.register_tool(NodeConnectSignal.new(p_data["connect_signal"]))
	p_tools.register_tool(NodeDisconnectSignal.new(p_data["disconnect_signal"]))
	p_tools.register_tool(NodeAttachScript.new(p_data["attach_script"]))
	p_tools.register_tool(NodeDetachScript.new(p_data["detach_script"]))


class NodeGetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.resolved({})

		var node_paths: Array = p_input.get("node_paths", [])
		var modified_only: bool = p_input.get("modified_only", false)
		var results := {}

		for node_path in node_paths:
			# The node path can carry a colon-separated property path (e.g.
			# "Child:mesh") to inspect a resource or other sub-object.
			var colon: int = node_path.find(":")
			var base_path: String = node_path if colon == -1 else node_path.substr(0, colon)
			var property_path: String = "" if colon == -1 else node_path.substr(colon + 1)
			if base_path.is_empty():
				base_path = "."

			var node := edited_scene_root.get_node_or_null(base_path)
			if not node:
				results[node_path] = {}
				continue

			if property_path.is_empty():
				results[node_path] = Utils.get_property_map(node, modified_only)
				continue

			var resolved := Utils.resolve_property_path(node, property_path)
			if resolved.has("error"):
				results[node_path] = { error = resolved['error'] }
			elif resolved['value'] is Object:
				results[node_path] = Utils.get_property_map(resolved['value'], modified_only)
			else:
				results[node_path] = Utils.encode_property_value(resolved['value'])

		return ToolResult.resolved(results)


class NodeSetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var action: String = p_input.get('action', '')
		var edits: Array = p_input.get('nodes', [])

		if action.is_empty():
			return ToolResult.rejected({error = "'action' is required"})

		var results := {}
		var errors := PackedStringArray()
		var ops := []

		# Resolve and decode everything up front: if any property is invalid,
		# we reject the whole call without changing anything.
		for edit in edits:
			var node_path: String = edit['node_path']
			var props: Dictionary = edit['properties']

			# The node path can carry a colon-separated property path (e.g.
			# "Child:mesh") to address a resource or other sub-object.
			var colon: int = node_path.find(":")
			var base_path: String = node_path if colon == -1 else node_path.substr(0, colon)
			var path_prefix: String = "" if colon == -1 else node_path.substr(colon + 1)
			if base_path.is_empty():
				base_path = "."

			var node := edited_scene_root.get_node_or_null(base_path)
			if not node:
				results[node_path] = false
				continue
			results[node_path] = true

			for prop_name in props:
				var full_path: String = prop_name if path_prefix.is_empty() else path_prefix + ":" + prop_name

				var resolved := Utils.resolve_property_path(node, full_path)
				if resolved.has("error"):
					errors.append("%s / %s: %s" % [node_path, prop_name, resolved['error']])
					continue

				var decoded := Utils.decode_property_value(props[prop_name], resolved['expected_type'])
				if decoded.has("error"):
					errors.append("%s / %s: %s" % [node_path, prop_name, decoded['error']])
					continue

				ops.append({
					node = node,
					path = full_path,
					value = decoded['value'],
					old_value = resolved['value'],
				})

		if not errors.is_empty():
			return ToolResult.rejected({error = "No properties were changed, due to the following errors:\n" + "\n".join(errors)})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("%s (AI)" % action)

		for op in ops:
			if op['path'].contains(":"):
				# A colon path: UndoRedo's do/undo properties use plain set(),
				# so go through set_indexed() instead.
				undo_redo.add_do_method(op['node'], "set_indexed", op['path'], op['value'])
				undo_redo.add_undo_method(op['node'], "set_indexed", op['path'], op['old_value'])
			else:
				undo_redo.add_do_property(op['node'], op['path'], op['value'])
				undo_redo.add_undo_property(op['node'], op['path'], op['old_value'])

		undo_redo.commit_action()

		return ToolResult.resolved(results)


class NodeAdd extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var parent_path: String = p_input.get('parent_path', '')
		var node_type: String = p_input.get('node_type', '')
		var props: Dictionary = p_input.get('properties', {})

		if parent_path.is_empty():
			return ToolResult.rejected({error = "'parent_path' is required"})
		if node_type.is_empty():
			return ToolResult.rejected({error = "'node_type' is required"})

		var parent = edited_scene_root.get_node_or_null(parent_path)
		if not parent:
			return ToolResult.rejected({error = "Cannot find node at 'parent_path': %s" % parent_path})

		var node = ClassDB.instantiate(node_type)
		if not node:
			return ToolResult.rejected({error = "Failed to create '%s'" % node_type})
		if not node is Node:
			if not node is RefCounted:
				node.free()
			return ToolResult.rejected({error = "'%s' is not a Node type" % node_type})

		# Resolve and decode all the property values before touching the
		# scene: if any property is invalid, we reject the whole call.
		var ops := []
		var errors := PackedStringArray()
		for prop_name in props:
			var resolved := Utils.resolve_property_path(node, prop_name)
			if resolved.has("error"):
				errors.append("%s: %s" % [prop_name, resolved['error']])
				continue

			var decoded := Utils.decode_property_value(props[prop_name], resolved['expected_type'])
			if decoded.has("error"):
				errors.append("%s: %s" % [prop_name, decoded['error']])
				continue

			ops.append({path = prop_name, value = decoded['value']})

		if not errors.is_empty():
			node.free()
			return ToolResult.rejected({error = "Node wasn't created, due to the following errors:\n" + "\n".join(errors)})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Create %s node (AI)" % node_type)
		Utils.editor_undo_redo_create_node(undo_redo, parent, node)

		for op in ops:
			if op['path'].contains(":"):
				undo_redo.add_do_method(node, "set_indexed", op['path'], op['value'])
			else:
				undo_redo.add_do_property(node, op['path'], op['value'])

		undo_redo.commit_action()

		return ToolResult.resolved({
			success = true,
			node_path = edited_scene_root.get_path_to(node),
		})


class NodeRemove extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var node_path: String = p_input.get('node_path', '')
		if node_path.is_empty():
			return ToolResult.rejected({error = "'node_path' is required"})

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({error = "Cannot find node at 'node_path': %s" % node_path})

		if node == edited_scene_root:
			return ToolResult.rejected({error = "Cannot remove scene root"})

		var parent = node.get_parent()

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Delete %s node (AI)" % node.name)
		Utils.editor_undo_redo_remove_node(undo_redo, parent, node)

		undo_redo.commit_action()

		return ToolResult.resolved({
			success = true,
		})


class NodeAddToGroup extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var node_path: String = p_input.get('node_path', '')
		var groups: Array = p_input.get('groups', [])

		if node_path.is_empty():
			return ToolResult.rejected({error = "'node_path' is required"})
		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({error = "Cannot find node at 'node_path': %s" % node_path})
		if groups.is_empty():
			return ToolResult.rejected({error = "'groups' is required"})

		# Only add groups the node isn't already in, so undo doesn't remove a
		# pre-existing membership.
		var to_add := []
		for group in groups:
			if not node.is_in_group(group):
				to_add.append(group)
		if to_add.is_empty():
			return ToolResult.resolved({success = true})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Add to group(s) (AI)")
		for group in to_add:
			# The second argument makes the membership persistent (saved to the
			# scene file).
			undo_redo.add_do_method(node, "add_to_group", group, true)
			undo_redo.add_undo_method(node, "remove_from_group", group)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true})


class NodeRemoveFromGroup extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var node_path: String = p_input.get('node_path', '')
		var groups: Array = p_input.get('groups', [])

		if node_path.is_empty():
			return ToolResult.rejected({error = "'node_path' is required"})
		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({error = "Cannot find node at 'node_path': %s" % node_path})
		if groups.is_empty():
			return ToolResult.rejected({error = "'groups' is required"})

		# Only remove groups the node is actually in.
		var to_remove := []
		for group in groups:
			if node.is_in_group(group):
				to_remove.append(group)
		if to_remove.is_empty():
			return ToolResult.resolved({success = true})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Remove from group(s) (AI)")
		for group in to_remove:
			undo_redo.add_do_method(node, "remove_from_group", group)
			undo_redo.add_undo_method(node, "add_to_group", group, true)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true})


class NodeGetGroups extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.resolved({})

		var node_paths: Array = p_input.get('node_paths', [])

		var results := {}
		for node_path in node_paths:
			var node = edited_scene_root.get_node_or_null(node_path)
			if not node:
				results[node_path] = []
				continue

			var groups := []
			for group in node.get_groups():
				# Skip internal groups (the engine prefixes those with "_").
				if str(group).begins_with("_"):
					continue
				groups.push_back(str(group))
			results[node_path] = groups

		return ToolResult.resolved(results)


class NodeConnectSignal extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var from_path: String = p_input.get('from_node', '')
		var signal_name: String = p_input.get('signal', '')
		var to_path: String = p_input.get('to_node', '')
		var method: String = p_input.get('method', '')

		if from_path.is_empty():
			return ToolResult.rejected({error = "'from_node' is required"})
		if signal_name.is_empty():
			return ToolResult.rejected({error = "'signal' is required"})
		if to_path.is_empty():
			return ToolResult.rejected({error = "'to_node' is required"})
		if method.is_empty():
			return ToolResult.rejected({error = "'method' is required"})

		var from_node = edited_scene_root.get_node_or_null(from_path)
		if not from_node:
			return ToolResult.rejected({error = "Cannot find 'from_node': %s" % from_path})
		var to_node = edited_scene_root.get_node_or_null(to_path)
		if not to_node:
			return ToolResult.rejected({error = "Cannot find 'to_node': %s" % to_path})

		if not from_node.has_signal(signal_name):
			return ToolResult.rejected({error = "%s has no signal named '%s'" % [from_node.get_class(), signal_name]})
		if not to_node.has_method(method):
			return ToolResult.rejected({error = "%s has no method named '%s'" % [to_node.get_class(), method]})

		var callable := Callable(to_node, method)
		if from_node.is_connected(signal_name, callable):
			return ToolResult.rejected({error = "'%s' is already connected to %s.%s" % [signal_name, to_path, method]})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Connect signal '%s' (AI)" % signal_name)
		# CONNECT_PERSIST makes the connection part of the saved scene.
		undo_redo.add_do_method(from_node, "connect", signal_name, callable, CONNECT_PERSIST)
		undo_redo.add_undo_method(from_node, "disconnect", signal_name, callable)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true})


class NodeDisconnectSignal extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var from_path: String = p_input.get('from_node', '')
		var signal_name: String = p_input.get('signal', '')
		var to_path: String = p_input.get('to_node', '')
		var method: String = p_input.get('method', '')

		if from_path.is_empty():
			return ToolResult.rejected({error = "'from_node' is required"})
		if signal_name.is_empty():
			return ToolResult.rejected({error = "'signal' is required"})
		if to_path.is_empty():
			return ToolResult.rejected({error = "'to_node' is required"})
		if method.is_empty():
			return ToolResult.rejected({error = "'method' is required"})

		var from_node = edited_scene_root.get_node_or_null(from_path)
		if not from_node:
			return ToolResult.rejected({error = "Cannot find 'from_node': %s" % from_path})
		var to_node = edited_scene_root.get_node_or_null(to_path)
		if not to_node:
			return ToolResult.rejected({error = "Cannot find 'to_node': %s" % to_path})

		var callable := Callable(to_node, method)
		if not from_node.is_connected(signal_name, callable):
			return ToolResult.rejected({error = "'%s' is not connected to %s.%s" % [signal_name, to_path, method]})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Disconnect signal '%s' (AI)" % signal_name)
		undo_redo.add_do_method(from_node, "disconnect", signal_name, callable)
		undo_redo.add_undo_method(from_node, "connect", signal_name, callable, CONNECT_PERSIST)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true})


class NodeAttachScript extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var node_path: String = p_input.get('node_path', '')
		var script_path: String = p_input.get('script_path', '')

		if node_path.is_empty():
			return ToolResult.rejected({error = "'node_path' is required"})
		if script_path.is_empty():
			return ToolResult.rejected({error = "'script_path' is required"})
		script_path = Utils.to_res_path(script_path)

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({error = "Cannot find node at 'node_path': %s" % node_path})
		if not FileAccess.file_exists(script_path):
			return ToolResult.rejected({error = "'%s' doesn't exist" % script_path})

		var script = load(script_path)
		if not script is Script:
			return ToolResult.rejected({error = "'%s' is not a script" % script_path})

		# Make sure the script's base type is compatible with the node.
		var base_type: String = script.get_instance_base_type()
		if base_type != "" and not ClassDB.is_parent_class(node.get_class(), base_type):
			return ToolResult.rejected({error = "Script extends '%s', which is not compatible with a node of type '%s'" % [base_type, node.get_class()]})

		var old_script = node.get_script()

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Attach script (AI)")
		undo_redo.add_do_property(node, "script", script)
		undo_redo.add_undo_property(node, "script", old_script)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true})


class NodeDetachScript extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({error = "No scene open"})

		var node_path: String = p_input.get('node_path', '')
		if node_path.is_empty():
			return ToolResult.rejected({error = "'node_path' is required"})

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({error = "Cannot find node at 'node_path': %s" % node_path})

		var old_script = node.get_script()
		if not old_script:
			return ToolResult.rejected({error = "Node '%s' has no script attached" % node_path})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Detach script (AI)")
		undo_redo.add_do_property(node, "script", null)
		undo_redo.add_undo_property(node, "script", old_script)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true})
