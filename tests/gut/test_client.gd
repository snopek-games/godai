extends GutTest

const Chat = preload("res://addons/godai/chat/chat.gd")
const ChatClient = preload("res://addons/godai/chat/client.gd")
const Provider = preload("res://addons/godai/chat/provider.gd")
const AnthropicProvider = preload("res://addons/godai/chat/provider/anthropic.gd")
const OpenAIChatCompletionsProvider = preload("res://addons/godai/chat/provider/openai_chat_completions.gd")
const ModelInfo = preload("res://addons/godai/chat/model_info.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")

const CANCELLED_TOOL_RESULT = {
	type = "tool_result",
	tool_use_id = "toolu_2",
	content = "This tool was not run, because the user cancelled the request.",
	is_error = true,
}


## Captures outgoing requests rather than hitting the real API. Tests feed
## responses back in through `_on_request_completed()`.
class StubClient extends ChatClient:
	var submitted_payloads: Array[Dictionary]

	func _do_http_request(p_request: Request, p_web_request: Provider.WebRequest) -> void:
		submitted_payloads.push_back(p_web_request.payload)
		var http_request := HTTPRequest.new()
		add_child(http_request)
		p_request._http_request = http_request


var client: StubClient
var chat: Chat
var responses: Array[Provider.Response]


func before_each() -> void:
	client = StubClient.new()
	client.provider = AnthropicProvider.new(AnthropicProvider.DEFAULT_URL, "test-key", "claude-test")
	add_child_autoqfree(client)

	chat = Chat.new()
	chat.add_message(Chat.Message.new(Chat.Role.USER, "Hello"))
	responses = []


func _submit() -> ChatClient.Request:
	var req := client.submit_chat(chat)
	req.completed.connect(func (p_resp): responses.push_back(p_resp))
	return req


func _respond(p_req: ChatClient.Request, p_content: Array, p_stop_reason: String) -> void:
	var data := {
		type = "message",
		role = "assistant",
		stop_reason = p_stop_reason,
		content = p_content,
	}
	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 200, PackedStringArray(),
		JSON.stringify(data).to_utf8_buffer(), p_req, p_req._http_request)


func _register_tool(p_name: String, p_callback: Callable) -> void:
	if not client.tools:
		client.tools = ToolManager.new()
	client.tools.register_tool(ToolManager.CallbackTool.new(p_name, p_name, "A test tool.", p_callback))


func test_create_provider() -> void:
	var anthropic := ChatClient.create_provider("anthropic", "http://a/", "key", "claude-test")
	assert_true(anthropic is AnthropicProvider)
	assert_eq([anthropic.url, anthropic.api_key, anthropic.model], ["http://a/", "key", "claude-test"])

	assert_true(ChatClient.create_provider("openai_chat_completions", "http://o/", "key", "gpt-test") is OpenAIChatCompletionsProvider)

	assert_null(ChatClient.create_provider("bogus", "", "", ""))
	assert_push_error("Unknown chat provider: 'bogus'")


func test_submit_without_a_provider_resolves_with_an_error() -> void:
	client.provider = null

	var req := _submit()
	assert_eq(responses.size(), 0, "resolves after the caller has had a chance to connect")
	await get_tree().process_frame

	assert_eq(responses.size(), 1)
	assert_eq(responses[0].get_error().type, "no_provider")
	assert_eq(client.submitted_payloads.size(), 0)
	assert_eq(chat.messages.size(), 1)


func test_rejected_option_is_dropped_and_the_request_retried() -> void:
	client.provider = OpenAIChatCompletionsProvider.new(OpenAIChatCompletionsProvider.DEFAULT_URL, "test-key", "gpt-test")

	var req := _submit()
	assert_eq(client.submitted_payloads[0]["max_completion_tokens"], ChatClient.DEFAULT_MAX_TOKENS)

	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 400, PackedStringArray(), JSON.stringify({
		error = {type = "invalid_request_error", message = "Unknown parameter: 'max_completion_tokens'."},
	}).to_utf8_buffer(), req, req._http_request)

	assert_eq(responses.size(), 0, "the request is retried rather than resolved")
	assert_engine_error("doesn't support max_completion_tokens", "the dropped option is reported")
	assert_eq(client.submitted_payloads.size(), 2)
	assert_false(client.submitted_payloads[1].has("max_completion_tokens"))
	assert_eq(client.submitted_payloads[1]["max_tokens"], ChatClient.DEFAULT_MAX_TOKENS)
	assert_eq(client.dropped_options(), [OpenAIChatCompletionsProvider.OPTION_MAX_COMPLETION_TOKENS])

	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 200, PackedStringArray(), JSON.stringify({
		choices = [{finish_reason = "stop", message = {role = "assistant", content = "Hi!"}}],
	}).to_utf8_buffer(), req, req._http_request)
	assert_eq(responses.size(), 1)
	assert_true(responses[0].is_success())


