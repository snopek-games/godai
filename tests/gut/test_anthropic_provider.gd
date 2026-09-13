extends GutTest

const Chat = preload("res://addons/godai/chat/chat.gd")
const Provider = preload("res://addons/godai/chat/provider.gd")
const AnthropicProvider = preload("res://addons/godai/chat/provider/anthropic.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ModelInfo = preload("res://addons/godai/chat/model_info.gd")

var provider: AnthropicProvider
var chat: Chat
var options: Provider.RequestOptions


func _large_image(p_width: int, p_height: int) -> Image:
	var img := Image.create_empty(p_width, p_height, false, Image.FORMAT_RGB8)
	img.fill(Color.BLUE)
	return img


func _png_size(p_base64: String) -> Vector2i:
	var img := Image.new()
	img.load_png_from_buffer(Marshalls.base64_to_raw(p_base64))
	return img.get_size()


func before_each() -> void:
	provider = AnthropicProvider.new(AnthropicProvider.DEFAULT_URL, "test-key", "claude-test")
	chat = Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "Hello"))
	options = Provider.RequestOptions.new()
	options.max_tokens = 1234
	options.effort = "high"


func test_build_request_url_and_headers() -> void:
	var request := provider.build_request(chat, options)

	assert_eq(request.url, "https://api.anthropic.com/v1/messages")
	assert_eq(request.headers, PackedStringArray([
		"x-api-key: test-key",
		"anthropic-version: " + AnthropicProvider.API_VERSION,
		"content-type: application/json",
	]))


func test_build_request_accepts_a_url_without_a_trailing_slash() -> void:
	provider = AnthropicProvider.new("http://localhost:1234/v1", "", "claude-test")

	assert_eq(provider.build_request(chat, options).url, "http://localhost:1234/v1/messages")


func test_build_request_payload() -> void:
	var payload := provider.build_request(chat, options).payload

	assert_eq(payload, {
		model = "claude-test",
		max_tokens = 1234,
		messages = [{role = "user", content = [{type = "text", text = "Hello"}]}],
		thinking = {type = "adaptive"},
		output_config = {effort = "high"},
	})


func test_build_request_sends_the_system_prompt() -> void:
	assert_false(provider.build_request(chat, options).payload.has("system"))

	chat.system = "Be helpful."

	assert_eq(provider.build_request(chat, options).payload["system"], "Be helpful.")


func test_build_request_maps_every_content_type() -> void:
	chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, [
		Chat.ThinkingContent.new("", "sig"),
		Chat.UnknownContent.new({type = "redacted_thinking", data = "opaque"}),
		Chat.TextContent.new("Saving."),
		Chat.ToolUseContent.new("toolu_1", "save_scene", {path = "a"}),
	]))
	chat.add_message(Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new("toolu_1", "{}", true)))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	assert_eq(messages[1], {role = "assistant", content = [
		{type = "thinking", thinking = "", signature = "sig"},
		{type = "redacted_thinking", data = "opaque"},
		{type = "text", text = "Saving."},
		{type = "tool_use", id = "toolu_1", name = "save_scene", input = {path = "a"}},
	]})
	assert_eq(messages[2], {role = "user", content = [
		{type = "tool_result", tool_use_id = "toolu_1", content = "{}", is_error = true},
	]})


func test_build_request_tool_result_with_image_attachments() -> void:
	var result := Chat.ToolResultContent.new("toolu_1", "took a screenshot")
	result.add_attachment(Chat.ImageContent.new("png", PackedByteArray([1, 2, 3])))
	chat.add_message(Chat.Message.new(Chat.Role.USER, result))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	assert_eq(messages[1], {role = "user", content = [
		{type = "tool_result", tool_use_id = "toolu_1", is_error = false, content = [
			{type = "text", text = "took a screenshot"},
			{type = "image", source = {type = "base64", media_type = "image/png", data = "AQID"}},
		]},
	]})


func test_build_request_tool_result_attachment_with_structured_content() -> void:
	var result := Chat.ToolResultContent.new("toolu_1", {ok = true})
	result.add_attachment(Chat.ImageContent.new("webp", PackedByteArray([1, 2, 3])))
	chat.add_message(Chat.Message.new(Chat.Role.USER, result))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	assert_eq(messages[1]["content"][0]["content"], [
		{type = "text", text = '{"ok":true}'},
		{type = "image", source = {type = "base64", media_type = "image/webp", data = "AQID"}},
	], "non-string results are serialized into the text block")


func test_build_request_tool_result_attachment_without_text() -> void:
	var result := Chat.ToolResultContent.new("toolu_1", "")
	result.add_attachment(Chat.ImageContent.new("png", PackedByteArray([1, 2, 3])))
	chat.add_message(Chat.Message.new(Chat.Role.USER, result))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	assert_eq(messages[1]["content"][0]["content"], [
		{type = "image", source = {type = "base64", media_type = "image/png", data = "AQID"}},
	], "the API rejects empty text blocks")


