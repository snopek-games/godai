@tool
extends EditorPlugin

# A fresh editor has no unsaved buffers, so this opens player.gd and edits it
# in the script editor without saving (speed 100 -> 250), which is the state
# the instruction pretends the user left behind.


func _enter_tree() -> void:
	# Not named _edit: that's an EditorPlugin virtual with a different signature.
	_fabricate_unsaved_edit.call_deferred()


func _fabricate_unsaved_edit() -> void:
	var script: Script = load("res://player.gd")
	# edit_script is silently dropped while the editor is still starting up, so
	# keep asking until the script editor actually has the buffer open.
	for i in 600:
		await get_tree().process_frame
		EditorInterface.edit_script(script)
		var editor := EditorInterface.get_script_editor().get_current_editor()
		if editor == null:
			continue
		var code_edit: TextEdit = editor.get_base_editor()
		if code_edit == null or not ("var speed := 100.0" in code_edit.text):
			continue
		code_edit.text = code_edit.text.replace("var speed := 100.0", "var speed := 250.0")
		return
