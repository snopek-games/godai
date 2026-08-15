@tool
extends EditorPlugin

# A fresh headless editor has no scene open and nothing selected, so this
# fabricates the editor state the instruction refers to.


func _enter_tree() -> void:
	_select.call_deferred()


func _select() -> void:
	await get_tree().process_frame
	EditorInterface.open_scene_from_path("res://main.tscn")
	for i in 100:
		await get_tree().process_frame
		var scene_root := EditorInterface.get_edited_scene_root()
		if scene_root != null and scene_root.name == "Main":
			var selection := EditorInterface.get_selection()
			selection.clear()
			for node_name in ["Crate1", "Crate2"]:
				var node := scene_root.get_node_or_null(NodePath(node_name))
				if node != null:
					selection.add_node(node)
			return