func test_dropped_options_are_kept_per_request_settings() -> void:
	client.provider = OpenAIChatCompletionsProvider.new(OpenAIChatCompletionsProvider.DEFAULT_URL, "test-key", "gpt-test")
	var req := _submit()
	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 400, PackedStringArray(), JSON.stringify({
		error = {type = "invalid_request_error", message = "Unknown parameter: 'max_completion_tokens'."},
	}).to_utf8_buffer(), req, req._http_request)
	assert_engine_error("doesn't support max_completion_tokens")
	assert_eq(client.dropped_options(), [OpenAIChatCompletionsProvider.OPTION_MAX_COMPLETION_TOKENS])

	client.provider = OpenAIChatCompletionsProvider.new(OpenAIChatCompletionsProvider.DEFAULT_URL, "other-key", "gpt-test")
	assert_eq(client.dropped_options(), [OpenAIChatCompletionsProvider.OPTION_MAX_COMPLETION_TOKENS], "a new key alone changes nothing")

	client.provider = OpenAIChatCompletionsProvider.new(OpenAIChatCompletionsProvider.DEFAULT_URL, "test-key", "gpt-other")
	assert_eq(client.dropped_options(), [], "another model starts fresh")
	_submit()
	assert_true(client.submitted_payloads[2].has("max_completion_tokens"))

	client.provider = OpenAIChatCompletionsProvider.new("http://localhost:1234/v1/", "test-key", "gpt-test")
	assert_eq(client.dropped_options(), [], "another URL starts fresh")

	client.provider = OpenAIChatCompletionsProvider.new(OpenAIChatCompletionsProvider.DEFAULT_URL, "test-key", "gpt-test")
	client.effort = "low"
	assert_eq(client.dropped_options(), [], "another reasoning setting starts fresh")

	client.effort = ""
	assert_eq(client.dropped_options(), [OpenAIChatCompletionsProvider.OPTION_MAX_COMPLETION_TOKENS], "the original settings remember what was learned")


func test_max_tokens_is_capped_by_the_model_output_limit() -> void:
	client.model_info = ModelInfo.new({limit = {output = 4000}})

	_submit()

	assert_eq(client.submitted_payloads[0]["max_tokens"], 4000)


func test_reasoning_settings_reach_the_provider() -> void:
	client.effort = "low"
	client.thinking = false
	client.budget_tokens = 0

	_submit()

	assert_eq(client.submitted_payloads[0]["thinking"], {type = "disabled"}, "thinking off wins for an unknown model")


func test_truncated_turn_resolves_with_an_error_and_drops_its_tool_use() -> void:
	var req := _submit()
	_respond(req, [
		{type = "text", text = "Let me save"},
		{type = "tool_use", id = "toolu_1", name = "save_scene", input = {}},
	], "max_tokens")

	assert_eq(responses.size(), 1)
	assert_eq(responses[0].get_error().type, "max_tokens")
	assert_eq(client.submitted_payloads.size(), 1, "no follow-up request")
	assert_eq(chat.messages[1].to_dict(), {
		role = "assistant",
		content = [{type = "text", text = "Let me save"}],
	})


func test_cancel_during_http_request() -> void:
	var req := _submit()

	req.cancel()
	req.cancel()

	assert_eq(responses.size(), 1, "resolves right away, and only once")
	assert_eq(responses[0].get_error().type, "cancelled")
	assert_eq(client.submitted_payloads.size(), 1, "no follow-up request")

	assert_eq(chat.messages.size(), 2)
	assert_eq(chat.messages[1].to_dict(), {
		role = "user",
		content = [{type = "text", text = ChatClient.CANCELLED_MESSAGE}],
	})


