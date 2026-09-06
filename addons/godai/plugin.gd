@tool
extends EditorPlugin

const EditorGlobals = preload("res://addons/godai/editor_globals.gd")

const GodaiDebuggerPlugin = preload("res://addons/godai/debugger_plugin.gd")
const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const GodaiPanelScene = preload("res://addons/godai/ui/godai_panel.tscn")
const GodaiPanel = preload("res://addons/godai/ui/godai_panel.gd")

var debugger_plugin: GodaiDebuggerPlugin
var dock: EditorDock
var panel: GodaiPanel
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

	dock = EditorDock.new()
	dock.title = "Godai"
	dock.dock_icon = preload("res://addons/godai/ui/icons/godai.svg")
	dock.force_show_icon = true
	dock.default_slot = EditorDock.DOCK_SLOT_BOTTOM
	dock.available_layouts = EditorDock.DOCK_LAYOUT_ALL
	dock.dock_shortcut = shortcut
	dock.add_child(panel)
	add_dock(dock)
	dock.visibility_changed.connect(_on_dock_visibility_changed)


func _exit_tree() -> void:
	EditorGlobals.shutdown()

	if debugger_plugin:
		remove_debugger_plugin(debugger_plugin)
		debugger_plugin = null

	if dock:
		remove_dock(dock)
		dock.queue_free()
		dock = null
		panel = null


func _on_dock_visibility_changed() -> void:
	if dock.is_visible_in_tree():
		panel.show_panel()
