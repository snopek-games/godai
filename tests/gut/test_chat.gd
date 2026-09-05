extends GutTest

const Chat = preload("res://addons/godai/chat/chat.gd")


func _content_types(p_msg: Chat.Message) -> Array:
	return p_msg.content.map(func (c): return c.to_dict()["type"])


func test_message_content_from_dict_requires_a_type() -> void:
	assert_null(Chat.MessageContent.from_dict({text = "hi"}))
	assert_not_null(Chat.MessageContent.from_dict({type = "text", text = "hi"}))
	assert_push_error(1, "the missing type is reported")


func test_message_content_from_dict_picks_the_class_for_each_type() -> void:
	var text := Chat.MessageContent.from_dict({type = "text", text = "hi"})
	assert_true(text is Chat.TextContent)
	assert_eq(text.text, "hi")

	var tool_use := Chat.MessageContent.from_dict({type = "tool_use", id = "toolu_1", name = "save_scene", input = {path = "a"}})
	assert_true(tool_use is Chat.ToolUseContent)
	assert_eq(tool_use.id, "toolu_1")
	assert_eq(tool_use.name, "save_scene")
	assert_eq(tool_use.input, {path = "a"})

	var tool_result := Chat.MessageContent.from_dict({type = "tool_result", tool_use_id = "toolu_1", content = "{}", is_error = true})
	assert_true(tool_result is Chat.ToolResultContent)
	assert_eq(tool_result.tool_use_id, "toolu_1")
	assert_eq(tool_result.content, "{}")
	assert_true(tool_result.is_error)

	var thinking := Chat.MessageContent.from_dict({type = "thinking", thinking = "hmm", signature = "sig"})
	assert_true(thinking is Chat.ThinkingContent)
	assert_eq(thinking.thinking, "hmm")
	assert_eq(thinking.signature, "sig")


func test_message_content_from_dict_tolerates_missing_fields() -> void:
	var tool_use: Chat.ToolUseContent = Chat.MessageContent.from_dict({type = "tool_use"})
	assert_eq(tool_use.id, "")
	assert_eq(tool_use.name, "")
	assert_eq(tool_use.input, {})

	var odd_input: Chat.ToolUseContent = Chat.MessageContent.from_dict({type = "tool_use", input = "not a dict"})
	assert_eq(odd_input.input, {})

	var tool_result: Chat.ToolResultContent = Chat.MessageContent.from_dict({type = "tool_result"})
	assert_eq(tool_result.tool_use_id, "")
	assert_eq(tool_result.content, "")
	assert_false(tool_result.is_error)


func test_unknown_content_round_trips_its_data() -> void:
	var data := {type = "redacted_thinking", data = "opaque"}
	var unknown := Chat.MessageContent.from_dict(data)

	assert_true(unknown is Chat.UnknownContent)
	assert_eq(unknown.to_dict(), data)


func test_content_to_dict() -> void:
	assert_eq(Chat.TextContent.new("hi").to_dict(), {type = "text", text = "hi"})
	assert_eq(Chat.ToolUseContent.new("toolu_1", "save_scene", {path = "a"}).to_dict(),
		{type = "tool_use", id = "toolu_1", name = "save_scene", input = {path = "a"}})
	assert_eq(Chat.ToolResultContent.new("toolu_1", "{}").to_dict(),
		{type = "tool_result", tool_use_id = "toolu_1", content = "{}", is_error = false})
	assert_eq(Chat.ThinkingContent.new("hmm", "sig").to_dict(), {type = "thinking", thinking = "hmm", signature = "sig"})


func test_message_accepts_a_plain_string_inside_a_content_array() -> void:
	var msg := Chat.Message.new(Chat.Role.USER, ["hello", Chat.TextContent.new("world")])

	assert_eq(msg.to_dict(), {
		role = "user",
		content = [{type = "text", text = "hello"}, {type = "text", text = "world"}],
	})


func test_message_accepts_a_single_string_or_content() -> void:
	assert_eq(Chat.Message.new(Chat.Role.USER, "hello").to_dict()["content"], [{type = "text", text = "hello"}])
	assert_eq(Chat.Message.new(Chat.Role.ASSISTANT, Chat.TextContent.new("hi")).to_dict()["content"], [{type = "text", text = "hi"}])


func test_message_rejects_other_content() -> void:
	var msg := Chat.Message.new(Chat.Role.USER, [42, "ok"])

	assert_eq(_content_types(msg), ["text"])
	assert_push_error(1, "the invalid item is reported")