func test_build_request_downscales_images_in_older_tool_results() -> void:
	for i in range(Provider.FULL_SIZE_ATTACHMENT_RESULTS + 1):
		var result := Chat.ToolResultContent.new("toolu_%d" % i, "shot")
		result.add_attachment(Chat.ImageContent.from_image(_large_image(1600, 800)))
		chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new("toolu_%d" % i, "take_screenshot")))
		chat.add_message(Chat.Message.new(Chat.Role.USER, result))

	var messages: Array = provider.build_request(chat, options).payload["messages"]

	var sizes := []
	for i in range(2, messages.size(), 2):
		sizes.push_back(_png_size(messages[i]["content"][0]["content"][1]["source"]["data"]))
	assert_eq(sizes, [Vector2i(1000, 500), Vector2i(1600, 800), Vector2i(1600, 800)], "only the oldest result is past the full-size window")


func test_build_request_unknown_model_without_settings_sends_no_reasoning() -> void:
	options.effort = ""

	var payload := provider.build_request(chat, options).payload

	assert_false(payload.has("thinking"))
	assert_false(payload.has("output_config"))


func test_build_request_known_model_with_effort_option() -> void:
	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["low", "high"]}]})

	options.effort = ""
	var payload := provider.build_request(chat, options).payload
	assert_eq(payload["thinking"], {type = "adaptive"})
	assert_false(payload.has("output_config"), "the model's default effort is left alone")

	options.effort = "high"
	payload = provider.build_request(chat, options).payload
	assert_eq(payload["output_config"], {effort = "high"})

	options.effort = "xhigh"
	payload = provider.build_request(chat, options).payload
	assert_eq(payload["thinking"], {type = "adaptive"})
	assert_false(payload.has("output_config"), "an effort the model doesn't take is not sent")


func test_build_request_known_model_without_effort_option_sends_nothing() -> void:
	options.model_info = ModelInfo.new({reasoning_options = [{type = "budget_tokens", min = 1024}]})
	options.effort = "high"

	var payload := provider.build_request(chat, options).payload

	assert_false(payload.has("thinking"))
	assert_false(payload.has("output_config"))


func test_build_request_budget_tokens() -> void:
	options.model_info = ModelInfo.new({reasoning_options = [{type = "budget_tokens", min = 1024}]})
	options.max_tokens = 16000

	options.budget_tokens = 500
	assert_eq(provider.build_request(chat, options).payload["thinking"], {type = "enabled", budget_tokens = 1024},
		"raised to the model's minimum")

	options.budget_tokens = 20000
	assert_eq(provider.build_request(chat, options).payload["thinking"], {type = "enabled", budget_tokens = 15999},
		"kept below max_tokens")

	options.model_info = null
	options.budget_tokens = 2048
	var payload := provider.build_request(chat, options).payload
	assert_eq(payload["thinking"], {type = "enabled", budget_tokens = 2048})
	assert_false(payload.has("output_config"), "a fixed budget replaces adaptive thinking and effort")

	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["high"]}]})
	payload = provider.build_request(chat, options).payload
	assert_eq(payload["thinking"], {type = "adaptive"}, "a model without the budget option ignores it")


func test_build_request_thinking_disabled() -> void:
	options.thinking = false

	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["high"]}, {type = "toggle"}]})
	var payload := provider.build_request(chat, options).payload
	assert_eq(payload["thinking"], {type = "disabled"})
	assert_false(payload.has("output_config"))

	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["high"]}]})
	payload = provider.build_request(chat, options).payload
	assert_eq(payload["thinking"], {type = "adaptive"}, "a model that can't switch thinking off keeps its default")

	options.model_info = null
	assert_eq(provider.build_request(chat, options).payload["thinking"], {type = "disabled"})


func test_build_request_maps_tools() -> void:
	var schema := {type = "object", properties = {path = {type = "string"}}}
	options.tools = [
		ToolManager.CallbackTool.new("save_scene", "Save Scene", "Saves the scene.", func (_input): return null, schema),
		ToolManager.CallbackTool.new("bare", "Bare", "", func (_input): return null, {}),
	]

	var payload := provider.build_request(chat, options).payload

	assert_eq(payload["tools"], [
		{name = "save_scene", description = "Saves the scene.", input_schema = schema},
		{name = "bare"},
	])


