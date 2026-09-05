extends Node

const Chat = preload("res://addons/godai/chat/chat.gd")
const ModelInfo = preload("res://addons/godai/chat/model_info.gd")
const Provider = preload("res://addons/godai/chat/provider.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")

const PROVIDERS := {
	anthropic = preload("res://addons/godai/chat/provider/anthropic.gd"),
	openai_chat_completions = preload("res://addons/godai/chat/provider/openai_chat_completions.gd"),
}

## `max_tokens` caps thinking and response text together, so this needs enough
## headroom for a whole turn of adaptive thinking plus the answer.
const DEFAULT_MAX_TOKENS = 16000

## Stop reasons that end the turn without the model having said everything it
## meant to. Mapped to something we can show the user, since the response itself
## usually looks like an ordinary (if short) reply.
const STOP_REASON_ERRORS = {
	Provider.StopReason.MAX_TOKENS: "The response was cut off after reaching the 'max_tokens' limit.",
	Provider.StopReason.REFUSAL: "The model declined to continue with this response.",
}

## Recorded in the chat when the user cancels a request, so a later continuation
## of the conversation makes sense to the model.
const CANCELLED_MESSAGE = "[The user cancelled the previous request.]"


class Request extends RefCounted:
	var chat: Chat

	var provider: Provider
	var max_tokens: int
	var effort: String
	var thinking: bool
	var budget_tokens: int
	var model_info: ModelInfo

	var _cancel_requested := false
	var _done := false
	var _http_request: HTTPRequest

	## Emitted when any response is received. May be emitted multiple times.
	signal response_received(response: Provider.Response)

	## Emitted when the final response is received. The `response_received`
	## signal will have been emitted 1 or more times before this one.
	signal completed(response: Provider.Response)

	signal cancel_requested
	signal cancel_forced

	func _init(p_chat: Chat) -> void:
		chat = p_chat

	func resolve(p_response: Provider.Response) -> void:
		if _done:
			return
		_done = true
		completed.emit(p_response)

	## Aborts an in-flight HTTP request right away, but lets a tool that is
	## already running finish and record its result before `completed` fires.
	## A second cancel() gives up on the running tool and finishes immediately,
	## recording it as cancelled.
	func cancel() -> void:
		if _done:
			return
		if _cancel_requested:
			cancel_forced.emit()
			return
		_cancel_requested = true
		cancel_requested.emit()

	func is_cancel_requested() -> bool:
		return _cancel_requested


signal tool_use_completed(p_name: String, p_result: ToolManager.ToolResult)

var provider: Provider
var max_tokens := DEFAULT_MAX_TOKENS
var effort := ""
var thinking := true
var budget_tokens := 0
var model_info: ModelInfo

var tools: ToolManager

## Optional hook, called as `tool_use_authorizer.call(name, input)` before a tool
## runs. Returns a ToolAuth.Request. When unset, every tool runs unauthorized.
var tool_use_authorizer: Callable

# Request key -> the options the API rejected.
var _unsupported_options: Dictionary


static func create_provider(p_name: String, p_url: String, p_api_key: String, p_model: String) -> Provider:
	var script: GDScript = PROVIDERS.get(p_name)
	if not script:
		push_error("Unknown chat provider: '%s'" % p_name)
		return null
	return script.new(p_url, p_api_key, p_model)


func submit_chat(p_chat: Chat) -> Request:
	var req := Request.new(p_chat)
	req.provider = provider
	req.max_tokens = max_tokens
	req.effort = effort
	req.thinking = thinking
	req.budget_tokens = budget_tokens
	req.model_info = model_info
	# Binding req directly would store a strong self-reference on its own
	# signal, so the Request (and its Chat) would never be freed.
	req.cancel_requested.connect(_on_request_cancel_requested.bind(weakref(req)))
	_submit_request(req)
	return req


