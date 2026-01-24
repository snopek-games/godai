extends Logger

var _enabled := false
var _messages: PackedStringArray
var _mutex := Mutex.new()

const ERROR_TYPES := {
	Logger.ERROR_TYPE_ERROR: "ERROR",
	Logger.ERROR_TYPE_WARNING: "WARNING",
	Logger.ERROR_TYPE_SCRIPT: "SCRIPT ERROR",
	Logger.ERROR_TYPE_SHADER: "SHADER ERROR",
}

func start() -> void:
	_mutex.lock()
	_messages.clear()
	_enabled = true
	_mutex.unlock()


func stop() -> PackedStringArray:
	_mutex.lock()
	var ret := _messages
	_messages = PackedStringArray()
	_enabled = false
	_mutex.unlock()
	return ret


func clear() -> void:
	_mutex.lock()
	_messages.clear()
	_mutex.unlock()


func get_messages() -> PackedStringArray:
	_mutex.lock()
	var ret := _messages.duplicate()
	_mutex.unlock()
	return ret


func _add_lines(p_msg: String) -> void:
	var lines := p_msg.split("\n")
	if len(lines) > 1 and lines[len(lines) - 1] == "":
		lines = lines.slice(0, len(lines) - 1)

	for line in lines:
		_messages.push_back(line)


func _log_error(p_function: String, p_file: String, p_line: int, p_code: String, p_rationale: String, p_editor_notify: bool, p_error_type: int, p_script_backtraces: Array[ScriptBacktrace]) -> void:
	_mutex.lock()

	if _enabled:
		var error_details: String = p_rationale if p_rationale != "" else p_code
		var msg: String = ERROR_TYPES[p_error_type] + ": " + error_details
		msg += "\n   at: %s (%s:%s)\n" % [p_function, p_file, p_line]
		for bt in p_script_backtraces:
			if not bt.is_empty():
				msg += bt.format(3) + "\n"

		_add_lines(msg)

	_mutex.unlock()


func _log_message(p_message: String, p_error: bool) -> void:
	_mutex.lock()

	if _enabled:
		var msg := p_message
		if p_error:
			msg = "ERROR: " + msg

		_add_lines(msg)

	_mutex.unlock()
