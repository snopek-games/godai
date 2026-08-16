extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")
const VerifiedPropertyTool = preload("res://addons/godai/tools/default/verified_property_tool.gd")

const UNSAVED_SCENE_NOTE := Utils.UNSAVED_SCENE_NOTE


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
		var node_paths: Array = p_input.get("node_paths", [])
		var modified_only: bool = not bool(p_input.get("include_defaults", false))
		var enums_as_ints: bool = bool(p_input.get("enums_as_ints", false))

		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		# Resolve all the nodes up front: if any node path is missing, we reject
		# the whole call.
		var targets := []
		var errors := PackedStringArray()

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
				errors.append("Cannot find node in 'node_paths': %s" % node_path)
				continue

			targets.append({
				node_path = node_path,
				node = node,
				property_path = property_path,
			})

		if not errors.is_empty():
			return ToolResult.rejected({errors = errors})

		var results := {}
		var translated := PackedStringArray()
		var prop_cache := {}

		for target in targets:
			var node_path: String = target['node_path']
			var property_path: String = target['property_path']

			if property_path.is_empty():
				var local := PackedStringArray()
				results[node_path] = Utils.get_property_map(target['node'], modified_only, enums_as_ints, local)
				for prop_name in local:
					translated.append("%s:%s" % [node_path, prop_name])
				continue

			var resolved := Utils.resolve_property_path(target['node'], property_path, prop_cache)
			if resolved.has("error"):
				results[node_path] = { error = resolved['error'] }
			elif resolved['value'] is Object:
				var local := PackedStringArray()
				results[node_path] = Utils.get_property_map(resolved['value'], modified_only, enums_as_ints, local)
				for prop_name in local:
					translated.append("%s:%s" % [node_path, prop_name])
			else:
				var encoded := Utils.encode_resolved_value(resolved, enums_as_ints)
				results[node_path] = encoded['value']
				if encoded['translated']:
					translated.append(node_path)

		var result := { nodes = results }
		if not translated.is_empty():
			result['notes'] = [Utils.enum_translation_note(translated)]
		return ToolResult.resolved(result)


class NodeSetProperties extends VerifiedPropertyTool:
	func get_properties_tool_name() -> String:
		return "get_node_properties"

	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var action: String = p_input.get('action', '')
		var edits: Dictionary = p_input.get('nodes', {})

		var errors := PackedStringArray()
		var warnings := PackedStringArray()
		var notes := PackedStringArray()
		var ops := []
		var prop_cache := {}

		logger.start()

		# Each property is prepared and set independently: one bad property
		# doesn't stop the others from being attempted.
		for node_path in edits:
			var props = edits[node_path]
			if not props is Dictionary:
				errors.append("%s: must map property names to values" % node_path)
				continue

			# The node path can carry a colon-separated property path (e.g.
			# "Child:mesh") to address a resource or other sub-object.
			var colon: int = node_path.find(":")
			var base_path: String = node_path if colon == -1 else node_path.substr(0, colon)
			var path_prefix: String = "" if colon == -1 else node_path.substr(colon + 1)
			if base_path.is_empty():
				base_path = "."

			var node := edited_scene_root.get_node_or_null(base_path)
			if not node:
				errors.append("%s: cannot find node" % node_path)
				continue

			for prop_name in props:
				var full_path: String = prop_name if path_prefix.is_empty() else path_prefix + ":" + prop_name

				var prepared := prepare_property_op(node, full_path, props[prop_name], prop_cache)
				if prepared.has("error"):
					errors.append("%s / %s: %s" % [node_path, prop_name, prepared['error']])
					continue
				collect_prepared_notices(prepared, "%s / %s" % [node_path, prop_name], notes, warnings)

				var op: Dictionary = prepared['op']
				op['object'] = node
				op['label'] = "%s / %s" % [node_path, prop_name]
				ops.append(op)

		if ops.is_empty() and not errors.is_empty():
			return ToolResult.rejected(build_rejection(errors))

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("%s (Godai)" % action)

		for op in ops:
			if op['path'].contains(":"):
				# A colon path: UndoRedo's do/undo properties use plain set(),
				# so go through set_indexed() instead.
				undo_redo.add_do_method(op['object'], "set_indexed", op['path'], op['value'])
				undo_redo.add_undo_method(op['object'], "set_indexed", op['path'], op['old_value'])
			else:
				undo_redo.add_do_property(op['object'], op['path'], op['value'])
				undo_redo.add_undo_property(op['object'], op['path'], op['old_value'])

		undo_redo.commit_action()

		verify_property_ops(ops, errors, warnings)

		return ToolResult.resolved(build_result({
			notes = [UNSAVED_SCENE_NOTE],
		}, errors, warnings, notes))


