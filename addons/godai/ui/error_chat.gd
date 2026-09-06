@tool
extends PanelContainer

@onready var label: RichTextLabel = %Label

func setup_error_chat(p_msg: String) -> void:
	label.text = p_msg
