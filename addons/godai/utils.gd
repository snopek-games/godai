extends RefCounted


static func get_scene_tree() -> SceneTree:
	var main_loop: MainLoop = Engine.get_main_loop()
	if main_loop is SceneTree:
		return main_loop
	return null


static func get_scene_root() -> Node:
	var scene_tree: SceneTree = get_scene_tree()
	if not scene_tree:
		return null

	return scene_tree.root


static func get_editor_debugger_node() -> Node:
	var root: Node = get_scene_root()
	if not root:
		return null

	var results = root.find_children("*", "EditorDebuggerNode", true, false)
	if len(results) == 0:
		return null

	return results[0]


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


static func editor_undo_redo_live_create_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	var editor_debugger_node := get_editor_debugger_node()
	if not editor_debugger_node:
		return

	var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
	if not edited_scene_root:
		return

	set_child_node_name(p_parent, p_child)

	p_undo_redo.add_do_method(editor_debugger_node, "live_debug_create_node", edited_scene_root.get_path_to(p_parent), p_child.get_class(), p_child.name)
	p_undo_redo.add_undo_method(editor_debugger_node, "live_debug_remove_node", NodePath(str(edited_scene_root.get_path_to(p_parent)) + "/" + p_child.name))


static func editor_undo_redo_live_remove_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	var editor_debugger_node := get_editor_debugger_node()
	if not editor_debugger_node:
		return

	var edited_scene_root: Node = EditorInterface.get_edited_scene_root()
	if not edited_scene_root:
		return

	p_undo_redo.add_do_method(editor_debugger_node, "live_debug_remove_and_keep_node", edited_scene_root.get_path_to(p_child), p_child.get_instance_id());
	p_undo_redo.add_undo_method(editor_debugger_node, "live_debug_restore_node", p_child.get_instance_id(), edited_scene_root.get_path_to(p_parent), p_child.get_index(false))
