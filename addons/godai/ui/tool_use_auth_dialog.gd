@tool
extends Window

const JsonView = preload("res://addons/godai/ui/json_view/json_view.gd")

@onready var panel_container: PanelContainer = %PanelContainer
@onready var name_field: Label = %NameField
@onready var input_field: JsonView = %InputField

@onready var allow_button_container: HBoxContainer = %AllowButtonContainer
@onready var allow_button: Button = %AllowButton
@onready var allow_menu: PopupMenu = %AllowMenu
@onready var allow_menu_button: Button = %AllowMenuButton

@onready var deny_button_container: HBoxContainer = %DenyButtonContainer
@onready var deny_button: Button = %DenyButton
@onready var deny_menu: PopupMenu = %DenyMenu
@onready var deny_menu_button: Button = %DenyMenuButton

enum AllowDenyType {
	ONCE,
	TOOL_FOR_SESSION,
	TOOL_ALWAYS,
	ALL_FOR_SESSION,
}

signal tool_use_allowed(p_type: AllowDenyType)
signal tool_use_denied(p_type: AllowDenyType)


func _ready() -> void:
	_update_panel_theme()
	if Engine.is_editor_hint() and not is_part_of_edited_scene():
		allow_menu_button.icon = EditorInterface.get_editor_theme().get_icon(&"GuiOptionArrow", &"EditorIcons")
		deny_menu_button.icon = EditorInterface.get_editor_theme().get_icon(&"GuiOptionArrow", &"EditorIcons")


func _notification(p_what: int) -> void:
	if p_what == NOTIFICATION_THEME_CHANGED and is_node_ready():
		_update_panel_theme()


func _update_panel_theme() -> void:
	panel_container.add_theme_stylebox_override("panel", get_theme_stylebox("panel", "AcceptDialog"))


func setup_tool_use_auth_dialog(p_tool_name: String, p_input, p_input_schema: Dictionary = {}) -> void:
	name_field.text = p_tool_name
	input_field.set_value(p_input, p_input_schema)

	allow_menu.set_item_text(0, 'Allow "%s" for this session' % p_tool_name)
	allow_menu.set_item_text(1, 'Allow "%s" always' % p_tool_name)
	deny_menu.set_item_text(0, 'Deny "%s" for this session' % p_tool_name)
	deny_menu.set_item_text(1, 'Deny "%s" always' % p_tool_name)


func _popup_menu(p_container: Control, p_popup_menu: PopupMenu) -> void:
	var screen_xform: Transform2D = p_container.get_screen_transform()
	var screen_rect := Rect2(screen_xform.origin, screen_xform.get_scale() * p_container.size)

	var pos := screen_rect.position
	pos.y += screen_rect.size.y

	p_popup_menu.position = pos
	p_popup_menu.popup()


func _toggle_menu(p_container: Control, p_popup_menu: PopupMenu) -> void:
	if p_popup_menu.visible:
		p_popup_menu.hide()
		return

	_popup_menu(p_container, p_popup_menu)


func _on_allow_button_pressed() -> void:
	tool_use_allowed.emit(AllowDenyType.ONCE)


func _on_allow_menu_id_pressed(p_id: int) -> void:
	# Ids match the values of the enum.
	tool_use_allowed.emit(p_id)


func _on_allow_menu_button_pressed() -> void:
	_toggle_menu(allow_button_container, allow_menu)


func _on_allow_menu_about_to_popup() -> void:
	allow_menu_button.set_pressed_no_signal(true)


func _on_allow_menu_popup_hide() -> void:
	allow_menu_button.set_pressed_no_signal(false)


func _on_deny_button_pressed() -> void:
	tool_use_denied.emit(AllowDenyType.ONCE)


func _on_deny_menu_id_pressed(p_id: int) -> void:
	# Ids match the values of the enum.
	tool_use_denied.emit(p_id)


func _on_deny_menu_button_pressed() -> void:
	_toggle_menu(deny_button_container, deny_menu)


func _on_deny_menu_about_to_popup() -> void:
	deny_menu_button.set_pressed_no_signal(true)


func _on_deny_menu_popup_hide() -> void:
	deny_menu_button.set_pressed_no_signal(false)


func _on_close_requested() -> void:
	# Closing the window is the same as denying this one use.
	tool_use_denied.emit(AllowDenyType.ONCE)
