@tool
extends PanelContainer

@onready var label: Label = %Label

func setup_error_chat(p_type: String, p_msg: String) -> void:
	label.text = "Error (%s): %s" % [p_type, p_msg]