func test_message_from_dict_round_trips() -> void:
	var data := {
		role = "assistant",
		content = [
			{type = "thinking", thinking = "", signature = "sig"},
			{type = "text", text = "Saving."},
			{type = "tool_use", id = "toolu_1", name = "save_scene", input = {}},
		],
	}

	var msg := Chat.Message.from_dict(data)

	assert_eq(msg.role, Chat.Role.ASSISTANT)
	assert_eq(msg.to_dict(), data)


func test_message_from_dict_accepts_a_plain_string_as_content() -> void:
	var msg := Chat.Message.from_dict({role = "user", content = "hello"})

	assert_eq(msg.to_dict()["content"], [{type = "text", text = "hello"}])


func test_message_from_dict_requires_a_known_role_and_content() -> void:
	assert_null(Chat.Message.from_dict({content = []}))
	assert_null(Chat.Message.from_dict({role = "user"}))
	assert_null(Chat.Message.from_dict({role = "system", content = []}))
	assert_push_error(3, "each problem is reported")


func test_remove_tool_use() -> void:
	var msg := Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.ThinkingContent.new(""),
		Chat.TextContent.new("Let me look at the scene."),
		Chat.ToolUseContent.new("toolu_1", "get_current_scene"),
		Chat.ToolUseContent.new("toolu_2", "get_current_scene_tree"),
	])

	msg.remove_tool_use()

	# Everything the user might still want to read survives; only the requests
	# that will never be answered are dropped.
	assert_eq(_content_types(msg), ["thinking", "text"])


func test_remove_tool_use_without_any() -> void:
	var msg := Chat.Message.new(Chat.Role.ASSISTANT, "All done.")

	msg.remove_tool_use()

	assert_eq(_content_types(msg), ["text"])


func test_remove_tool_use_leaving_nothing() -> void:
	# A turn cut off before it produced anything but the tool request. The caller
	# checks for this, and drops the message rather than sending empty content.
	var msg := Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("toolu_1", "stop_project"))

	msg.remove_tool_use()

	assert_eq(msg.content.size(), 0)


func test_remove_tool_use_leaves_the_rest_of_the_chat_alone() -> void:
	var chat := Chat.new()

	# An earlier turn that ran its tool and got an answer.
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("toolu_1", "get_current_scene")))
	chat.add_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("toolu_1", "{}")))

	# A later turn, cut short before its tool could run.
	var truncated := Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.TextContent.new("Now I'll save."),
		Chat.ToolUseContent.new("toolu_2", "save_scene"),
	])
	truncated.remove_tool_use()
	chat.add_message(truncated)

	# The answered pair is still intact; only the unanswered request is gone.
	assert_eq(_content_types(chat.messages[0]), ["tool_use"])
	assert_eq(_content_types(chat.messages[1]), ["tool_result"])
	assert_eq(_content_types(chat.messages[2]), ["text"])


func test_repair_dangling_tool_use_keeps_answered_pairs() -> void:
	var chat := Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("toolu_1", "get_current_scene")))
	chat.add_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("toolu_1", "{}")))
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.TextContent.new("Now I'll save."),
		Chat.ToolUseContent.new("toolu_2", "save_scene"),
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
			content = Chat.DANGLING_TOOL_USE_MESSAGE,
			is_error = true,
		}],
	})


func test_repair_dangling_tool_use_answers_an_interrupted_restart() -> void:
	var chat := Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "restart please"))
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("toolu_1", "restart_editor")))

	chat.repair_dangling_tool_use()

	assert_eq(chat.messages.size(), 3)
	assert_eq(_content_types(chat.messages[1]), ["tool_use"],
		"the restart stays in the transcript so a resumed chat doesn't replay it")
	assert_eq(_content_types(chat.messages[2]), ["tool_result"])


func test_repair_dangling_tool_use_merges_into_the_following_user_message() -> void:
	var chat := Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.ToolUseContent.new("toolu_1", "get_current_scene"),
		Chat.ToolUseContent.new("toolu_2", "save_scene"),
	]))
	chat.add_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("toolu_1", "{}")))

	chat.repair_dangling_tool_use()

	assert_eq(chat.messages.size(), 2)
	assert_eq(chat.messages[1].content.map(func (c): return c.tool_use_id), ["toolu_2", "toolu_1"])


func test_remove_tool_use_keeps_the_message_serializable() -> void:
	var msg := Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.TextContent.new("Saving."),
		Chat.ToolUseContent.new("toolu_1", "save_scene"),
	])

	msg.remove_tool_use()

	assert_eq(msg.to_dict(), {
		role = "assistant",
		content = [{type = "text", text = "Saving."}],
	})
