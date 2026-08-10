@tool
extends EditorPlugin

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const GodaiPanelScene = preload("res://addons/godai/ui/godai_panel.tscn")
const GodaiPanel = preload("res://addons/godai/ui/godai_panel.gd")

var panel: GodaiPanel
var panel_button: Button
var shortcut := Shortcut.new()


func _enable_plugin() -> void:
	pass


func _disable_plugin() -> void:
	pass


func _enter_tree() -> void:
	GodaiEditorSettings.add_editor_settings()

	panel = GodaiPanelScene.instantiate()

	var key_event = InputEventKey.new()
	key_event.keycode = KEY_I
	key_event.ctrl_pressed = true
	key_event.command_or_control_autoremap = true
	shortcut.events = [key_event]

	panel_button = add_control_to_bottom_panel(panel, "Godai", shortcut)
	panel_button.pressed.connect(panel.show_panel)


func _exit_tree() -> void:
	if panel:
		remove_control_from_bottom_panel(panel)

		panel.queue_free()
		panel = null
