@tool
extends PanelContainer

@onready var title_label: Label = %TitleLabel
@onready var info_button: Button = %InfoButton

var _tool_use_id: String
var _tool_name: String
var _tool_input: Dictionary
var _tool_output
var _tool_output_is_error := false

signal info_requested(id: String, name: String, input, output, is_error: bool)


func setup_tool_chat(p_id: String, p_name: String, p_title: String, p_input: Dictionary) -> void:
	_tool_use_id = p_id
	_tool_name = p_name
	_tool_input = p_input

	title_label.text = p_title


func set_tool_output(p_output, p_is_error := false) -> void:
	_tool_output = p_output
	_tool_output_is_error = p_is_error


func _on_info_button_pressed() -> void:
	info_requested.emit(_tool_use_id, _tool_name, _tool_input, _tool_output, _tool_output_is_error)
