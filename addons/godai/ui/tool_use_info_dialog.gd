@tool
extends AcceptDialog

@onready var tool_id_field: Label = %IDField
@onready var tool_name_field: Label = %NameField
@onready var tool_input_field: RichTextLabel = %InputField
@onready var tool_output_field: RichTextLabel = %OutputField

var tool_use_id: String

func setup_tool_info(p_id: String, p_name: String, p_input, p_output = null) -> void:
	tool_use_id = p_id

	tool_id_field.text = p_id
	tool_name_field.text = p_name
	tool_input_field.text = _to_json(p_input)

	if p_output != null:
		update_output(p_output)
	else:
		tool_output_field.text = ""


func update_output(p_output) -> void:
	var data
	if p_output is String:
		if p_output != "":
			data = JSON.parse_string(p_output)
	else:
		data = p_output

	tool_output_field.text = _to_json(data)


func _to_json(p_data) -> String:
	return JSON.stringify(p_data, "    ")

