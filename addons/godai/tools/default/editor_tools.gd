extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const VerifiedSettingsTool = preload("res://addons/godai/tools/default/verified_settings_tool.gd")
const EditorGlobals = preload("res://addons/godai/editor_globals.gd")
const Utils = preload("res://addons/godai/utils.gd")
const CustomLogger = preload("res://addons/godai/custom_logger.gd")
const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")

const GODAI_SETTING_MESSAGE = "'%s' is a Godai setting; those are only accessible to the user, via Editor Settings."


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(EditorGetSettings.new(p_data["get_editor_settings"]))
	p_tools.register_tool(EditorSetSettings.new(p_data["set_editor_settings"]))
	p_tools.register_tool(EditorRestart.new(p_data["restart_editor"]))
	p_tools.register_tool(EditorClose.new(p_data["close_editor"]))
	p_tools.register_tool(LogGetMessages.new(p_data["get_log_messages"]))
	p_tools.register_tool(LogClearMessages.new(p_data["clear_log_messages"]))
	p_tools.register_tool(EditorScriptExecute.new(p_data["execute_editor_script"]))


class EditorGetSettings extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var names: Array = p_input.get('names', [])
		var include_defaults: bool = p_input.get('include_defaults', false)
		var enums_as_ints: bool = bool(p_input.get('enums_as_ints', false))

		for name in names:
			if GodaiEditorSettings.is_godai_setting(name):
				return ToolResult.rejected({errors = [GODAI_SETTING_MESSAGE % name]})

		var result := Utils.get_settings_map(EditorInterface.get_editor_settings(), names, include_defaults, enums_as_ints)
		if result.has('error'):
			return ToolResult.rejected({errors = [result['error']]})

		# Underscored (editor-internal) settings are hidden from the full listing but still reachable by name.
		var settings: Dictionary = result['settings']
		for name in settings.keys():
			if GodaiEditorSettings.is_godai_setting(name) or (names.is_empty() and name.begins_with("_")):
				settings.erase(name)

		var resolved := { settings = settings }
		var translated := PackedStringArray()
		for name in result['translated']:
			if settings.has(name):
				translated.append(name)
		if not translated.is_empty():
			resolved['notes'] = [Utils.enum_translation_note(translated)]
		return ToolResult.resolved(resolved)


class EditorSetSettings extends VerifiedSettingsTool:
	func get_properties_tool_name() -> String:
		return "get_editor_settings"

	func get_settings_object() -> Object:
		return EditorInterface.get_editor_settings()

	func check_setting_allowed(p_name: String) -> String:
		if GodaiEditorSettings.is_godai_setting(p_name):
			return GODAI_SETTING_MESSAGE % p_name
		return ""

	# The editor has no API to force editor settings to disk; it persists
	# them itself (e.g. on shutdown), so save_settings stays a no-op.


class EditorRestart extends DefaultTool:
	# Set in the environment by the functional tests, so exercising this tool
	# doesn't actually shut down (and kill) the editor under test.
	const DISABLE_CLOSE_ENV := "GODAI_DISABLE_CLOSE"

	func execute(p_input) -> ToolResult:
		var save := not bool(p_input.get('skip_save', false))

		# Unattended (e.g. automated): there's no user to prompt, so go ahead.
		if GodaiEditorSettings.is_unattended():
			var result := ToolResult.resolved({success = true})
			_restart.call_deferred(save)
			return result

		# Otherwise, prompt the user before restarting, so they don't lose work.
		var result := ToolResult.new()

		var dialog := ConfirmationDialog.new()
		dialog.title = "Restart Editor"
		if save:
			dialog.dialog_text = "Save all changes and restart the Godot editor?"
			dialog.ok_button_text = "Save & Restart"
		else:
			dialog.dialog_text = "Restart the Godot editor, discarding any unsaved changes?"
			dialog.ok_button_text = "Restart"

		dialog.confirmed.connect(func () -> void:
			result.resolve({success = true})
			_restart.call_deferred(save)
		)
		# Both the cancel button and closing the dialog (Escape / window close)
		# count as declining the restart.
		var on_canceled := func () -> void:
			result.reject({errors = ["The user declined to restart the editor"]})
		dialog.canceled.connect(on_canceled)
		dialog.close_requested.connect(on_canceled)
		# Clean up the dialog once it's dismissed, however that happened.
		dialog.visibility_changed.connect(func () -> void:
			if not dialog.visible:
				dialog.queue_free()
		)

		EditorInterface.popup_dialog_centered(dialog)

		return result

	func _restart(p_save: bool) -> void:
		# A running game is a separate process that would outlive the editor.
		EditorInterface.stop_playing_scene()

		if p_save:
			EditorInterface.save_all_scenes()

		# The functional tests disable the actual restart, so the editor under
		# test survives (everything up to this point still runs).
		if OS.get_environment(DISABLE_CLOSE_ENV) != "":
			return

		# Not EditorInterface.restart_editor(): its unsaved-changes prompt has no
		# user to answer it unattended. OS.get_cmdline_args() strips --editor/--path
		# (and the headless flags), so those are rebuilt and the rest kept.
		var args := PackedStringArray(["--editor", "--path", ProjectSettings.globalize_path("res://").simplify_path()])
		if DisplayServer.get_name() == "headless":
			args.append_array(["--display-driver", "headless", "--audio-driver", "Dummy"])
		args.append_array(OS.get_cmdline_args())
		OS.set_restart_on_exit(true, args)
		Engine.get_main_loop().quit()