func test_cancel_while_tool_is_running() -> void:
	var slow_result := ToolManager.ToolResult.new()
	_register_tool("slow_tool", func (_input): return slow_result)
	var other_tool_ran := []
	_register_tool("other_tool", func (_input):
		other_tool_ran.push_back(true)
		return ToolManager.ToolResult.resolved("other output"))

	var req := _submit()
	_respond(req, [
		{type = "tool_use", id = "toolu_1", name = "slow_tool", input = {}},
		{type = "tool_use", id = "toolu_2", name = "other_tool", input = {}},
	], "tool_use")

	req.cancel()
	assert_eq(responses.size(), 0, "waits for the running tool")

	slow_result.resolve("slow output")

	assert_eq(responses.size(), 1)
	assert_eq(responses[0].get_error().type, "cancelled")
	assert_eq(other_tool_ran.size(), 0, "tools after the cancel never start")
	assert_eq(client.submitted_payloads.size(), 1, "no follow-up request")

	# The finished tool's real result is kept; the rest are marked cancelled.
	assert_eq(chat.messages.size(), 3)
	assert_eq(chat.messages[2].to_dict(), {
		role = "user",
		content = [
			{type = "tool_result", tool_use_id = "toolu_1", content = "slow output", is_error = false},
			CANCELLED_TOOL_RESULT,
			{type = "text", text = ChatClient.CANCELLED_MESSAGE},
		],
	})


func test_second_cancel_stops_waiting_for_the_running_tool() -> void:
	var slow_result := ToolManager.ToolResult.new()
	_register_tool("slow_tool", func (_input): return slow_result)
	var other_tool_ran := []
	_register_tool("other_tool", func (_input):
		other_tool_ran.push_back(true)
		return ToolManager.ToolResult.resolved("other output"))

	var req := _submit()
	_respond(req, [
		{type = "tool_use", id = "toolu_1", name = "slow_tool", input = {}},
		{type = "tool_use", id = "toolu_2", name = "other_tool", input = {}},
	], "tool_use")

	req.cancel()
	assert_eq(responses.size(), 0, "first cancel waits for the running tool")

	req.cancel()
	assert_eq(responses.size(), 1, "second cancel stops waiting")
	assert_eq(responses[0].get_error().type, "cancelled")

	assert_eq(chat.messages.size(), 3)
	assert_eq(chat.messages[2].to_dict(), {
		role = "user",
		content = [
			{type = "tool_result", tool_use_id = "toolu_1", content = "The user cancelled the request while this tool was running. It may or may not have taken effect.", is_error = true},
			CANCELLED_TOOL_RESULT,
			{type = "text", text = ChatClient.CANCELLED_MESSAGE},
		],
	})

	slow_result.resolve("late output")

	assert_eq(chat.messages.size(), 3, "the late tool result adds nothing")
	assert_eq(responses.size(), 1)
	assert_eq(other_tool_ran.size(), 0)


func test_second_cancel_releases_the_await_on_the_running_tool() -> void:
	var slow_result := ToolManager.ToolResult.new()
	_register_tool("slow_tool", func (_input): return slow_result)

	var req := _submit()
	_respond(req, [
		{type = "tool_use", id = "toolu_1", name = "slow_tool", input = {}},
	], "tool_use")

	req.cancel()
	req.cancel()

	assert_eq(responses.size(), 1)
	assert_true(slow_result.is_done(), "the awaited tool result is finished so the coroutine can exit")


func test_unknown_tool_gets_an_error_result_and_the_chat_continues() -> void:
	_register_tool("real_tool", func (_input): return ToolManager.ToolResult.resolved("ok"))

	var req := _submit()
	_respond(req, [
		{type = "tool_use", id = "toolu_1", name = "imaginary_tool", input = {}},
	], "tool_use")

	assert_eq(responses.size(), 0, "the request is not aborted")
	assert_eq(client.submitted_payloads.size(), 2, "a follow-up request tells the model about the error")
	assert_eq(chat.messages.size(), 3)
	assert_eq(chat.messages[2].to_dict(), {
		role = "user",
		content = [{type = "tool_result", tool_use_id = "toolu_1", content = "Unknown tool: imaginary_tool", is_error = true}],
	})

	_respond(req, [{type = "text", text = "Sorry about that."}], "end_turn")

	assert_eq(responses.size(), 1)
	assert_true(responses[0].is_success())


func test_mid_turn_response_with_no_content_resolves_with_an_error() -> void:
	var req := _submit()

	_respond(req, [], "tool_use")

	assert_eq(responses.size(), 1, "the request never hangs")
	assert_eq(responses[0].get_error().type, "invalid_response")
	assert_eq(client.submitted_payloads.size(), 1, "no follow-up request")
	assert_eq(chat.messages.size(), 1, "no empty assistant message lands in the chat")


