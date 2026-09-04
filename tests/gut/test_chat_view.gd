extends GutTest

const ChatViewScene = preload("res://addons/godai/ui/chat_view.tscn")
const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
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


func test_show_chat_renders_each_message_kind() -> void:
	var chat := ClaudeClient.Chat.new()
	chat.add_message(ClaudeClient.Message.new("user", "hello"))
	chat.add_message(ClaudeClient.Message.new("assistant", [
		{type = "text", text = "hi!"},
		{type = "tool_use", id = "toolu_1", name = TOOL, input = {}},
	]))
	chat.add_message(ClaudeClient.Message.new("user", [
		{type = "tool_result", tool_use_id = "toolu_1", content = "{}"},
	]))
	chat.add_message(ClaudeClient.Message.new("user", ClaudeClient.CANCELLED_MESSAGE))

	_view.show_chat(chat)

	var items := _items()
	assert_eq(items.size(), 4)
	assert_eq(items[0].label.text, "hello")
	assert_eq(items[1].text, "hi!")
	assert_eq(items[3].text, "Cancelled by user")

	# The tool item carries its use and result, shown when info is requested.
	watch_signals(items[2])
	items[2].info_button.pressed.emit()
	assert_signal_emitted_with_parameters(items[2], "info_requested", ["toolu_1", TOOL, {}, "{}"])


func test_show_chat_replaces_the_previous_chat() -> void:
	var first := ClaudeClient.Chat.new()
	first.add_message(ClaudeClient.Message.new("user", "hello"))
	_view.show_chat(first)

	var second := ClaudeClient.Chat.new()
	second.add_message(ClaudeClient.Message.new("user", "other"))
	second.add_message(ClaudeClient.Message.new("assistant", "reply"))
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
	_view.show_message(ClaudeClient.Message.new("assistant", [{type = "tool_use"}]))

	assert_eq(_items().size(), 1)

	_view.show_message(ClaudeClient.Message.new("user", [{type = "tool_result"}]))

	assert_eq(_items().size(), 1)


func test_tool_result_for_an_unknown_id_adds_nothing() -> void:
	_view.show_message(ClaudeClient.Message.new("user", [
		{type = "tool_result", tool_use_id = "nope", content = "{}"},
	]))

	assert_eq(_items().size(), 0)


func test_set_loading() -> void:
	_view.set_loading(true, "Cancelling")
	assert_true(_view.loading_label.visible)
	assert_eq(_view.loading_label.base_text, "Cancelling")

	_view.set_loading(false)
	assert_false(_view.loading_label.visible)