class NodeAdd extends VerifiedPropertyTool:
	func get_properties_tool_name() -> String:
		return "get_node_properties"

	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var parent_path: String = p_input.get('parent_path', '')
		var node_type: String = p_input.get('node_type', '')
		var props: Dictionary = p_input.get('properties', {})

		var parent = edited_scene_root.get_node_or_null(parent_path)
		if not parent:
			return ToolResult.rejected({errors = ["Cannot find node at 'parent_path': %s" % parent_path]})

		var node = ClassDB.instantiate(node_type)
		if not node:
			return ToolResult.rejected({errors = ["Failed to create '%s'" % node_type]})
		if not node is Node:
			if not node is RefCounted:
				node.free()
			return ToolResult.rejected({errors = ["'%s' is not a Node type" % node_type]})

		logger.start()

		# The node is created even when a property fails: each one is prepared
		# and set independently. Property problems are warnings, not errors, so
		# 'success' reflects the node's creation - retrying the whole call would
		# create a duplicate node.
		var ops := []
		var warnings := PackedStringArray()
		var notes := PackedStringArray()
		var prop_cache := {}
		for prop_name in props:
			var prepared := prepare_property_op(node, prop_name, props[prop_name], prop_cache)
			if prepared.has("error"):
				warnings.append("%s: %s" % [prop_name, prepared['error']])
				continue
			collect_prepared_notices(prepared, prop_name, notes, warnings)

			var op: Dictionary = prepared['op']
			op['object'] = node
			op['label'] = prop_name
			ops.append(op)

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Create %s node (Godai)" % node_type)
		Utils.editor_undo_redo_create_node(undo_redo, parent, node)

		for op in ops:
			if op['path'].contains(":"):
				undo_redo.add_do_method(node, "set_indexed", op['path'], op['value'])
			else:
				undo_redo.add_do_property(node, op['path'], op['value'])

		undo_redo.commit_action()

		verify_property_ops(ops, warnings, warnings)

		return ToolResult.resolved(build_result({
			node_path = str(edited_scene_root.get_path_to(node)),
			notes = [UNSAVED_SCENE_NOTE],
		}, PackedStringArray(), warnings, notes))


class NodeRemove extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var node_path: String = p_input.get('node_path', '')

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({errors = ["Cannot find node at 'node_path': %s" % node_path]})

		if node == edited_scene_root:
			return ToolResult.rejected({errors = ["Cannot remove scene root"]})

		var parent = node.get_parent()

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Delete %s node (Godai)" % node.name)
		Utils.editor_undo_redo_remove_node(undo_redo, parent, node)

		undo_redo.commit_action()

		return ToolResult.resolved({
			success = true,
			notes = [UNSAVED_SCENE_NOTE],
		})


class NodeAddToGroup extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var node_path: String = p_input.get('node_path', '')
		var groups: Array = p_input.get('groups', [])

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({errors = ["Cannot find node at 'node_path': %s" % node_path]})

		# Only add groups the node isn't already in, so undo doesn't remove a
		# pre-existing membership.
		var to_add := []
		for group in groups:
			if not node.is_in_group(group):
				to_add.append(group)
		if to_add.is_empty():
			return ToolResult.resolved({success = true})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Add to group(s) (Godai)")
		for group in to_add:
			# The second argument makes the membership persistent (saved to the
			# scene file).
			undo_redo.add_do_method(node, "add_to_group", group, true)
			undo_redo.add_undo_method(node, "remove_from_group", group)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true, notes = [UNSAVED_SCENE_NOTE]})


class NodeRemoveFromGroup extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var node_path: String = p_input.get('node_path', '')
		var groups: Array = p_input.get('groups', [])

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({errors = ["Cannot find node at 'node_path': %s" % node_path]})

		# Only remove groups the node is actually in.
		var to_remove := []
		for group in groups:
			if node.is_in_group(group):
				to_remove.append(group)
		if to_remove.is_empty():
			return ToolResult.resolved({success = true})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Remove from group(s) (Godai)")
		for group in to_remove:
			undo_redo.add_do_method(node, "remove_from_group", group)
			undo_redo.add_undo_method(node, "add_to_group", group, true)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true, notes = [UNSAVED_SCENE_NOTE]})


