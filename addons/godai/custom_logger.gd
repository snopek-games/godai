extends "res://addons/godai/custom_logger_base.gd"

var _enabled := false
var _messages: PackedStringArray
var _mutex := Mutex.new()

## If greater than zero, only the most recent this-many messages are kept
## (the buffer acts as a ring). Zero means keep everything.
var max_messages := 0

## When true, each captured line is prefixed with the local time it arrived,
## as "[HH:MM:SS.mmm] ".
var timestamps := false

# ANSI escape sequences (terminal colors, etc).
var _ansi_regex := RegEx.create_from_string("\\x1b\\[[0-9;]*[A-Za-z]")
# Any remaining control characters, except newline and tab.
var _control_char_regex := RegEx.create_from_string("[\\x00-\\x08\\x0B-\\x1F\\x7F]")

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


func add_lines(p_msg: String) -> void:
	_mutex.lock()
	if _enabled:
		_add_lines(p_msg)
	_mutex.unlock()


func _add_lines(p_msg: String) -> void:
	# Remove special characters so we don't generate invalid JSON.
	var msg := _ansi_regex.sub(p_msg, "", true)
	msg = _control_char_regex.sub(msg, "", true)

	var lines := msg.split("\n")
	if len(lines) > 1 and lines[len(lines) - 1] == "":
		lines = lines.slice(0, len(lines) - 1)

	var stamp := ""
	if timestamps:
		# One clock sample, so the seconds and milliseconds always agree.
		var unix := Time.get_unix_time_from_system()
		var bias: int = Time.get_time_zone_from_system()["bias"]
		stamp = "[%s.%03d] " % [Time.get_time_string_from_unix_time(int(unix) + bias * 60), int(fmod(unix, 1.0) * 1000)]

	for line in lines:
		_messages.push_back(stamp + line)

	# Keep the buffer bounded when a maximum is set.
	if max_messages > 0 and _messages.size() > max_messages:
		_messages = _messages.slice(_messages.size() - max_messages)


func _log_error(p_function: String, p_file: String, p_line: int, p_code: String, p_rationale: String, p_editor_notify: bool, p_error_type: int, p_script_backtraces: Array[ScriptBacktrace]) -> void:
	_mutex.lock()

	if _enabled:
		super._log_error(p_function, p_file, p_line, p_code, p_rationale, p_editor_notify, p_error_type, p_script_backtraces)

	_mutex.unlock()


func _log_message(p_message: String, p_error: bool) -> void:
	_mutex.lock()

	if _enabled:
		super._log_message(p_message, p_error)

	_mutex.unlock()
