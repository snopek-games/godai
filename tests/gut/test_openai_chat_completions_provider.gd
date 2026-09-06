extends GutTest

const Chat = preload("res://addons/godai/chat/chat.gd")
const Provider = preload("res://addons/godai/chat/provider.gd")
const OpenAIProvider = preload("res://addons/godai/chat/provider/openai_chat_completions.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ModelInfo = preload("res://addons/godai/chat/model_info.gd")

var provider: OpenAIProvider
var chat: Chat
var options: Provider.RequestOptions


func before_each() -> void:
	provider = OpenAIProvider.new(OpenAIProvider.DEFAULT_URL, "test-key", "gpt-test")
	chat = Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "Hello"))
	options = Provider.RequestOptions.new()
	options.max_tokens = 1234
	options.effort = "high"


func _content_types(p_msg: Chat.Message) -> Array:
	return p_msg.content.map(func (c): return c.to_dict()["type"])


func test_build_request_url_and_headers() -> void:
	var request := provider.build_request(chat, options)

	assert_eq(request.url, "https://api.openai.com/v1/chat/completions")
	assert_eq(request.headers, PackedStringArray([
		"authorization: Bearer test-key",
		"content-type: application/json",
	]))


func test_build_request_accepts_a_url_without_a_trailing_slash() -> void:
	provider = OpenAIProvider.new("http://localhost:11434/v1", "", "llama")

	assert_eq(provider.build_request(chat, options).url, "http://localhost:11434/v1/chat/completions")


func test_build_request_payload() -> void:
	var payload := provider.build_request(chat, options).payload

	assert_eq(payload, {
		model = "gpt-test",
		messages = [{role = "user", content = "Hello"}],
		max_completion_tokens = 1234,
		reasoning_effort = "high",
	})


func test_build_request_sends_the_system_prompt_as_the_first_message() -> void:
	chat.system = "Be helpful."

	assert_eq(provider.build_request(chat, options).payload["messages"], [
		{role = "system", content = "Be helpful."},
		{role = "user", content = "Hello"},
	])


func test_build_request_converts_the_conversation() -> void:
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.ThinkingContent.new("hmm", "sig"),
		Chat.UnknownContent.new({type = "redacted_thinking", data = "opaque"}),
		Chat.TextContent.new("Saving."),
		Chat.ToolUseContent.new("call_1", "save_scene", {path = "a"}),
		Chat.ToolUseContent.new("call_2", "get_current_scene"),
	]))
	chat.add_message(Chat.Message.new(Chat.Role.USER, [
		Chat.ToolResultContent.new("call_1", "saved"),
		Chat.ToolResultContent.new("call_2", "boom", true),
		Chat.TextContent.new("Now what?"),
	]))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	assert_eq(messages, [
		{role = "user", content = "Hello"},
		{role = "assistant", content = "Saving.", tool_calls = [
			{id = "call_1", type = "function", function = {name = "save_scene", arguments = '{"path":"a"}'}},
			{id = "call_2", type = "function", function = {name = "get_current_scene", arguments = "{}"}},
		]},
		{role = "tool", tool_call_id = "call_1", content = "saved"},
		{role = "tool", tool_call_id = "call_2", content = "Error: boom"},
		{role = "user", content = "Now what?"},
	])


func test_build_request_assistant_message_shapes() -> void:
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("call_1", "save_scene")))
	chat.add_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("call_1", {ok = true})))
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, [Chat.TextContent.new("One."), Chat.TextContent.new("Two.")]))
	chat.add_message(Chat.Message.new(Chat.Role.USER, "Thanks"))
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ThinkingContent.new("only thoughts")))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	assert_eq(messages[1], {role = "assistant", tool_calls = [
		{id = "call_1", type = "function", function = {name = "save_scene", arguments = "{}"}},
	]}, "no content key when the turn was only tool calls")
	assert_eq(messages[2], {role = "tool", tool_call_id = "call_1", content = '{"ok":true}'},
		"non-string results are serialized")
	assert_eq(messages[3], {role = "assistant", content = "One.\n\nTwo."})
	assert_eq(messages[5], {role = "assistant", content = ""}, "a turn with nothing sendable stays in the alternation")


func test_build_request_unknown_model_sends_effort_verbatim() -> void:
	for level in ["low", "xhigh", "max"]:
		options.effort = level
		assert_eq(provider.build_request(chat, options).payload["reasoning_effort"], level)

	options.effort = ""
	assert_false(provider.build_request(chat, options).payload.has("reasoning_effort"))


