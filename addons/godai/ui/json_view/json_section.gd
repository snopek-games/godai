@tool
extends VBoxContainer

const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")

@onready var header: Button = %Header
@onready var body: MarginContainer = %Body

var title := ""
var count_text := ""
var tooltip := ""
var expanded := true
var _body_builder: Callable


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	_apply()


func setup(p_title: String, p_count_text: String, p_tooltip: String, p_expanded: bool, p_body_builder: Callable) -> void:
	title = p_title
	count_text = p_count_text
	tooltip = p_tooltip
	expanded = p_expanded
	_body_builder = p_body_builder
	if is_node_ready():
		_apply()


func get_body_content() -> Control:
	return body.get_child(0) if body.get_child_count() > 0 else null


func _apply() -> void:
	header.tooltip_text = tooltip
	header.set_pressed_no_signal(expanded)
	_update_body()


func _update_body() -> void:
	if expanded and body.get_child_count() == 0 and _body_builder.is_valid():
		var content: Control = _body_builder.call()
		content.size_flags_horizontal = SIZE_EXPAND_FILL
		body.add_child(content)
	body.visible = expanded

	header.icon = JsonFormat.icon(&"GuiTreeArrowDown" if expanded else &"GuiTreeArrowRight", self)
	header.text = title if count_text == "" else "%s  (%s)" % [title, count_text]


func _on_header_toggled(p_pressed: bool) -> void:
	expanded = p_pressed
	_update_body()
