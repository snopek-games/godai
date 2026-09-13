@tool
extends AcceptDialog

const Chat = preload("res://addons/godai/chat/chat.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const JsonView = preload("res://addons/godai/ui/json_view/json_view.gd")
const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")

const THUMBNAIL_SIZE := Vector2(160, 100)

@onready var tool_id_field: Label = %IDField
@onready var tool_name_field: Label = %NameField
@onready var tool_input_field: JsonView = %InputField
@onready var tool_output_field: JsonView = %OutputField
@onready var attachments_field: ScrollContainer = %AttachmentsField
@onready var attachments_container: HBoxContainer = %AttachmentsContainer
@onready var attachment_view_dialog: AcceptDialog = %AttachmentViewDialog

var tool_use_id: String
var _output_schema: Dictionary


func setup_tool_info(p_id: String, p_name: String, p_input, p_output = null, p_is_error := false, p_tool: ToolManager.Tool = null, p_attachments: Array = []) -> void:
	tool_use_id = p_id
	_output_schema = p_tool.output_schema if p_tool else {}

	tool_id_field.text = p_id
	tool_name_field.text = p_name
	tool_input_field.set_value(p_input, p_tool.input_schema if p_tool else {})
	update_output(p_output, p_is_error, p_attachments)


func update_output(p_output, p_is_error := false, p_attachments: Array = []) -> void:
	var severity := JsonFormat.Severity.ERROR if p_is_error else JsonFormat.Severity.NONE
	tool_output_field.set_value(p_output, _output_schema, severity)

	for child in attachments_container.get_children():
		child.free()

	for attachment in p_attachments:
		if not attachment is Chat.ImageContent:
			continue
		var image: Image = attachment.to_image()
		if image == null:
			continue
		var thumbnail := Button.new()
		thumbnail.icon = ImageTexture.create_from_image(image)
		thumbnail.expand_icon = true
		thumbnail.custom_minimum_size = THUMBNAIL_SIZE
		thumbnail.tooltip_text = "%d x %d" % [image.get_width(), image.get_height()]
		thumbnail.pressed.connect(attachment_view_dialog.show_image.bind(image))
		attachments_container.add_child(thumbnail)

	attachments_field.visible = attachments_container.get_child_count() > 0

