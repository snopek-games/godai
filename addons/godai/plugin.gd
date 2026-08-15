@tool
extends EditorPlugin

const EditorGlobals = preload("res://addons/godai/editor_globals.gd")

const GodaiDebuggerPlugin = preload("res://addons/godai/debugger_plugin.gd")
const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const GodaiPanelScene = preload("res://addons/godai/ui/godai_panel.tscn")
const GodaiPanel = preload("res://addons/godai/ui/godai_panel.gd")

var debugger_plugin: GodaiDebuggerPlugin
var panel: GodaiPanel
var panel_button: Button
var shortcut := Shortcut.new()


func _enable_plugin() -> void:
	add_autoload_singleton("Godai", "res://addons/godai/game/godai.gd")


func _disable_plugin() -> void:
	remove_autoload_singleton("Godai")


func _enter_tree() -> void:
	EditorGlobals.setup()

	GodaiEditorSettings.add_editor_settings()

	debugger_plugin = GodaiDebuggerPlugin.new()
	add_debugger_plugin(debugger_plugin)

	panel = GodaiPanelScene.instantiate()

	var key_event = InputEventKey.new()
	key_event.keycode = KEY_I
	key_event.ctrl_pressed = true
	key_event.command_or_control_autoremap = true
	shortcut.events = [key_event]

	panel_button = add_control_to_bottom_panel(panel, "Godai", shortcut)
	panel_button.pressed.connect(panel.show_panel)


func _exit_tree() -> void:
	EditorGlobals.shutdown()

	if debugger_plugin:
		remove_debugger_plugin(debugger_plugin)
		debugger_plugin = null

	if panel:
		remove_control_from_bottom_panel(panel)

		panel.queue_free()
		panel = null
