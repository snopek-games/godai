extends GutTest

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")


func _content_types(p_msg: ClaudeClient.Message) -> Array:
	return p_msg.content.map(func (c): return c.get_type())


func test_message_content_from_dict_requires_a_type() -> void:
	assert_null(ClaudeClient.MessageContent.from_dict({text = "hi"}))
	assert_not_null(ClaudeClient.MessageContent.from_dict({type = "text", text = "hi"}))
	assert_push_error(1, "the missing type is reported")


func test_message_accepts_a_plain_string_inside_a_content_array() -> void:
	var msg := ClaudeClient.Message.new("user", ["hello", {type = "text", text = "world"}])

	assert_eq(msg.to_dict(), {
		role = "user",
		content = [{type = "text", text = "hello"}, {type = "text", text = "world"}],
	})


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


func test_repair_dangling_tool_use_keeps_answered_pairs() -> void:
	var chat := ClaudeClient.Chat.new()
	chat.add_message(ClaudeClient.Message.new("assistant", [
		{type = "tool_use", id = "toolu_1", name = "get_current_scene", input = {}},
	]))
	chat.add_message(ClaudeClient.Message.new("user", [
		{type = "tool_result", tool_use_id = "toolu_1", content = "{}"},
	]))
	chat.add_message(ClaudeClient.Message.new("assistant", [
		{type = "text", text = "Now I'll save."},
		{type = "tool_use", id = "toolu_2", name = "save_scene", input = {}},
	]))

	chat.repair_dangling_tool_use()

	assert_eq(chat.messages.size(), 4)
	assert_eq(_content_types(chat.messages[0]), ["tool_use"])
	assert_eq(_content_types(chat.messages[1]), ["tool_result"])
	assert_eq(_content_types(chat.messages[2]), ["text", "tool_use"])
	assert_eq(chat.messages[3].to_dict(), {
		role = "user",
		content = [{
			type = "tool_result",
			tool_use_id = "toolu_2",
			content = "Something went wrong and this tool didn't record its result. It may or may not have taken effect.",
			is_error = true,
		}],
	})


func test_repair_dangling_tool_use_answers_an_interrupted_restart() -> void:
	var chat := ClaudeClient.Chat.new()
	chat.add_message(ClaudeClient.Message.new("user", "restart please"))
	chat.add_message(ClaudeClient.Message.new("assistant", [
		{type = "tool_use", id = "toolu_1", name = "restart_editor", input = {}},
	]))

	chat.repair_dangling_tool_use()

	assert_eq(chat.messages.size(), 3)
	assert_eq(_content_types(chat.messages[1]), ["tool_use"],
		"the restart stays in the transcript so a resumed chat doesn't replay it")
	assert_eq(_content_types(chat.messages[2]), ["tool_result"])


func test_repair_dangling_tool_use_merges_into_the_following_user_message() -> void:
	var chat := ClaudeClient.Chat.new()
	chat.add_message(ClaudeClient.Message.new("assistant", [
		{type = "tool_use", id = "toolu_1", name = "get_current_scene", input = {}},
		{type = "tool_use", id = "toolu_2", name = "save_scene", input = {}},
	]))
	chat.add_message(ClaudeClient.Message.new("user", [
		{type = "tool_result", tool_use_id = "toolu_1", content = "{}"},
	]))

	chat.repair_dangling_tool_use()

	assert_eq(chat.messages.size(), 2)
	assert_eq(chat.messages[1].content.map(func (c): return c.data.get("tool_use_id")),
		["toolu_2", "toolu_1"])


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
