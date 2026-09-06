@tool
extends PanelContainer

@onready var label: RichTextLabel = %Label

func setup_user_chat(p_text: String) -> void:
	label.text = p_text
