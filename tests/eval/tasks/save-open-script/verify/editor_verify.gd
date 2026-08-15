# Godot flushes open script buffers to disk when the editor exits, even on a
# skip-save close, so whether the agent saved is only observable here, while
# the editor is still running.

var checks: Array = []

EditorInterface.edit_script(load("res://player.gd"))
var buffer_text: String = ""
var editor := EditorInterface.get_script_editor().get_current_editor()
if editor != null and editor.get_base_editor() != null:
	buffer_text = editor.get_base_editor().text
var buffer_edited: bool = "var speed := 250.0" in buffer_text
checks.append({
	"name": "buffer_has_edit",
	"ok": buffer_edited,
	"detail": "" if buffer_edited else "the fixture's unsaved edit (speed 250) never reached the script editor buffer",
})

var on_disk: String = FileAccess.get_file_as_string("res://player.gd")
var saved: bool = "var speed := 250.0" in on_disk
checks.append({
	"name": "saved_to_disk",
	"ok": saved,
	"detail": "" if saved else "player.gd on disk still has the old speed; the buffer was not saved",
})

var passed_count: int = 0
for c in checks:
	if c["ok"]:
		passed_count += 1

print("GODAI_VERIFY_JSON:" + JSON.stringify({
	"passed": passed_count == checks.size(),
	"checks_passed": passed_count,
	"checks_total": checks.size(),
	"checks": checks,
}))
