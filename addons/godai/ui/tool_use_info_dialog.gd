@tool
extends AcceptDialog

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const JsonView = preload("res://addons/godai/ui/json_view/json_view.gd")
const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")

@onready var tool_id_field: Label = %IDField
@onready var tool_name_field: Label = %NameField
@onready var tool_input_field: JsonView = %InputField
@onready var tool_output_field: JsonView = %OutputField

var tool_use_id: String
var _output_schema: Dictionary


func setup_tool_info(p_id: String, p_name: String, p_input, p_output = null, p_is_error := false, p_tool: ToolManager.Tool = null) -> void:
	tool_use_id = p_id
	_output_schema = p_tool.output_schema if p_tool else {}

	tool_id_field.text = p_id
	tool_name_field.text = p_name
	tool_input_field.set_value(p_input, p_tool.input_schema if p_tool else {})
	update_output(p_output, p_is_error)


func update_output(p_output, p_is_error := false) -> void:
	var severity := JsonFormat.Severity.ERROR if p_is_error else JsonFormat.Severity.NONE
	tool_output_field.set_value(p_output, _output_schema, severity)