class EditorClose extends DefaultTool:
	# Set in the environment by the functional tests, so exercising this tool
	# doesn't actually close (and kill) the editor under test.
	const DISABLE_CLOSE_ENV := "GODAI_DISABLE_CLOSE"

	func execute(p_input) -> ToolResult:
		var save := not bool(p_input.get('skip_save', false))

		# Unattended (e.g. automated): there's no user to prompt, so go ahead.
		if GodaiEditorSettings.is_unattended():
			var result := ToolResult.resolved({success = true})
			_close.call_deferred(save)
			return result

		# Otherwise, prompt the user before closing, so they don't lose work.
		var result := ToolResult.new()

		var dialog := ConfirmationDialog.new()
		dialog.title = "Close Editor"
		if save:
			dialog.dialog_text = "Save all changes and close the Godot editor?"
			dialog.ok_button_text = "Save & Close"
		else:
			dialog.dialog_text = "Close the Godot editor, discarding any unsaved changes?"
			dialog.ok_button_text = "Close"

		dialog.confirmed.connect(func () -> void:
			result.resolve({success = true})
			_close.call_deferred(save)
		)
		# Both the cancel button and closing the dialog (Escape / window close)
		# count as declining to close the editor.
		var on_canceled := func () -> void:
			result.reject({errors = ["The user declined to close the editor"]})
		dialog.canceled.connect(on_canceled)
		dialog.close_requested.connect(on_canceled)
		# Clean up the dialog once it's dismissed, however that happened.
		dialog.visibility_changed.connect(func () -> void:
			if not dialog.visible:
				dialog.queue_free()
		)

		EditorInterface.popup_dialog_centered(dialog)

		return result

	func _close(p_save: bool) -> void:
		# A running game is a separate process that would outlive the editor.
		EditorInterface.stop_playing_scene()

		if p_save:
			EditorInterface.save_all_scenes()

		# The functional tests disable the actual shutdown, so the editor under
		# test survives (everything up to this point still runs).
		if OS.get_environment(DISABLE_CLOSE_ENV) != "":
			return

		Engine.get_main_loop().quit()


class LogGetMessages extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var count := int(p_input.get('count', 100))

		var messages := EditorGlobals.logger.get_messages()
		if count > 0 and messages.size() > count:
			messages = messages.slice(messages.size() - count)

		return ToolResult.resolved({ messages = messages })


class LogClearMessages extends DefaultTool:
	func execute(p_input) -> ToolResult:
		EditorGlobals.logger.clear()
		return ToolResult.resolved({ success = true })


class EditorScriptExecute extends DefaultTool:
	const SCRIPT_TEMPLATE = """@tool
extends Node

const __Utils = preload("res://addons/godai/utils.gd")

signal __run_completed(success: bool)

func __run():
	# Await just in case the user code does.
	var err = await __user_code()
	__run_completed.emit(err == OK)

func editor_undo_redo_create_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	__Utils.editor_undo_redo_create_node(p_undo_redo, p_parent, p_child)

func editor_undo_redo_remove_node(p_undo_redo: EditorUndoRedoManager, p_parent: Node, p_child: Node) -> void:
	__Utils.editor_undo_redo_remove_node(p_undo_redo, p_parent, p_child)

func __user_code() -> Error:
	# USER CODE START
{user_code}
	# USER CODE END
	return OK
"""

	var logger: CustomLogger

	func _init(p_data: Dictionary) -> void:
		super._init(p_data)
		logger = CustomLogger.new()
		OS.add_logger(logger)

	func _notification(p_what: int) -> void:
		match p_what:
			NOTIFICATION_PREDELETE:
				OS.remove_logger(logger)

	func execute(p_input) -> ToolResult:
		var code: String = p_input.get('code', '')

		var main_loop = Engine.get_main_loop()
		if not main_loop is SceneTree:
			return ToolResult.rejected({errors = ["No scene tree"]})

		var root_node: Node = main_loop.get_root()
		if not root_node:
			return ToolResult.rejected({errors = ["No root node"]})

		var full_source = SCRIPT_TEMPLATE.replace('{user_code}', _process_user_code(code))

		logger.start()

		var script = GDScript.new()
		script.source_code = full_source

		var script_error = script.reload()
		if script_error != OK:
			return ToolResult.rejected({errors = ["Script failed to parse"], output = logger.stop()})

		var script_node = Node.new()
		script_node.name = "EditorScriptNode"
		script_node.script = script
		root_node.add_child(script_node)

		if not script_node.has_method("__run") or not script_node.has_signal("__run_completed"):
			script_node.queue_free()
			return ToolResult.rejected({errors = ["Script parsed but is malformed"]})

		var result := ToolResult.new()

		script_node.connect("__run_completed", _on_run_completed.bind(script_node, result))
		script_node.call("__run")

		# @todo: we probably want some kind of timeout?

		return result

	func _process_user_code(p_code: String) -> String:
		var output := PackedStringArray()

		var first_space_count := 0

		for line in p_code.split("\n"):
			var processed: String = line

			# All lines need at least one tab.
			var tabs := "\t"

			# Count the leading spaces (if any).
			var space_count := 0
			for i in range(processed.length()):
				if processed[i] != " ":
					break
				space_count += 1

			# If this is the first line with spaces, capture that number.
			if first_space_count == 0:
				first_space_count = space_count

			# Replace the spaces with tabs.
			if space_count > 0:
				processed = processed.substr(space_count)
				# Assume that the first space count is the tab width.
				for i in range(space_count / first_space_count):
					tabs += "\t"

			output.push_back(tabs + processed)

		return "\n".join(output)

	func _on_run_completed(p_success: bool, p_script_node: Node, p_result: ToolResult) -> void:
		if p_success:
			p_result.resolve({success = true, output = logger.stop()})
		else:
			p_result.reject({errors = ["Failed to execute script"], output = logger.stop()})

		p_script_node.queue_free()