func test_build_request_known_model_filters_effort() -> void:
	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["low", "medium", "high"]}]})

	options.effort = "medium"
	assert_eq(provider.build_request(chat, options).payload["reasoning_effort"], "medium")

	options.effort = "xhigh"
	assert_false(provider.build_request(chat, options).payload.has("reasoning_effort"))

	options.model_info = ModelInfo.new({})
	options.effort = "high"
	assert_false(provider.build_request(chat, options).payload.has("reasoning_effort"), "a model without reasoning options gets none")


func test_build_request_thinking_disabled() -> void:
	options.thinking = false

	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["low", "high"]}, {type = "toggle"}]})
	assert_eq(provider.build_request(chat, options).payload["reasoning_effort"], "none")

	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["low", "high"]}]})
	assert_eq(provider.build_request(chat, options).payload["reasoning_effort"], "high", "a model that can't switch thinking off keeps the effort")

	options.model_info = null
	assert_eq(provider.build_request(chat, options).payload["reasoning_effort"], "none")


func test_build_request_falls_back_to_max_tokens_when_dropped() -> void:
	options.dropped_options = [OpenAIProvider.OPTION_MAX_COMPLETION_TOKENS]

	var payload := provider.build_request(chat, options).payload

	assert_false(payload.has("max_completion_tokens"))
	assert_eq(payload["max_tokens"], 1234)
	assert_eq(payload["reasoning_effort"], "high")


func test_build_request_maps_tools() -> void:
	var schema := {type = "object", properties = {path = {type = "string"}}}
	options.tools = [
		ToolManager.CallbackTool.new("save_scene", "Save Scene", "Saves the scene.", func (_input): return null, schema),
		ToolManager.CallbackTool.new("bare", "Bare", "", func (_input): return null, {}),
	]

	var payload := provider.build_request(chat, options).payload

	assert_eq(payload["tools"], [
		{type = "function", function = {name = "save_scene", description = "Saves the scene.", parameters = schema}},
		{type = "function", function = {name = "bare", parameters = OpenAIProvider.EMPTY_PARAMETERS}},
	])


func test_parse_response_message() -> void:
	var response := provider.parse_response(200, {
		id = "chatcmpl-1",
		object = "chat.completion",
		choices = [{
			index = 0,
			finish_reason = "tool_calls",
			message = {
				role = "assistant",
				reasoning_content = "Let me think.",
				content = "Saving.",
				tool_calls = [
					{id = "call_1", type = "function", function = {name = "save_scene", arguments = '{"path": "a"}'}},
					{id = "call_2", type = "function", function = {name = "get_current_scene", arguments = ""}},
					{id = "call_3", type = "function", function = {name = "odd", arguments = "not json"}},
					"not a call",
				],
			},
		}],
		usage = {prompt_tokens = 100, completion_tokens = 20, total_tokens = 120, prompt_tokens_details = {cached_tokens = 30}},
	})

	assert_true(response.is_success())
	assert_eq(response.stop_reason, Provider.StopReason.TOOL_USE)
	assert_eq(response.usage.to_dict(), {input_tokens = 70, output_tokens = 20, cache_creation_input_tokens = 0, cache_read_input_tokens = 30})
	assert_eq(response.raw["id"], "chatcmpl-1")

	var msg := response.message
	assert_eq(msg.role, Chat.Role.ASSISTANT)
	assert_eq(_content_types(msg), ["thinking", "text", "tool_use", "tool_use", "tool_use"])
	assert_eq(msg.content[0].thinking, "Let me think.")
	assert_eq(msg.content[1].text, "Saving.")
	assert_eq(msg.content[2].id, "call_1")
	assert_eq(msg.content[2].name, "save_scene")
	assert_eq(msg.content[2].input, {path = "a"})
	assert_eq(msg.content[3].input, {}, "empty arguments become an empty input")
	assert_eq(msg.content[4].input, {}, "unparsable arguments become an empty input")


func test_parse_response_text_only() -> void:
	var response := provider.parse_response(200, {
		choices = [{finish_reason = "stop", message = {role = "assistant", content = "Hi!"}}],
	})

	assert_eq(response.stop_reason, Provider.StopReason.END_TURN)
	assert_eq(_content_types(response.message), ["text"])
	assert_eq(response.usage.to_dict(), Provider.Usage.new().to_dict())


func test_parse_response_null_content_with_tool_calls() -> void:
	var response := provider.parse_response(200, {
		choices = [{finish_reason = "stop", message = {role = "assistant", content = null,
			tool_calls = [{id = "call_1", type = "function", function = {name = "save_scene", arguments = "{}"}}]}}],
	})

	assert_eq(_content_types(response.message), ["tool_use"])
	assert_eq(response.stop_reason, Provider.StopReason.TOOL_USE, "tool calls always need answering")