func _on_request_cancel_requested(p_request_wr: WeakRef) -> void:
	var req: Request = p_request_wr.get_ref()
	if req and req._http_request:
		req._http_request.cancel_request()
		req._http_request.queue_free()
		req._http_request = null
		_finish_cancelled(req)


func _finish_cancelled(p_request: Request, p_tool_results: Array[Chat.MessageContent] = []) -> void:
	if p_request._done:
		return
	var content: Array[Chat.MessageContent] = p_tool_results.duplicate()
	content.push_back(Chat.TextContent.new(CANCELLED_MESSAGE))
	p_request.chat.add_message(Chat.Message.new(Chat.Role.USER, content))
	p_request.resolve(Provider.Response.failed("cancelled", "The request was cancelled."))


func _cancelled_tool_result(p_tool_id: String) -> Chat.ToolResultContent:
	return Chat.ToolResultContent.new(p_tool_id, "This tool was not run, because the user cancelled the request.", true)


func _interrupted_tool_result(p_tool_id: String) -> Chat.ToolResultContent:
	return Chat.ToolResultContent.new(p_tool_id, "The user cancelled the request while this tool was running. It may or may not have taken effect.", true)


func _submit_request(p_request: Request) -> void:
	if p_request.is_cancel_requested():
		_finish_cancelled(p_request)
		return

	if p_request.provider == null:
		p_request.resolve.call_deferred(Provider.Response.failed("no_provider", "No chat provider is configured. Check the Godai API settings in Editor Settings."))
		return

	var options := Provider.RequestOptions.new()
	options.max_tokens = p_request.max_tokens
	if p_request.model_info and p_request.model_info.output_limit > 0:
		options.max_tokens = mini(options.max_tokens, p_request.model_info.output_limit)
	options.effort = p_request.effort
	options.thinking = p_request.thinking
	options.budget_tokens = p_request.budget_tokens
	options.model_info = p_request.model_info
	if tools:
		options.tools.assign(tools.tools.values())
	options.dropped_options.assign(_unsupported_options.get(_request_key(p_request), {}).keys())

	_do_http_request(p_request, p_request.provider.build_request(p_request.chat, options))


func _do_http_request(p_request: Request, p_web_request: Provider.WebRequest) -> void:
	var http_request := HTTPRequest.new()
	add_child(http_request)

	p_request._http_request = http_request
	http_request.request_completed.connect(_on_request_completed.bind(p_request, http_request))

	http_request.request(p_web_request.url, p_web_request.headers, HTTPClient.METHOD_POST, JSON.stringify(p_web_request.payload))