func test_parse_response_message() -> void:
	var response := provider.parse_response(200, {
		type = "message",
		role = "assistant",
		stop_reason = "tool_use",
		content = [
			{type = "thinking", thinking = "", signature = "sig"},
			{type = "text", text = "Saving."},
			{type = "tool_use", id = "toolu_1", name = "save_scene", input = {path = "a"}},
			{type = "server_tool_use", id = "srv_1", name = "web_search", input = {}},
			"not a block",
		],
		usage = {input_tokens = 10, output_tokens = 20, cache_creation_input_tokens = 3, cache_read_input_tokens = 4, service_tier = "standard"},
	})

	assert_true(response.is_success())
	assert_eq(response.stop_reason, Provider.StopReason.TOOL_USE)
	assert_eq(response.usage.to_dict(), {input_tokens = 10, output_tokens = 20, cache_creation_input_tokens = 3, cache_read_input_tokens = 4})
	assert_eq(response.raw["stop_reason"], "tool_use")

	var msg := response.message
	assert_eq(msg.role, Chat.Role.ASSISTANT)
	assert_eq(msg.content.size(), 4)
	assert_true(msg.content[0] is Chat.ThinkingContent)
	assert_eq(msg.content[0].signature, "sig")
	assert_true(msg.content[1] is Chat.TextContent)
	assert_true(msg.content[2] is Chat.ToolUseContent)
	assert_eq(msg.content[2].input, {path = "a"})
	assert_true(msg.content[3] is Chat.UnknownContent)
	assert_eq(msg.content[3].data["type"], "server_tool_use")


func test_parse_response_stop_reasons() -> void:
	var expected := {
		end_turn = Provider.StopReason.END_TURN,
		stop_sequence = Provider.StopReason.END_TURN,
		tool_use = Provider.StopReason.TOOL_USE,
		pause_turn = Provider.StopReason.PAUSE_TURN,
		max_tokens = Provider.StopReason.MAX_TOKENS,
		refusal = Provider.StopReason.REFUSAL,
		something_new = Provider.StopReason.UNKNOWN,
	}
	for name in expected:
		var response := provider.parse_response(200, {type = "message", stop_reason = name, content = []})
		assert_eq(response.stop_reason, expected[name], name)

	assert_eq(provider.parse_response(200, {type = "message", content = []}).stop_reason, Provider.StopReason.UNKNOWN)


func test_parse_response_without_usage() -> void:
	var response := provider.parse_response(200, {type = "message", stop_reason = "end_turn", content = []})

	assert_eq(response.usage.to_dict(), Provider.Usage.new().to_dict())


func test_parse_response_error_body() -> void:
	var response := provider.parse_response(400, {
		type = "error",
		error = {type = "invalid_request_error", message = "max_tokens: too big"},
	})

	assert_true(response.is_error())
	assert_eq(response.get_error().type, "invalid_request_error")
	assert_eq(response.get_error().message, "max_tokens: too big")
	assert_null(response.message)


func test_parse_response_non_2xx_without_an_error_body() -> void:
	var response := provider.parse_response(502, {message = "Bad Gateway"})

	assert_true(response.is_error())
	assert_eq(response.get_error().type, "unknown_error")


func test_parse_response_of_an_unknown_type_has_no_message() -> void:
	var response := provider.parse_response(200, {type = "weird", stop_reason = "pause_turn"})

	assert_true(response.is_success())
	assert_null(response.message)
	assert_eq(response.stop_reason, Provider.StopReason.PAUSE_TURN)


func test_get_rejected_option() -> void:
	var cases := {
		"thinking.type.adaptive: Extra inputs are not permitted": AnthropicProvider.OPTION_ADAPTIVE_THINKING,
		"Adaptive thinking is not supported by this model.": AnthropicProvider.OPTION_ADAPTIVE_THINKING,
		"The effort parameter is not supported by this model.": AnthropicProvider.OPTION_EFFORT,
		"max_tokens must be greater than thinking.budget_tokens": "",
		"Overloaded": "",
	}
	for message in cases:
		assert_eq(provider.get_rejected_option(Provider.ResponseError.new("invalid_request_error", message)), cases[message], message)


func test_build_request_omits_dropped_reasoning_options() -> void:
	options.model_info = ModelInfo.new({reasoning_options = [{type = "effort", values = ["high"]}]})

	options.dropped_options = [AnthropicProvider.OPTION_ADAPTIVE_THINKING]
	var payload := provider.build_request(chat, options).payload
	assert_false(payload.has("thinking"))
	assert_eq(payload["output_config"], {effort = "high"}, "effort still works without adaptive thinking")

	options.dropped_options = [AnthropicProvider.OPTION_ADAPTIVE_THINKING, AnthropicProvider.OPTION_EFFORT]
	payload = provider.build_request(chat, options).payload
	assert_false(payload.has("thinking"))
	assert_false(payload.has("output_config"))


func test_stop_reason_name() -> void:
	assert_eq(Provider.stop_reason_name(Provider.StopReason.END_TURN), "end_turn")
	assert_eq(Provider.stop_reason_name(Provider.StopReason.MAX_TOKENS), "max_tokens")
	assert_eq(Provider.stop_reason_name(Provider.StopReason.UNKNOWN), "unknown")
