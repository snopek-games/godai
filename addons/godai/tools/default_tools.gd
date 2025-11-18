extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const Utils = preload("res://addons/godai/utils.gd")

const DEFAULT_TOOLS_JSON = "res://addons/godai/tools/default_tools.json"

static func register(p_tools: ToolManager) -> void:
	var data := _load_json_data()

	p_tools.register_tool(SceneGetCurrent.new(data["get_current_scene"]))
	p_tools.register_tool(SceneGetTree.new(data["get_current_scene_tree"]))
	p_tools.register_tool(NodeGetProperties.new(data["get_node_properties"]))
	p_tools.register_tool(NodeSetProperties.new(data["set_node_properties"]))
	p_tools.register_tool(NodeCreate.new(data["node_create"]))
	p_tools.register_tool(NodeRemove.new(data["node_remove"]))
	p_tools.register_tool(ClassDBGetClasses.new(data["classdb_get_classes"]))
	p_tools.register_tool(EditorScriptExecute.new(data["execute_editor_script"]))


static func _load_json_data() -> Dictionary:
	var fa := FileAccess.open(DEFAULT_TOOLS_JSON, FileAccess.READ)
	if fa:
		var data: Dictionary = JSON.parse_string(fa.get_as_text())
		var tools: Dictionary = data["tools"]
		# Store the name in the value so we don't have to repeat it.
		for name in tools.keys():
			tools[name]["name"] = name
		return tools

	return Dictionary()


@abstract
class DefaultTool extends ToolManager.Tool:
	func _init(p_data: Dictionary) -> void:
		name = p_data['name']
		title = p_data.get('title', name)

		var raw_desc = p_data['description']
		if raw_desc is Array:
			description = "\n".join(raw_desc)
		else:
			description = raw_desc

		input_schema = p_data.get("input_schema", ToolManager.INPUT_SCHEMA_EMPTY)


class SceneGetCurrent extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.resolved_json({
				scene_path = "",
				root_node_type = "",
				root_node_name = "",
			})

		var scene_path: String = edited_scene_root.scene_file_path
		if scene_path.is_empty():
			scene_path = "[unsaved]"

		return ToolResult.resolved_json({
			scene_path = scene_path,
			root_node_type = edited_scene_root.get_class(),
			root_node_name = edited_scene_root.name,
		})


class SceneGetTree extends DefaultTool:
	func _get_node_structure(p_node: Node, p_root: Node) -> Dictionary:
		var data := {
			name = p_node.name,
			type = p_node.get_class(),
			path = str(p_root.get_path_to(p_node)),
		}

		var script := p_node.get_script()
		if script:
			data["script"] = script.resource_path

		var children := p_node.get_children()
		if children.size() > 0:
			var children_data := []
			for child in children:
				if child.owner != p_root:
					continue
				children_data.push_back(_get_node_structure(child, p_root))
			data["children"] = children_data

		return data

	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.resolved_json({})

		return ToolResult.resolved_json(_get_node_structure(edited_scene_root, edited_scene_root))


class NodeGetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.resolved_json({})

		var node_paths: Array = p_input["node_paths"]
		var results := {}

		for node_path in node_paths:
			var props := {}

			var node := edited_scene_root.get_node_or_null(node_path)
			if node:
				for prop in node.get_property_list():
					var prop_name: String = prop['name']
					var prop_usage: int = prop['usage']

					if prop_name.begins_with("_") or prop_usage & PROPERTY_USAGE_INTERNAL:
						continue
					if prop_usage & PROPERTY_USAGE_GROUP or prop_usage & PROPERTY_USAGE_CATEGORY or prop_usage & PROPERTY_USAGE_SUBGROUP:
						continue

					props[prop_name] = var_to_str(node.get(prop_name))

			results[node_path] = props

		return ToolResult.resolved_json(results)


class NodeSetProperties extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.resolved({})

		var action: String = p_input['action']
		var edits: Array = p_input['nodes']

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("%s (AI)" % action)

		var results := {}

		for edit in edits:
			var node_path = edit['node_path']
			var props = edit['properties']

			var node := edited_scene_root.get_node_or_null(node_path)
			if node:
				results[node_path] = true
				for prop_name in props:
					undo_redo.add_do_property(node, prop_name, str_to_var(props[prop_name]))
					undo_redo.add_undo_property(node, prop_name, node.get(prop_name))
			else:
				results[node_path] = false

		undo_redo.commit_action()

		return ToolResult.resolved_json(results)


class NodeCreate extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.resolved_json(false)

		var parent_path: String = p_input['parent_path']
		var node_type: String = p_input['node_type']
		var props: Dictionary = p_input['properties']

		var parent = edited_scene_root.get_node_or_null(parent_path)
		if not parent:
			return ToolResult.resolved_json({ success = false })

		var node = ClassDB.instantiate(node_type)
		if not node:
			return ToolResult.resolved_json({ success = false })

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Create %s node (AI)" % node_type)
		undo_redo.add_do_method(parent, "add_child", node, true)
		undo_redo.add_do_method(node, "set_owner", edited_scene_root)
		undo_redo.add_do_method(EditorInterface.get_selection(), "add_node", node)
		undo_redo.add_do_reference(node)
		undo_redo.add_undo_method(parent, "remove_child", node)

		Utils.editor_undo_redo_live_create_node(undo_redo, parent, node)

		for prop_name in props:
			undo_redo.add_do_property(node, prop_name, str_to_var(props[prop_name]))
		undo_redo.commit_action()

		return ToolResult.resolved_json({
			success = true,
			node_path = edited_scene_root.get_path_to(node),
		})


