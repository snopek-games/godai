extends Node

const CustomLoggerBase = preload("res://addons/godai/custom_logger_base.gd")

class GameLogger extends CustomLoggerBase:
	func _add_lines(p_msg: String) -> void:
		EngineDebugger.send_message("godai:log", [p_msg])

var _logger: GameLogger


func _ready() -> void:
	if EngineDebugger.is_active():
		_logger = GameLogger.new()
		OS.add_logger(_logger)


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_PREDELETE:
			if _logger:
				OS.remove_logger(_logger)
				_logger = null
