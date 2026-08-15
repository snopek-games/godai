extends EditorDebuggerPlugin

const EditorGlobals = preload("res://addons/godai/editor_globals.gd")


func _has_capture(p_capture: String) -> bool:
	return p_capture == "godai"


func _capture(p_message: String, p_data: Array, p_session_id: int) -> bool:
	# @todo Identify the messages as coming from a particular game instance (by session id?)

	match p_message:
		"godai:log":
			EditorGlobals.logger.add_lines(p_data[0])
			return true

	return false
