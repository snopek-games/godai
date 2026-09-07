@tool
extends Control

const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")
const JsonViewBuilder = preload("res://addons/godai/ui/json_view/json_view_builder.gd")

const TOGGLE_BUTTON_IDLE_ALPHA := 0.4

@onready var panel: PanelContainer = %Panel
@onready var content_container: MarginContainer = %Content
@onready var raw_text: TextEdit = %RawText
@onready var toggle_button: Button = %ToggleButton

@export var expand_all := false
@export var empty_text := "(none)"
@export var null_text := "(none)"

var value = null
var schema: Dictionary = {}
var severity := JsonFormat.Severity.NONE
var _toggle_button_hovered := false


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	_update_toggle_button()
	_update_panel_theme()

	var font := JsonFormat.source_font(self)
	if font:
		raw_text.add_theme_font_override("font", font)

	_rebuild()


func _notification(p_what: int) -> void:
	if p_what == NOTIFICATION_THEME_CHANGED and is_node_ready():
		_update_panel_theme()


func _update_panel_theme() -> void:
	var style: StyleBox = get_theme_stylebox("normal", "RichTextLabel")
	raw_text.add_theme_stylebox_override("normal", style)
	raw_text.add_theme_stylebox_override("read_only", style)

	var panel_style: StyleBox = style.duplicate()
	content_container.add_theme_constant_override("margin_right", int(style.get_margin(SIDE_RIGHT)))
	panel_style.content_margin_right = 0
	panel.add_theme_stylebox_override("panel", panel_style)


func set_value(p_value, p_schema: Dictionary = {}, p_severity := JsonFormat.Severity.NONE) -> void:
	value = _parse(p_value)
	schema = p_schema
	severity = p_severity
	if is_node_ready():
		_rebuild()


func get_content() -> Control:
	return content_container.get_child(0) if content_container.get_child_count() > 0 else null


func _parse(p_value):
	if p_value is String and p_value != "":
		var json := JSON.new()
		if json.parse(p_value) == OK and json.data != null:
			return json.data
	return p_value


func _rebuild() -> void:
	for child in content_container.get_children():
		content_container.remove_child(child)
		child.free()

	var builder := JsonViewBuilder.new()
	builder.expand_all = expand_all
	builder.empty_text = empty_text
	builder.null_text = null_text
	builder.root_severity = severity
	var content := builder.build(value, schema)
	content.size_flags_horizontal = SIZE_EXPAND_FILL
	content_container.add_child(content)

	raw_text.text = _raw_text()
	_show_current_view()


func _raw_text() -> String:
	if value == null:
		return ""
	if value is String:
		return value
	return JSON.stringify(value, "    ")


func _show_current_view() -> void:
	var raw := toggle_button.button_pressed
	panel.visible = not raw
	raw_text.visible = raw


func _update_toggle_button() -> void:
	var raw := toggle_button.button_pressed
	toggle_button.tooltip_text = "Show formatted view" if raw else "Show raw JSON"
	toggle_button.modulate.a = 1.0 if raw or _toggle_button_hovered else TOGGLE_BUTTON_IDLE_ALPHA


func _on_toggle_button_toggled(_pressed: bool) -> void:
	_show_current_view()
	_update_toggle_button()


func _on_toggle_button_mouse_entered() -> void:
	_toggle_button_hovered = true
	_update_toggle_button()


func _on_toggle_button_mouse_exited() -> void:
	_toggle_button_hovered = false
	_update_toggle_button()
