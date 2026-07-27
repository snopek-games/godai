extends GutTest

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")


func _content_types(p_msg: ClaudeClient.Message) -> Array:
	return p_msg.content.map(func (c): return c.get_type())


func test_remove_tool_use() -> void:
	var msg := ClaudeClient.Message.new("assistant", [
		{type = "thinking", thinking = ""},
		{type = "text", text = "Let me look at the scene."},
		{type = "tool_use", id = "toolu_1", name = "get_current_scene", input = {}},
		{type = "tool_use", id = "toolu_2", name = "get_current_scene_tree", input = {}},
	])

	msg.remove_tool_use()

	# Everything the user might still want to read survives; only the requests
	# that will never be answered are dropped.
	assert_eq(_content_types(msg), ["thinking", "text"])


func test_remove_tool_use_without_any() -> void:
	var msg := ClaudeClient.Message.new("assistant", [
		{type = "text", text = "All done."},
	])

	msg.remove_tool_use()

	assert_eq(_content_types(msg), ["text"])


func test_remove_tool_use_leaving_nothing() -> void:
	# A turn cut off before it produced anything but the tool request. The caller
	# checks for this, and drops the message rather than sending empty content.
	var msg := ClaudeClient.Message.new("assistant", [
		{type = "tool_use", id = "toolu_1", name = "stop_project", input = {}},
	])

	msg.remove_tool_use()

	assert_eq(msg.content.size(), 0)


func test_remove_tool_use_leaves_the_rest_of_the_chat_alone() -> void:
	var chat := ClaudeClient.Chat.new()

	# An earlier turn that ran its tool and got an answer.
	chat.add_message(ClaudeClient.Message.new("assistant", [
		{type = "tool_use", id = "toolu_1", name = "get_current_scene", input = {}},
	]))
	chat.add_message(ClaudeClient.Message.new("user", [
		{type = "tool_result", tool_use_id = "toolu_1", content = "{}"},
	]))

	# A later turn, cut short before its tool could run.
	var truncated := ClaudeClient.Message.new("assistant", [
		{type = "text", text = "Now I'll save."},
		{type = "tool_use", id = "toolu_2", name = "save_scene", input = {}},
	])
	truncated.remove_tool_use()
	chat.add_message(truncated)

	# The answered pair is still intact; only the unanswered request is gone.
	assert_eq(_content_types(chat.messages[0]), ["tool_use"])
	assert_eq(_content_types(chat.messages[1]), ["tool_result"])
	assert_eq(_content_types(chat.messages[2]), ["text"])


func test_remove_tool_use_keeps_the_message_serializable() -> void:
	var msg := ClaudeClient.Message.new("assistant", [
		{type = "text", text = "Saving."},
		{type = "tool_use", id = "toolu_1", name = "save_scene", input = {}},
	])

	msg.remove_tool_use()

	assert_eq(msg.to_dict(), {
		role = "assistant",
		content = [{type = "text", text = "Saving."}],
	})
