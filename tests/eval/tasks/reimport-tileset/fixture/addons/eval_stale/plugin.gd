@tool
extends EditorPlugin

# The instruction says tileset.png was overwritten outside Godot. A fresh
# fixture has nothing stale, so this overwrites the image (red -> blue) after
# the editor has imported the original; the import cache then really is stale
# until something reimports.


func _enter_tree() -> void:
	_overwrite_after_import.call_deferred()


func _overwrite_after_import() -> void:
	var filesystem := EditorInterface.get_resource_filesystem()
	# Overwriting before the first import finishes would import the new pixels
	# directly, leaving nothing stale.
	for i in 600:
		await get_tree().process_frame
		if filesystem.is_scanning() or not ResourceLoader.exists("res://sprites/tileset.png"):
			continue
		break
	for i in 30:
		await get_tree().process_frame

	var img := Image.create_empty(64, 64, false, Image.FORMAT_RGBA8)
	img.fill(Color.BLUE)
	img.save_png(ProjectSettings.globalize_path("res://sprites/tileset.png"))

	# The marker lets solution.sh wait out the fabrication; .godot keeps it off
	# the workspace diff.
	FileAccess.open("res://.godot/eval_stale_done", FileAccess.WRITE).store_string("done")
