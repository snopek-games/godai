extends GutTest

const ChatViewScene = preload("res://addons/godai/ui/chat_view.tscn")
const Chat = preload("res://addons/godai/chat/chat.gd")
const ChatClient = preload("res://addons/godai/chat/client.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const DefaultToolsLoader = preload("res://addons/godai/tools/default/loader.gd")

const TOOL := "get_current_project"

var _view


func before_each() -> void:
	_view = ChatViewScene.instantiate()
	var tools := ToolManager.new()
	DefaultToolsLoader.load_default_tools(tools)
	_view.tools = tools
	add_child_autofree(_view)


func _items() -> Array:
	return _view.chat_container.get_children().filter(
		func (c): return not c.is_queued_for_deletion())


func test_show_chat_renders_markdown_as_bbcode() -> void:
	var chat := Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "fix `x` and [b]this[/b]"))
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "**done** [see](https://a.b)"))

	_view.show_chat(chat)

	var items := _items()
	assert_eq(items[0].markdown_label.markdown, "fix `x` and [b]this[/b]")
	assert_eq(items[0].markdown_label.label.text, "fix [code]x[/code] and [lb]b]this[lb]/b]")
	assert_eq(items[1].markdown_label.markdown, "**done** [see](https://a.b)")
	assert_eq(items[1].markdown_label.label.text, "[b]done[/b] [url=https://a.b]see[/url]")


func test_show_chat_renders_each_message_kind() -> void:
	var chat := Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.TextContent.new("hi!"),
		Chat.ToolUseContent.new("toolu_1", TOOL),
	]))
	chat.add_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("toolu_1", "{}")))
	chat.add_message(Chat.Message.new(Chat.Role.USER, ChatClient.CANCELLED_MESSAGE))

	_view.show_chat(chat)

	var items := _items()
	assert_eq(items.size(), 4)
	assert_eq(items[0].markdown_label.markdown, "hello")
	assert_eq(items[1].markdown_label.markdown, "hi!")
	assert_eq(items[3].text, "Cancelled by user")

	# The tool item carries its use and result, shown when info is requested.
	watch_signals(items[2])
	items[2].info_button.pressed.emit()
	assert_signal_emitted_with_parameters(items[2], "info_requested", ["toolu_1", TOOL, {}, "{}"])


func test_show_chat_replaces_the_previous_chat() -> void:
	var first := Chat.new()
	first.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	_view.show_chat(first)

	var second := Chat.new()
	second.add_message(Chat.Message.new(Chat.Role.USER, "other"))
	second.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "reply"))
	_view.show_chat(second)

	assert_eq(_items().size(), 2)

	_view.show_chat(null)

	assert_eq(_items().size(), 0)


func test_show_error() -> void:
	_view.show_error("something broke")

	var items := _items()
	assert_eq(items.size(), 1)
	assert_eq(items[0].label.text, "something broke")


func test_tool_content_missing_its_keys_still_renders() -> void:
	_view.show_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("", "")))

	assert_eq(_items().size(), 1)

	_view.show_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("")))

	assert_eq(_items().size(), 1)


func test_tool_result_for_an_unknown_id_adds_nothing() -> void:
	_view.show_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("nope", "{}")))

	assert_eq(_items().size(), 0)


func test_set_loading() -> void:
	_view.set_loading(true, "Cancelling")
	assert_true(_view.loading_label.visible)
	assert_eq(_view.loading_label.base_text, "Cancelling")

	_view.set_loading(false)
	assert_false(_view.loading_label.visible)


func test_welcome_note_shows_only_before_a_chat_starts() -> void:
	_view.show_chat(null)
	assert_true(_view.welcome_note.visible)
	assert_true(_view.welcome_note.get_started_label.visible)
	assert_false(_view.welcome_note.fix_section.visible)

	_view.show_message(Chat.Message.new(Chat.Role.USER, "hello"))
	assert_false(_view.welcome_note.visible)

	var chat := Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	_view.show_chat(chat)
	assert_false(_view.welcome_note.visible)

	_view.show_chat(null)
	assert_true(_view.welcome_note.visible)


func test_welcome_note_names_each_problem() -> void:
	_view.show_chat(null)

	_view.api_configured = false
	assert_true(_view.welcome_note.fix_section.visible)
	assert_false(_view.welcome_note.get_started_label.visible)
	assert_eq(_view.welcome_note.fix_label.text, _view.FIX_NOT_CONFIGURED_TEXT)
	assert_true(_view.welcome_note.settings_button.visible)
	assert_false(_view.welcome_note.go_online_button.visible)

	_view.online = false
	assert_eq(_view.welcome_note.fix_label.text, _view.FIX_BOTH_TEXT)
	assert_true(_view.welcome_note.settings_button.visible)
	assert_true(_view.welcome_note.go_online_button.visible)

	_view.api_configured = true
	assert_eq(_view.welcome_note.fix_label.text, _view.FIX_OFFLINE_TEXT)
	assert_false(_view.welcome_note.settings_button.visible)
	assert_true(_view.welcome_note.go_online_button.visible)

	_view.online = true
	assert_false(_view.welcome_note.fix_section.visible)
	assert_true(_view.welcome_note.get_started_label.visible)


func test_status_messages_only_show_on_editor_chats() -> void:
	_view.api_configured = false
	_view.online = false

	_view.show_chat(null)
	assert_false(_view.not_configured_message.visible, "not on <new>")
	assert_false(_view.offline_message.visible)

	_view.show_chat(Chat.new(), true)
	assert_false(_view.not_configured_message.visible, "not on external chats")
	assert_false(_view.offline_message.visible)

	_view.show_chat(Chat.new())
	assert_true(_view.not_configured_message.visible)
	assert_false(_view.offline_message.visible, "configuration comes before going online")

	_view.api_configured = true
	assert_false(_view.not_configured_message.visible)
	assert_true(_view.offline_message.visible)

	_view.online = true
	assert_false(_view.not_configured_message.visible)
	assert_false(_view.offline_message.visible)


func test_fix_buttons_forward_their_requests() -> void:
	watch_signals(_view)

	_view.welcome_note.settings_button.pressed.emit()
	_view.not_configured_message.button.pressed.emit()
	assert_signal_emit_count(_view, "settings_requested", 2)

	_view.welcome_note.go_online_button.pressed.emit()
	_view.offline_message.button.pressed.emit()
	assert_signal_emit_count(_view, "go_online_requested", 2)