class NodeRemove extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.resolved_json(false)

		var node_path: String = p_input['node_path']

		var node = edited_scene_root.get_node_or_null(node_path)
		if not node:
			return ToolResult.resolved_json({ success = false })

		if node == edited_scene_root:
			return ToolResult.resolved_json({ success = false })

		var parent = node.get_parent()

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Delete %s node (AI)" % node.name)
		undo_redo.add_do_method(parent, "remove_child", node)
		undo_redo.add_undo_method(parent, "add_child", node, true)
		undo_redo.add_undo_method(parent, "move_child", node, node.get_index(false))
		undo_redo.add_undo_method(node, "set_owner", edited_scene_root)
		undo_redo.add_undo_reference(node)

		Utils.editor_undo_redo_live_remove_node(undo_redo, parent, node)

		undo_redo.commit_action()

		return ToolResult.resolved_json({
			success = true,
		})


class ClassDBGetClasses extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var class_list := ClassDB.get_class_list()

		var result := []
		for name in class_list:
			var class_data := {
				name = name,
				parent = ClassDB.get_parent_class(name),
				enabled = ClassDB.is_class_enabled(name),
				can_instantiate = ClassDB.can_instantiate(name),
			}

			var api_type: String
			match ClassDB.class_get_api_type(name):
				ClassDB.API_CORE:
					api_type = "core"
				ClassDB.API_EDITOR:
					api_type = "editor"
				ClassDB.API_EXTENSION:
					api_type = "extension"
				ClassDB.API_EDITOR_EXTENSION:
					api_type = "editor_extension"
				_:
					api_type = "unknown"
			class_data["api_type"] = api_type

			result.push_back(class_data)

		return ToolResult.resolved_json(result)


class EditorScriptExecute extends DefaultTool:
	const SCRIPT_TEMPLATE = """@tool
extends Node

const __Utils = preload("res://addons/godai/utils.gd")

var __output := PackedStringArray()

signal __run_completed(success: bool, output: PackedStringArray)

func __custom_print(...values: Array) -> void:
	var content := ""
	for value in values:
		content += str(value)
	print(content)
	__output.push_back(content)

func __run():
	# Await just in case the user code does.
	var err = await __user_code()
	__run_completed.emit(err == OK, __output)

func editor_undo_redo_live_create_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	__Utils.editor_undo_redo_live_create_node(p_undo_redo, p_parent, p_child)

func editor_undo_redo_live_remove_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	__Utils.editor_undo_redo_live_remove_node(p_undo_redo, p_parent, p_child)

func __user_code() -> Error:
	# USER CODE START
{user_code}
	# USER CODE END
	return OK
"""

	func execute(p_input) -> ToolResult:
		var code = p_input['code']
		if code.is_empty():
			return ToolResult.resolved_json({error = "No code"})

		var main_loop = Engine.get_main_loop()
		if not main_loop is SceneTree:
			return ToolResult.resolved_json({error = "No scene tree"})

		var root_node: Node = main_loop.get_root()
		if not root_node:
			return ToolResult.resolved_json({error = "No root node"})

		var full_source = SCRIPT_TEMPLATE.replace('{user_code}', _process_user_code(code))

		var script = GDScript.new()
		script.source_code = full_source

		var script_error = script.reload()
		if script_error != OK:
			return ToolResult.resolved_json({error = "Script failed to parse"})

		var script_node = Node.new()
		script_node.name = "EditorScriptNode"
		script_node.script = script
		root_node.add_child(script_node)

		if not script_node.has_method("__run") or not script_node.has_signal("__run_completed"):
			script_node.queue_free()
			return ToolResult.resolved_json({error = "Script parsed but is malformed"})

		var result := ToolResult.new()

		script_node.connect("__run_completed", _on_run_completed.bind(script_node, result))
		script_node.call("__run")

		# @todo: we probably want some kind of timeout?

		return result

	func _process_user_code(p_code: String) -> String:
		var output := PackedStringArray()

		var first_space_count := 0

		for line in p_code.split("\n"):
			var processed: String = line

			processed = processed.replace("print(", "__custom_print(")

			# All lines need at least one tab.
			var tabs := "\t"

			# Count the leading spaces (if any).
			var space_count := 0
			for i in range(line.length()):
				if line[i] != " ":
					break
				space_count += 1

			# If this is the first line with spaces, capture that number.
			if first_space_count == 0:
				first_space_count = space_count

			# Replace the spaces with tabs.
			if space_count > 0:
				# Assume that the first space count is the tab width.
				for i in range(space_count / first_space_count):
					tabs += "\t"

			output.push_back("\t" + processed)

		return "\n".join(output)

	func _on_run_completed(p_success: bool, p_output: PackedStringArray, p_script_node: Node, p_result: ToolResult) -> void:
		if p_success:
			p_result.resolve_json({success = true, output = p_output})
		else:
			p_result.resolve_json({error = "Failed to execute script"})

		p_script_node.queue_free()
