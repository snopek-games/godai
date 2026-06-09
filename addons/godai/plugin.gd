@tool
extends EditorPlugin

const GodaiPanelScene = preload("res://addons/godai/ui/godai_panel.tscn")
const GodaiPanel = preload("res://addons/godai/ui/godai_panel.gd")

var panel: GodaiPanel
var panel_button: Button
var shortcut := Shortcut.new()


func _add_editor_setting(p_name: String, p_type: int, p_default, p_hint = null, p_hint_string = null) -> void:
	var settings: EditorSettings = EditorInterface.get_editor_settings()

	if not settings.has_setting(p_name):
		settings.set_setting(p_name, p_default)

	settings.set_initial_value(p_name, p_default, false)

	var info := {
		name = p_name,
		type = p_type,
	}
	if p_hint != null:
		info['hint'] = p_hint
	if p_hint_string != null:
		info['hint_string'] = p_hint_string

	settings.add_property_info(info)


func add_editor_settings() -> void:
	_add_editor_setting(GodaiPanel.ANTHROPIC_API_KEY_SETTING, TYPE_STRING, "", PROPERTY_HINT_PASSWORD)
	_add_editor_setting(GodaiPanel.MCP_TRANSPORT_SETTING, TYPE_INT, 0, PROPERTY_HINT_ENUM, "WebSocket,HTTP")


func _enable_plugin() -> void:
	pass


func _disable_plugin() -> void:
	pass


func _enter_tree() -> void:
	add_editor_settings()

	panel = GodaiPanelScene.instantiate()

	var key_event = InputEventKey.new()
	key_event.keycode = KEY_I
	key_event.ctrl_pressed = true
	key_event.command_or_control_autoremap = true
	shortcut.events = [key_event]

	panel_button = add_control_to_bottom_panel(panel, "AI", shortcut)
	panel_button.pressed.connect(panel.show_panel)


func _exit_tree() -> void:
	if panel:
		remove_control_from_bottom_panel(panel)

		panel.queue_free()
		panel = null