func test_parse_response_truncated_tool_calls_keep_max_tokens() -> void:
	var response := provider.parse_response(200, {
		choices = [{finish_reason = "length", message = {role = "assistant", content = "",
			tool_calls = [{id = "call_1", type = "function", function = {name = "save_scene", arguments = "{"}}]}}],
	})

	assert_eq(response.stop_reason, Provider.StopReason.MAX_TOKENS)


func test_parse_response_finish_reasons() -> void:
	var expected := {
		stop = Provider.StopReason.END_TURN,
		tool_calls = Provider.StopReason.TOOL_USE,
		function_call = Provider.StopReason.TOOL_USE,
		length = Provider.StopReason.MAX_TOKENS,
		content_filter = Provider.StopReason.REFUSAL,
		something_new = Provider.StopReason.UNKNOWN,
	}
	for name in expected:
		var response := provider.parse_response(200, {choices = [{finish_reason = name, message = {content = "x"}}]})
		assert_eq(response.stop_reason, expected[name], name)


func test_parse_response_reasoning_field_variant() -> void:
	var response := provider.parse_response(200, {
		choices = [{finish_reason = "stop", message = {content = "Hi", reasoning = "Because."}}],
	})

	assert_eq(_content_types(response.message), ["thinking", "text"])
	assert_eq(response.message.content[0].thinking, "Because.")


func test_parse_response_without_choices_has_no_message() -> void:
	var response := provider.parse_response(200, {object = "chat.completion", choices = []})

	assert_true(response.is_success())
	assert_null(response.message)
	assert_eq(response.stop_reason, Provider.StopReason.UNKNOWN)


func test_parse_response_error_body() -> void:
	var response := provider.parse_response(400, {
		error = {message = "Unsupported parameter: 'reasoning_effort' is not supported with this model.", type = "invalid_request_error", param = "reasoning_effort", code = "unsupported_parameter"},
	})

	assert_true(response.is_error())
	assert_eq(response.get_error().type, "invalid_request_error")
	assert_string_contains(response.get_error().message, "reasoning_effort")
	assert_eq(response.get_error().param, "reasoning_effort")


func test_parse_response_error_falls_back_to_the_code() -> void:
	var response := provider.parse_response(200, {error = {message = "nope", code = "model_not_found"}})

	assert_true(response.is_error())
	assert_eq(response.get_error().type, "model_not_found")


func test_parse_response_non_2xx_without_an_error_body() -> void:
	var response := provider.parse_response(502, {message = "Bad Gateway"})

	assert_true(response.is_error())
	assert_eq(response.get_error().type, "unknown_error")


func test_get_rejected_option() -> void:
	var cases := [
		["Unknown parameter: 'max_completion_tokens'.", "", OpenAIProvider.OPTION_MAX_COMPLETION_TOKENS],
		["Unsupported parameter: 'max_completion_tokens' is not supported with this model.", "", OpenAIProvider.OPTION_MAX_COMPLETION_TOKENS],
		["Unsupported parameter: 'max_completion_tokens' is not supported with this model.", "max_completion_tokens", OpenAIProvider.OPTION_MAX_COMPLETION_TOKENS],
		["max_completion_tokens is too large", "", ""],
		["Invalid 'max_completion_tokens': integer above maximum value.", "max_completion_tokens", ""],
		["Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.", "max_tokens", ""],
		["Unsupported parameter: 'reasoning_effort' is not supported with this model.", "", OpenAIProvider.OPTION_REASONING_EFFORT],
		["Unsupported value: 'none' is not supported with this model. Supported values are: 'low', 'medium', and 'high'.", "reasoning_effort", OpenAIProvider.OPTION_REASONING_EFFORT],
		["Unsupported value: 'none' is not supported with this model.", "", ""],
		["Invalid value: 'none'. Supported values are: 'low', 'medium', and 'high'.", "reasoning_effort", OpenAIProvider.OPTION_REASONING_EFFORT],
		["Rate limit reached", "", ""],
	]
	for c in cases:
		assert_eq(provider.get_rejected_option(Provider.ResponseError.new("invalid_request_error", c[0], c[1])), c[2], "%s (param '%s')" % [c[0], c[1]])


func test_build_request_omits_dropped_reasoning_effort() -> void:
	options.dropped_options = [OpenAIProvider.OPTION_REASONING_EFFORT]

	assert_false(provider.build_request(chat, options).payload.has("reasoning_effort"))

	options.thinking = false
	assert_false(provider.build_request(chat, options).payload.has("reasoning_effort"), "thinking off has nothing left to send")