func test_mid_turn_response_of_an_unknown_type_resolves_with_an_error() -> void:
	var req := _submit()

	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 200, PackedStringArray(),
		JSON.stringify({type = "weird", stop_reason = "pause_turn"}).to_utf8_buffer(), req, req._http_request)

	assert_eq(responses.size(), 1, "the request never hangs")
	assert_eq(responses[0].get_error().type, "invalid_response")


func test_non_json_response_body_resolves_with_an_error() -> void:
	var req := _submit()

	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 502, PackedStringArray(),
		"<html>Bad Gateway</html>".to_utf8_buffer(), req, req._http_request)

	assert_eq(responses.size(), 1, "the request never hangs")
	assert_eq(responses[0].get_error().type, "invalid_response")
	assert_engine_error('Condition "error != Error::OK" is true', "JSON.parse_string reports the bad body")


func test_truncated_success_body_resolves_with_an_error() -> void:
	var req := _submit()

	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 200, PackedStringArray(),
		'{"type": "mess'.to_utf8_buffer(), req, req._http_request)

	assert_eq(responses.size(), 1)
	assert_eq(responses[0].get_error().type, "invalid_response")
	assert_engine_error('Condition "error != Error::OK" is true', "JSON.parse_string reports the bad body")


func test_stale_response_after_cancel_is_ignored() -> void:
	var req := _submit()
	var http := req._http_request

	req.cancel()
	assert_eq(responses.size(), 1)
	assert_eq(chat.messages.size(), 2)

	# A request_completed emission that was already queued when the cancel ran.
	var data := {
		type = "message",
		role = "assistant",
		stop_reason = "end_turn",
		content = [{type = "text", text = "Hi!"}],
	}
	client._on_request_completed(HTTPRequest.RESULT_SUCCESS, 200, PackedStringArray(),
		JSON.stringify(data).to_utf8_buffer(), req, http)

	assert_eq(responses.size(), 1, "already-resolved request is untouched")
	assert_eq(chat.messages.size(), 2, "no message lands after the cancellation")


func test_cancel_while_waiting_for_authorization() -> void:
	var tool_ran := []
	_register_tool("other_tool", func (_input):
		tool_ran.push_back(true)
		return ToolManager.ToolResult.resolved("other output"))

	var auth_requests: Array[ToolAuth.Request] = []
	client.tool_use_authorizer = func (p_name, p_input):
		var auth := ToolAuth.Request.new(p_name, p_input)
		auth_requests.push_back(auth)
		return auth

	var req := _submit()
	_respond(req, [
		{type = "tool_use", id = "toolu_2", name = "other_tool", input = {}},
	], "tool_use")

	assert_eq(auth_requests.size(), 1)
	req.cancel()
	# The panel denies pending approvals after cancelling; the denial must
	# come out as a cancellation, not run the tool or record a refusal.
	auth_requests[0].resolve(false)

	assert_eq(responses.size(), 1)
	assert_eq(responses[0].get_error().type, "cancelled")
	assert_eq(tool_ran.size(), 0)

	assert_eq(chat.messages.size(), 3)
	assert_eq(chat.messages[2].to_dict(), {
		role = "user",
		content = [
			CANCELLED_TOOL_RESULT,
			{type = "text", text = ChatClient.CANCELLED_MESSAGE},
		],
	})


func test_request_is_freed_once_it_completes() -> void:
	var req := _submit()
	_respond(req, [{type = "text", text = "Hi!"}], "end_turn")

	var wr: WeakRef = weakref(req)
	req = null

	assert_null(wr.get_ref(), "no reference cycle keeps the request alive")


func test_cancelled_request_is_freed() -> void:
	var req := _submit()
	req.cancel()

	var wr: WeakRef = weakref(req)
	req = null

	assert_null(wr.get_ref(), "no reference cycle keeps the request alive")


func test_cancel_after_completion_does_nothing() -> void:
	var req := _submit()
	_respond(req, [{type = "text", text = "Hi!"}], "end_turn")

	assert_eq(responses.size(), 1)
	assert_true(responses[0].is_success())

	req.cancel()

	assert_eq(responses.size(), 1)
	assert_eq(chat.messages.size(), 2, "no cancellation note")
