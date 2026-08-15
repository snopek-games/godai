@abstract
extends Logger

const ERROR_TYPES := {
	Logger.ERROR_TYPE_ERROR: "ERROR",
	Logger.ERROR_TYPE_WARNING: "WARNING",
	Logger.ERROR_TYPE_SCRIPT: "SCRIPT ERROR",
	Logger.ERROR_TYPE_SHADER: "SHADER ERROR",
}

@abstract
func _add_lines(p_msg: String) -> void


func _log_error(p_function: String, p_file: String, p_line: int, p_code: String, p_rationale: String, p_editor_notify: bool, p_error_type: int, p_script_backtraces: Array[ScriptBacktrace]) -> void:
	var error_details: String = p_rationale if p_rationale != "" else p_code
	var msg: String = ERROR_TYPES[p_error_type] + ": " + error_details
	msg += "\n   at: %s (%s:%s)\n" % [p_function, p_file, p_line]
	for bt in p_script_backtraces:
		if not bt.is_empty():
			msg += bt.format(3) + "\n"

	_add_lines(msg)


func _log_message(p_message: String, p_error: bool) -> void:
	var msg := p_message
	if p_error:
		msg = "ERROR: " + msg

	_add_lines(msg)
