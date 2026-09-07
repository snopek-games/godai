extends GutTest

const ToolUseAuthDialogScene = preload("res://addons/godai/ui/tool_use_auth_dialog.tscn")
const ToolUseAuthDialog = preload("res://addons/godai/ui/tool_use_auth_dialog.gd")

var _dialog


func before_each() -> void:
	_dialog = ToolUseAuthDialogScene.instantiate()
	add_child_autofree(_dialog)


func test_setup_fills_the_fields_and_menus() -> void:
	_dialog.setup_tool_use_auth_dialog("create_file", {path = "res://x.txt"})

	assert_eq(_dialog.name_field.text, "create_file")
	assert_eq(_dialog.input_field.value, {path = "res://x.txt"})
	assert_string_contains(_dialog.input_field.raw_text.text, "res://x.txt")
	assert_eq(_dialog.allow_menu.get_item_text(0), 'Allow "create_file" for this session')
	assert_eq(_dialog.allow_menu.get_item_text(1), 'Allow "create_file" always')
	assert_eq(_dialog.deny_menu.get_item_text(0), 'Deny "create_file" for this session')
	assert_eq(_dialog.deny_menu.get_item_text(1), 'Deny "create_file" always')


func test_buttons_emit_their_allow_deny_types() -> void:
	watch_signals(_dialog)

	_dialog.allow_button.pressed.emit()
	assert_signal_emitted_with_parameters(_dialog, "tool_use_allowed", [ToolUseAuthDialog.AllowDenyType.ONCE])

	_dialog.allow_menu.id_pressed.emit(ToolUseAuthDialog.AllowDenyType.TOOL_ALWAYS)
	assert_signal_emitted_with_parameters(_dialog, "tool_use_allowed", [ToolUseAuthDialog.AllowDenyType.TOOL_ALWAYS])

	_dialog.deny_button.pressed.emit()
	assert_signal_emitted_with_parameters(_dialog, "tool_use_denied", [ToolUseAuthDialog.AllowDenyType.ONCE])

	_dialog.deny_menu.id_pressed.emit(ToolUseAuthDialog.AllowDenyType.TOOL_FOR_SESSION)
	assert_signal_emitted_with_parameters(_dialog, "tool_use_denied", [ToolUseAuthDialog.AllowDenyType.TOOL_FOR_SESSION])


func test_closing_the_window_denies_once() -> void:
	watch_signals(_dialog)

	_dialog.close_requested.emit()

	assert_signal_emitted_with_parameters(_dialog, "tool_use_denied", [ToolUseAuthDialog.AllowDenyType.ONCE])
