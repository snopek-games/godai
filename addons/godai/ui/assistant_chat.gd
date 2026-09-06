@tool
extends MarginContainer

@onready var markdown_label = %MarkdownLabel


func setup_assistant_chat(p_text: String) -> void:
	markdown_label.set_markdown(p_text)
