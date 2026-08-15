extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(SceneGetCurrent.new(p_data["get_current_scene"]))
	p_tools.register_tool(SceneGetTree.new(p_data["get_current_scene_tree"]))
	p_tools.register_tool(SceneCreate.new(p_data["create_scene"]))
	p_tools.register_tool(SceneOpen.new(p_data["open_scene"]))
	p_tools.register_tool(SceneGetSelectedNodes.new(p_data["get_selected_nodes"]))
	p_tools.register_tool(SceneInstantiate.new(p_data["instantiate_scene"]))
	p_tools.register_tool(SceneSave.new(p_data["save_scene"]))
	p_tools.register_tool(SceneSaveAs.new(p_data["save_scene_as"]))


class SceneGetCurrent extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()

		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var scene_path: String = edited_scene_root.scene_file_path
		if scene_path.is_empty():
			scene_path = "[unsaved]"

		return ToolResult.resolved({
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

		var script: Script = p_node.get_script()
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
			return ToolResult.rejected({errors = ["No scene open"]})

		return ToolResult.resolved(_get_node_structure(edited_scene_root, edited_scene_root))


class SceneCreate extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var root_node_type: String = p_input.get('root_node_type', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' already exists" % file_path]})

		if not ClassDB.class_exists(root_node_type):
			return ToolResult.rejected({errors = ["Unknown 'root_node_type': %s" % root_node_type]})

		var root_node = ClassDB.instantiate(root_node_type)
		if not root_node:
			return ToolResult.rejected({errors = ["Failed to create '%s'" % root_node_type]})
		if not root_node is Node:
			if not root_node is RefCounted:
				root_node.free()
			return ToolResult.rejected({errors = ["'%s' is not a Node type" % root_node_type]})

		# Like in the editor, we convert the filename to Pascal case.
		root_node.name = file_path.get_file().get_basename().to_pascal_case()

		var err: Error

		var packed_scene := PackedScene.new()
		err = packed_scene.pack(root_node)
		if err != OK:
			root_node.free()
			return ToolResult.rejected({errors = ["Failed to pack scene: %s" % error_string(err)]})

		var dir_path = file_path.get_base_dir()
		if not DirAccess.dir_exists_absolute(dir_path):
			err = DirAccess.make_dir_recursive_absolute(dir_path)
			if err != OK:
				root_node.free()
				return ToolResult.rejected({errors = ["Failed to make parent directory '%s': %s" % [dir_path, error_string(err)]]})

		err = ResourceSaver.save(packed_scene, file_path)
		if err != OK:
			root_node.free()
			return ToolResult.rejected({errors = ["Failed to save scene '%s': %s" % [file_path, error_string(err)]]})

		root_node.free()

		EditorInterface.open_scene_from_path(file_path)

		return ToolResult.resolved({success = true})


class SceneOpen extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		EditorInterface.open_scene_from_path(file_path)

		return ToolResult.resolved({success = true})


class SceneGetSelectedNodes extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var selection := EditorInterface.get_selection()

		var paths := []
		for node in selection.get_selected_nodes():
			# Only nodes that are part of the edited scene have a meaningful
			# path relative to its root.
			if node == edited_scene_root or edited_scene_root.is_ancestor_of(node):
				paths.push_back(str(edited_scene_root.get_path_to(node)))

		return ToolResult.resolved({ node_paths = paths })


class SceneInstantiate extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		var parent_path: String = p_input.get('parent_path', '')
		var scene_path: String = p_input.get('scene_path', '')
		var node_name: String = p_input.get('name', '')

		scene_path = Utils.to_res_path(scene_path)
		if scene_path.is_empty():
			return ToolResult.rejected({errors = ["'scene_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(scene_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % scene_path]})

		var parent = edited_scene_root.get_node_or_null(parent_path)
		if not parent:
			return ToolResult.rejected({errors = ["Cannot find node at 'parent_path': %s" % parent_path]})

		# Don't allow instantiating a scene into itself.
		if scene_path == edited_scene_root.scene_file_path:
			return ToolResult.rejected({errors = ["Cannot instantiate a scene into itself"]})

		var packed = load(scene_path)
		if not packed is PackedScene:
			return ToolResult.rejected({errors = ["'%s' is not a scene" % scene_path]})

		var node = packed.instantiate(PackedScene.GEN_EDIT_STATE_INSTANCE)
		if not node:
			return ToolResult.rejected({errors = ["Failed to instantiate '%s'" % scene_path]})
		if not node_name.is_empty():
			node.name = node_name

		var undo_redo = EditorInterface.get_editor_undo_redo()
		undo_redo.create_action("Instantiate %s (Godai)" % scene_path.get_file())
		Utils.editor_undo_redo_create_node(undo_redo, parent, node)
		undo_redo.commit_action()

		return ToolResult.resolved({
			success = true,
			node_path = str(edited_scene_root.get_path_to(node)),
		})


class SceneSave extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		if edited_scene_root.scene_file_path.is_empty():
			return ToolResult.rejected({errors = ["The current scene has never been saved, so it has no file path. Use save_scene_as instead."]})

		var err := EditorInterface.save_scene()
		if err != OK:
			return ToolResult.rejected({errors = ["Failed to save scene: %s" % error_string(err)]})

		return ToolResult.resolved({
			success = true,
			scene_path = edited_scene_root.scene_file_path,
		})


class SceneSaveAs extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
		if not edited_scene_root:
			return ToolResult.rejected({errors = ["No scene open"]})

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})

		# The scene's own path isn't a collision: the agent just picked
		# save_scene_as where save_scene would do, so save in place.
		if file_path == edited_scene_root.scene_file_path:
			var err := EditorInterface.save_scene()
			if err != OK:
				return ToolResult.rejected({errors = ["Failed to save scene: %s" % error_string(err)]})
			return ToolResult.resolved({
				success = true,
				scene_path = edited_scene_root.scene_file_path,
			})

		if FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' already exists" % file_path]})

		EditorInterface.save_scene_as(file_path)
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' failed to save" % file_path]})

		return ToolResult.resolved({
			success = true,
			scene_path = edited_scene_root.scene_file_path,
		})
