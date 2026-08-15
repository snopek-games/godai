extends RefCounted

const CustomLogger = preload("res://addons/godai/custom_logger.gd")

static var logger: CustomLogger


static func setup() -> void:
	logger = CustomLogger.new()
	logger.max_messages = 1000
	OS.add_logger(logger)
	logger.start()


static func shutdown() -> void:
	OS.remove_logger(logger)
	logger = null
