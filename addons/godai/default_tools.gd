extends RefCounted

const ToolManager = preload("res://addons/godai/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const Utils = preload("res://addons/godai/utils.gd")

static func register(p_tools: ToolManager) -> void:
	p_tools.register_tool(SceneGetCurrent.new())
	p_tools.register_tool(SceneGetTree.new())
	p_tools.register_tool(NodeGetProperties.new())
	p_tools.register_tool(NodeSetProperties.new())
	p_tools.register_tool(NodeCreate.new())
	p_tools.register_tool(NodeRemove.new())
	p_tools.register_tool(ClassDBGetClasses.new())
	p_tools.register_tool(EditorScriptExecute.new())


class SceneGetCurrent extends ToolManager.Tool:
	func _init() -> void:
		name = "get_current_scene"

		description = "Gets information about the scene that is currently opened in the Godot editor.\n\n" +\
			"Returns:\n" +\
			" - scene_path: The path to the scene file\n" +\
			" - root_node_type: The class of the root Node of the scene\n" +\
			" - root_node_name: The name of the root Node of the scene\n" +\
			"If no scene is currently open, it'll return an empty string for all of the above.\n"

		input_schema = ToolManager.INPUT_SCHEMA_EMPTY
		# @todo Define the output_schema

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


class SceneGetTree extends ToolManager.Tool:
	func _init() -> void:
		name = "get_current_scene_tree"

		description = "Gets the tree for the scene that is currently opened in the Godot editor.\n\n" + \
			"Returns a tree of objects representing the nodes in the scene with the following keys:\n" +\
			" - name: The Node name\n" +\
			" - type: The class of the Node\n" +\
			" - path: The path of the node within the scene, relative to the scene root\n" +\
			" - script: The path to the script file, if this node has a script attached; otherwise, it'll be omitted\n" +\
			" - children: An array of objects with the same structure, representing the child nodes; will be omitted if node has no children\n" +\
			"If no scene is currently open, it'll return an empty object.\n"

		input_schema = ToolManager.INPUT_SCHEMA_EMPTY
		# @todo Define the output_schema

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


class NodeGetProperties extends ToolManager.Tool:
	func _init() -> void:
		name = "get_node_properties"
		description = "Gets the values of properties on specific nodes in the current scene\n\n" +\
			"Returns an object with properties for each node specified by node path, containing another object with the property values of that object.\n" +\
			"The property values are converted to a JSON string using Godot's `var_to_str()` function.\n"

		input_schema = {
			type = "object",
			properties = {
				node_paths = {
					type = "array",
					items = {
						type = "string",
						description = "The path to the node within the scene, relative to the scene root"
					}
				}
			}
		}
		# @todo Define the output_schema

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

					print("PROP: ", prop)
					props[prop_name] = var_to_str(node.get(prop_name))

			results[node_path] = props

		return ToolResult.resolved_json(results)


class NodeSetProperties extends ToolManager.Tool:
	func _init() -> void:
		name = "set_node_properties"
		description = "Sets the values of properties on specific nodes in the current scene\n\n" +\
			"Only pass in the properties you actually wish to change.\n" +\
			"EXAMPLE: {\"action\": , \"Update property on node\": [{\"node_path\": \"path/to/node\": \"properties\": {\"property\": \"value\"}}]}\n" +\
			"This uses the Godot editor's undo/redo system to set the properties, so as many set operations as possible should be done in a single call, so they can all be undone at once.\n" +\
			"Returns an object with properties for each node path, containing true if we were able to find the node; otherwise, false or missing.\n" +\
			"Note: Just because this returns successfully, doesn't mean all properties were able to be set to the requested value. Always check that the properties have the correct value afterwards.\n"

		input_schema = {
			type = "object",
			properties = {
				action = {
					type = "string",
					description = "Human-readable description of the action that will be shown in Godot's undo/redo history",
				},
				nodes = {
					type = "array",
					items = {
						type = "object",
						properties = {
							node_path = {
								type = "string",
								description = "The path to the node within the scene, relative to the scene root"
							},
							properties = {
								type = "object",
								description = "Keys are the Godot property names to set",
								additionalProperties = {
									type = "string",
									description = "The property value as a string - it will be converted back to the Godot type using Godot's `str_to_var()`. This will only work for simple types and not resources."
								},
							},
						},
					},
				},
			},
		}
		# @todo Define the output_schema

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


class NodeCreate extends ToolManager.Tool:
	func _init() -> void:
		name = "create_node"
		description = "Creates a new node in the current scene, with the given properties set.\n\n" +\
			#"Only pass in properties to set that differ from their default values"
			"Returns true if successful; otherwise false\n" +\
			"Note: Just because this returns successfully, doesn't mean all properties were able to be set to the requested value. Always check that the properties have the correct value afterwards.\n"

		input_schema = {
			type = "object",
			properties = {
				parent_path = {
					type = "string",
					description = "Path to the parent node, relative the scene root",
				},
				node_type = {
					type = "string",
					description = "The name of the node class to create",
				},
				properties = {
					type = "object",
					description = "Keys are the Godot property names to set",
					additionalProperties = {
						type = "string",
						description = "The property value as a string - it will be converted back to the Godot type using Godot's `str_to_var()`. This will only work for simple types and not resources."
					},
				},
			}
		}
		# @todo Define the output_schema

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


class NodeRemove extends ToolManager.Tool:
	func _init() -> void:
		name = "remove_node"
		description = "Removes the given node (and all its children) from the current scene"

		input_schema = {
			type = "object",
			properties = {
				node_path = {
					type = "string",
					description = "Path to the node, relative to the scene root",
				}
			}
		}
		# @todo Define the output_schema

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


class ClassDBGetClasses extends ToolManager.Tool:
	func _init() -> void:
		name = "classdb_get_classes"
		description = "Gets all the classes registered in ClassDB with a little bit of information about them, including: parent class, API type, and whether they are enabled or can be instantiated."

		input_schema = ToolManager.INPUT_SCHEMA_EMPTY
		# @todo Define the output_schema

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


#class ClassDBGetPropertyList extends ToolManager.Tool:
#	func _init() -> void:
#		name = "classdb_get_property_list"
#		description = "Gets the list of properties for the given class, along with information about each property's type, hint, usage and default value"
#
#		input_schema = {
#		}
#		# @todo Define the output_schema
#
#	func execute(p_input) -> ToolResult:
#		pass


class EditorScriptExecute extends ToolManager.Tool:
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

	func _init() -> void:
		name = "execute_editor_script"
		description = "Executes the given GDScript code in the editor, in the context of a Node that is a child of the scene currently being edited.\n\n" +\
			"If you modify the current scene, you MUST use `EditorUndoRedoManager` from `EditorInterface.get_editor_undo_redo()`, and the action name MUST end with \"(AI)\"." +\
			"You can find nodes relative to the scene root using `EditorInterface.get_edited_scene_root().get_node_or_null(node_path)`." +\
			"Two helper methods have been provided:\n"+\
			" - `func editor_undo_redo_live_create_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void`" +\
			" - `func editor_undo_redo_live_remove_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void`" +\
			"If you are using `EditorUndoRedoManager` to add or remove a node, you MUST call one of those helper methods before calling `commit_action()`. This will add some `add_do_method()` and `add_undo_method()` calls to ensure the changes are synchronized to the live game if the game is running." +\
			"If the script successfully runs, the output from `print()` will be returned."

		input_schema = {
			type = "object",
			properties = {
				code = {
					type = "string",
					description = "A snippet of GDScript code"
				},
			},
		}
		# @todo Define the output_schema


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


# @todo List class properties (from ClassDB)
# @todo List node properties (from a specific node)
# @todo Get class documentation