func _on_request_completed(p_result: int, p_code: int, p_headers: PackedStringArray, p_body: PackedByteArray, p_request: Request, p_http_request: HTTPRequest) -> void:
	var req := p_request
	req._http_request = null
	p_http_request.queue_free()

	if req._done:
		return

	var data = JSON.parse_string(p_body.get_string_from_utf8())

	var resp: Provider.Response
	if p_result != OK:
		resp = Provider.Response.failed("http_request_error", "Unable to make HTTP request")
	elif not data is Dictionary:
		resp = Provider.Response.failed("invalid_response", "Invalid JSON in the response body (HTTP code %d)" % p_code)
	else:
		resp = req.provider.parse_response(p_code, data)
		if resp.is_error():
			var unsupported := req.provider.get_rejected_option(resp.error)
			var key := _request_key(req)
			if not unsupported.is_empty() and not _unsupported_options.get(key, {}).has(unsupported):
				push_warning("Model '%s' doesn't support %s: retrying without it." % [req.provider.model, unsupported])
				if not _unsupported_options.has(key):
					_unsupported_options[key] = {}
				_unsupported_options[key][unsupported] = true
				_submit_request(req)
				return

	var msg := resp.message
	var complete := true

	if resp.is_success():
		# We're going to make a follow-up request!
		if resp.stop_reason in [Provider.StopReason.TOOL_USE, Provider.StopReason.PAUSE_TURN]:
			complete = false
		elif STOP_REASON_ERRORS.has(resp.stop_reason):
			resp.error = Provider.ResponseError.new(Provider.stop_reason_name(resp.stop_reason), STOP_REASON_ERRORS[resp.stop_reason])

			# If the turn was cut short, we need to remove any tool_use, because
			# we'll never respond to them, which will lead to any continuation of
			# the conversation to hit an API error.
			if msg:
				msg.remove_tool_use()

	req.response_received.emit(resp)

	# Process the message into the chat.
	if msg and msg.content.size() > 0:
		# Add the message to the chat.
		req.chat.add_message(msg)

		if not complete:
			# Process any tools.
			var tool_results: Array[Chat.MessageContent]
			var tool_uses: Array[Chat.ToolUseContent]
			for content in msg.content:
				if content is Chat.ToolUseContent:
					tool_uses.push_back(content)

			for i in tool_uses.size():
				var tool_use := tool_uses[i]

				if req.is_cancel_requested():
					tool_results.push_back(_cancelled_tool_result(tool_use.id))
					continue

				var tool_obj: ToolManager.Tool = tools.tools.get(tool_use.name) if tools else null
				if not tool_obj:
					tool_results.push_back(Chat.ToolResultContent.new(tool_use.id, "Unknown tool: %s" % tool_use.name, true))
					continue

				var allowed := await _authorize_tool_use(tool_use.name, tool_use.input)
				if req._done:
					return
				if req.is_cancel_requested():
					tool_results.push_back(_cancelled_tool_result(tool_use.id))
					continue

				var tool_result: ToolManager.ToolResult
				if allowed:
					tool_result = tool_obj.execute(tool_use.input)
					if not tool_result.is_done():
						var on_force := func ():
							var results := tool_results.duplicate()
							results.push_back(_interrupted_tool_result(tool_use.id))
							for j in range(i + 1, tool_uses.size()):
								results.push_back(_cancelled_tool_result(tool_uses[j].id))
							_finish_cancelled(req, results)
							tool_result.reject("The user cancelled the request.")

						req.cancel_forced.connect(on_force)
						await tool_result.completed
						req.cancel_forced.disconnect(on_force)

						if req._done:
							return
				else:
					tool_result = ToolAuth.denied_result(tool_use.name)

				tool_results.push_back(Chat.ToolResultContent.new(tool_use.id, tool_result.get_content_as_string(), tool_result.is_error()))
				tool_use_completed.emit(tool_use.name, tool_result)

			if req.is_cancel_requested():
				_finish_cancelled(req, tool_results)
				return

			if tool_results.size() > 0:
				req.chat.add_message(Chat.Message.new(Chat.Role.USER, tool_results))

			# Submit the chat again.
			_submit_request(req)

	elif not complete:
		complete = true
		resp.error = Provider.ResponseError.new("invalid_response", "The response ended mid-turn without any content to continue from.")

	if complete:
		req.resolve(resp)


func dropped_options() -> Array:
	if provider == null:
		return []
	return _unsupported_options.get(_settings_key(provider, effort, thinking, budget_tokens), {}).keys()


static func _request_key(p_request: Request) -> String:
	return _settings_key(p_request.provider, p_request.effort, p_request.thinking, p_request.budget_tokens)


static func _settings_key(p_provider: Provider, p_effort: String, p_thinking: bool, p_budget_tokens: int) -> String:
	var script: Script = p_provider.get_script()
	return JSON.stringify([script.resource_path, p_provider.url, p_provider.model, p_effort, p_thinking, p_budget_tokens])


func _authorize_tool_use(p_name: String, p_input) -> bool:
	if not tool_use_authorizer.is_valid():
		return true

	var request = tool_use_authorizer.call(p_name, p_input)
	if not request.is_done():
		await request.completed
	return request.allowed