class NodeGetGroups extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var node_paths: Array = p_input.get('node_paths', [])

		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		# Resolve all the nodes up front: if any node path is missing, we reject
		# the whole call.
		var nodes := []
		var errors := PackedStringArray()
		for node_path in node_paths:
			var node = edited_scene_root.get_node_or_null(node_path)
			if not node:
				errors.append("Cannot find node in 'node_paths': %s" % node_path)
				continue
			nodes.append(node)

		if not errors.is_empty():
			return ToolResult.rejected({errors = errors})

		var results := {}
		for i in range(node_paths.size()):
			var node: Node = nodes[i]

			var groups := []
			for group in node.get_groups():
				# Skip internal groups (the engine prefixes those with "_").
				if str(group).begins_with("_"):
					continue
				groups.push_back(str(group))
			results[node_paths[i]] = groups

		return ToolResult.resolved({ nodes = results })


class NodeConnectSignal extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var from_path: String = p_input.get('from_node', '')
		var signal_name: String = p_input.get('signal', '')
		var to_path: String = p_input.get('to_node', '')
		var method: String = p_input.get('method', '')

		var from_node = edited_scene_root.get_node_or_null(from_path)
		if not from_node:
			return ToolResult.rejected({errors = ["Cannot find 'from_node': %s" % from_path]})
		var to_node = edited_scene_root.get_node_or_null(to_path)
		if not to_node:
			return ToolResult.rejected({errors = ["Cannot find 'to_node': %s" % to_path]})

		if not from_node.has_signal(signal_name):
			return ToolResult.rejected({errors = ["%s has no signal named '%s'" % [from_node.get_class(), signal_name]]})
		if not to_node.has_method(method):
			return ToolResult.rejected({errors = ["%s has no method named '%s'" % [to_node.get_class(), method]]})

		var callable := Callable(to_node, method)
		if from_node.is_connected(signal_name, callable):
			return ToolResult.rejected({errors = ["'%s' is already connected to %s.%s" % [signal_name, to_path, method]]})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Connect signal '%s' (Godai)" % signal_name)
		# CONNECT_PERSIST makes the connection part of the saved scene.
		undo_redo.add_do_method(from_node, "connect", signal_name, callable, CONNECT_PERSIST)
		undo_redo.add_undo_method(from_node, "disconnect", signal_name, callable)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true, notes = [UNSAVED_SCENE_NOTE]})


class NodeDisconnectSignal extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var from_path: String = p_input.get('from_node', '')
		var signal_name: String = p_input.get('signal', '')
		var to_path: String = p_input.get('to_node', '')
		var method: String = p_input.get('method', '')

		var from_node = edited_scene_root.get_node_or_null(from_path)
		if not from_node:
			return ToolResult.rejected({errors = ["Cannot find 'from_node': %s" % from_path]})
		var to_node = edited_scene_root.get_node_or_null(to_path)
		if not to_node:
			return ToolResult.rejected({errors = ["Cannot find 'to_node': %s" % to_path]})

		var callable := Callable(to_node, method)
		if not from_node.is_connected(signal_name, callable):
			return ToolResult.rejected({errors = ["'%s' is not connected to %s.%s" % [signal_name, to_path, method]]})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Disconnect signal '%s' (Godai)" % signal_name)
		undo_redo.add_do_method(from_node, "disconnect", signal_name, callable)
		undo_redo.add_undo_method(from_node, "connect", signal_name, callable, CONNECT_PERSIST)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true, notes = [UNSAVED_SCENE_NOTE]})


class NodeAttachScript extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var node_path: String = p_input.get('node_path', '')
		var script_path: String = p_input.get('script_path', '')

		script_path = Utils.to_res_path(script_path)
		if script_path.is_empty():
			return ToolResult.rejected({errors = ["'script_path' must be inside the project (res://)"]})

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({errors = ["Cannot find node at 'node_path': %s" % node_path]})
		if not FileAccess.file_exists(script_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % script_path]})

		var script = load(script_path)
		if not script is Script:
			return ToolResult.rejected({errors = ["'%s' is not a script" % script_path]})

		var script_error := Utils.check_script_for_node(node, script)
		if not script_error.is_empty():
			return ToolResult.rejected({errors = [script_error]})

		var old_script = node.get_script()

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Attach script (Godai)")
		undo_redo.add_do_property(node, "script", script)
		undo_redo.add_undo_property(node, "script", old_script)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true, notes = [UNSAVED_SCENE_NOTE]})


class NodeDetachScript extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var node_path: String = p_input.get('node_path', '')

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.rejected({errors = ["Cannot find node at 'node_path': %s" % node_path]})

		var old_script = node.get_script()
		if not old_script:
			return ToolResult.rejected({errors = ["Node '%s' has no script attached" % node_path]})

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Detach script (Godai)")
		undo_redo.add_do_property(node, "script", null)
		undo_redo.add_undo_property(node, "script", old_script)
		undo_redo.commit_action()

		return ToolResult.resolved({success = true, notes = [UNSAVED_SCENE_NOTE]})
