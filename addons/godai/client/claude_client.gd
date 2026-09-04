extends Node

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")

const ANTHROPIC_BASE_URL = "https://api.anthropic.com/v1/"
const ANTHROPIC_VERSION = "2023-06-01"
const DEFAULT_CLAUDE_MODEL = "claude-sonnet-5"

## `max_tokens` caps thinking and response text together, so this needs enough
## headroom for a whole turn of adaptive thinking plus the answer.
const DEFAULT_MAX_TOKENS = 16000

## Stop reasons that end the turn without Claude having said everything it meant
## to. Mapped to something we can show the user, since the response itself
## usually looks like an ordinary (if short) reply.
const STOP_REASON_ERRORS = {
	max_tokens = "The response was cut off after reaching the 'max_tokens' limit.",
	refusal = "Claude declined to continue with this response.",
}

## Recorded in the chat when the user cancels a request, so a later continuation
## of the conversation makes sense to the model.
const CANCELLED_MESSAGE = "[The user cancelled the previous request.]"

## Request options that only some models take, and which the model name (typed
## into a setting by the user) doesn't tell us.
const OPTION_ADAPTIVE_THINKING = "adaptive thinking"
const OPTION_EFFORT = "effort"


class MessageContent extends RefCounted:
	var data: Dictionary

	func _init(p_data: Dictionary) -> void:
		data = p_data

	func get_type() -> String:
		var type = data.get("type")
		if type is String:
			return type
		return ""

	func to_dict() -> Dictionary:
		return data

	static func from_dict(p_data: Dictionary) -> MessageContent:
		if not (p_data.get("type") is String):
			push_error("MessageContent is missing type: %s" % p_data)
			return null

		return MessageContent.new(p_data)

	static func create_text(p_text: String) -> MessageContent:
		return MessageContent.new({type = "text", text = p_text})


class Message extends RefCounted:
	var role: String
	var content: Array[MessageContent]

	func _init(p_role: String, p_content) -> void:
		role = p_role

		var tmp: Array = p_content if p_content is Array else [p_content]
		for c in tmp:
			if c is String:
				content.push_back(MessageContent.create_text(c))
			elif c is Dictionary:
				var v := MessageContent.from_dict(c)
				if v:
					content.push_back(v)
			elif c is MessageContent:
				content.push_back(c)
			else:
				push_error("Invalid message content: %s", c)

	func remove_tool_use() -> void:
		var kept: Array[MessageContent]
		for c in content:
			if c.get_type() != "tool_use":
				kept.push_back(c)
		content = kept

	func to_dict() -> Dictionary:
		return {
			role = role,
			content = content.map(func (v): return v.to_dict())
		}

	static func from_dict(p_data: Dictionary) -> Message:
		if not (p_data.get('role') is String) or not p_data.has('content'):
			push_error("Message is missing role or content: %s" % p_data)
			return null
		return Message.new(p_data['role'], p_data['content'])


class Chat extends RefCounted:
	var messages: Array[Message]

	signal message_added(message: Message)

	func add_message(p_msg: Message) -> void:
		messages.push_back(p_msg)
		message_added.emit(p_msg)

	func to_dict() -> Dictionary:
		return {
			messages = messages.map(func(v): return v.to_dict())
		}

	func repair_dangling_tool_use() -> void:
		var answered := {}
		for msg in messages:
			for c in msg.content:
				if c.get_type() == "tool_result":
					answered[c.data.get("tool_use_id")] = true

		var i := 0
		while i < messages.size():
			var results: Array[MessageContent]
			for c in messages[i].content:
				if c.get_type() == "tool_use" and not answered.has(c.data.get("id")):
					results.push_back(MessageContent.from_dict({
						type = "tool_result",
						tool_use_id = c.data.get("id"),
						content = "Something went wrong and this tool didn't record its result. It may or may not have taken effect.",
						is_error = true,
					}))
			i += 1
			if results.is_empty():
				continue
			if i < messages.size() and messages[i].role == "user":
				results.append_array(messages[i].content)
				messages[i].content = results
			else:
				messages.insert(i, Message.new("user", results))
				i += 1

	func print_debug() -> void:
		print(" === CHAT:")
		for msg in messages:
			print(msg.to_dict())


class ResponseError extends RefCounted:
	var type: String
	var message: String

	func _init(p_type: String, p_message: String) -> void:
		type = p_type
		message = p_message


class Response extends RefCounted:
	var payload
	var error: ResponseError

	func _init(p_payload = null, p_error: ResponseError = null) -> void:
		payload = p_payload
		error = p_error

	func is_error() -> bool:
		return error != null

	func is_success() -> bool:
		return not is_error()

	func get_error() -> ResponseError:
		return error


class Request extends RefCounted:
	var chat: Chat

	var model: String
	var max_tokens: int
	var effort: String

	var _cancel_requested := false
	var _done := false
	var _http_request: HTTPRequest

	## Emitted when any response is received. May be emitted multiple times.
	signal response_received(response: Response)

	## Emitted when the final response is received. The `response_received`
	## signal will have been emitted 1 or more times before this one.
	signal completed(response: Response)

	signal cancel_requested
	signal cancel_forced

	func _init(p_chat: Chat) -> void:
		chat = p_chat

	func resolve(p_response: Response) -> void:
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

var base_url := ANTHROPIC_BASE_URL

var api_key: String
var model := DEFAULT_CLAUDE_MODEL
var max_tokens := DEFAULT_MAX_TOKENS
var effort := "high"

var tools: ToolManager

## Optional hook, called as `tool_use_authorizer.call(name, input)` before a tool
## runs. Returns a ToolAuth.Request. When unset, every tool runs unauthorized.
var tool_use_authorizer: Callable

# Model name -> the options it has already rejected, so we stop sending them.
var _unsupported_options: Dictionary


func _init() -> void:
	var url_env := OS.get_environment("GODAI_ANTHROPIC_BASE_URL")
	if not url_env.is_empty():
		base_url = url_env if url_env.ends_with("/") else url_env + "/"


func submit_chat(p_chat: Chat) -> Request:
	var req := Request.new(p_chat)
	req.model = model
	req.max_tokens = max_tokens
	req.effort = effort
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


func _finish_cancelled(p_request: Request, p_tool_results: Array[MessageContent] = []) -> void:
	if p_request._done:
		return
	var content: Array[MessageContent] = p_tool_results.duplicate()
	content.push_back(MessageContent.create_text(CANCELLED_MESSAGE))
	p_request.chat.add_message(Message.new("user", content))
	p_request.resolve(Response.new(null, ResponseError.new("cancelled", "The request was cancelled.")))


func _cancelled_tool_result(p_tool_id: String) -> MessageContent:
	return MessageContent.from_dict({
		type = "tool_result",
		tool_use_id = p_tool_id,
		content = "This tool was not run, because the user cancelled the request.",
		is_error = true,
	})


func _interrupted_tool_result(p_tool_id: String) -> MessageContent:
	return MessageContent.from_dict({
		type = "tool_result",
		tool_use_id = p_tool_id,
		content = "The user cancelled the request while this tool was running. It may or may not have taken effect.",
		is_error = true,
	})


func _submit_request(p_request: Request) -> void:
	if p_request.is_cancel_requested():
		_finish_cancelled(p_request)
		return

	var data: Dictionary = p_request.chat.to_dict()
	data['model'] = p_request.model
	data['max_tokens'] = p_request.max_tokens

	if not p_request.effort.is_empty():
		if _supports(p_request.model, OPTION_ADAPTIVE_THINKING):
			data['thinking'] = {type = "adaptive"}
		if _supports(p_request.model, OPTION_EFFORT):
			data['output_config'] = {effort = p_request.effort}

	if tools and tools.tools.size() > 0:
		data['tools'] = tools.tools.values().map(func (v): return v.to_dict())

	_do_http_request(p_request, HTTPClient.METHOD_POST, "messages", data)


func _do_http_request(p_request: Request, p_method: int, p_url: String, p_payload: Dictionary = {}) -> void:
	var http_request := HTTPRequest.new()
	add_child(http_request)

	p_request._http_request = http_request
	http_request.request_completed.connect(_on_request_completed.bind(p_request, http_request))

	var headers := PackedStringArray()
	headers.resize(3)
	headers[0] = 'X-API-Key: ' + api_key
	headers[1] = 'Anthropic-Version: ' + ANTHROPIC_VERSION
	headers[2] = 'Content-Type: application/json'

	var payload: String
	if p_method != HTTPClient.METHOD_GET and p_method != HTTPClient.METHOD_HEAD:
		payload = JSON.stringify(p_payload)

	http_request.request(base_url + p_url, headers, p_method, payload)


func _on_request_completed(p_result: int, p_code: int, p_headers: PackedStringArray, p_body: PackedByteArray, p_request: Request, p_http_request: HTTPRequest) -> void:
	var req := p_request
	req._http_request = null
	p_http_request.queue_free()

	if req._done:
		return

	var data = JSON.parse_string(p_body.get_string_from_utf8())

	var msg: Message
	var complete := true

	var resp := Response.new()
	if p_result != OK:
		resp.error = ResponseError.new("http_request_error", "Unable to make HTTP request")
	elif not data is Dictionary:
		resp.error = ResponseError.new("invalid_response", "Invalid JSON in the response body (HTTP code %d)" % p_code)
	elif p_code < 200 or p_code >= 300 or data.get("type") == "error":
		var error: Dictionary = data.get("error", {})
		var message: String = error.get("message", "")

		var unsupported := _get_rejected_option(message)
		if not unsupported.is_empty() and _supports(req.model, unsupported):
			_mark_unsupported(req.model, unsupported)
			_submit_request(req)
			return

		resp.error = ResponseError.new(error.get("type", "unknown_error"), message)
	else:
		resp.payload = data

		var stop_reason: String = data.get("stop_reason", "")

		# We're going to make a follow-up request!
		if stop_reason in ["tool_use", "pause_turn"]:
			complete = false
		elif stop_reason in STOP_REASON_ERRORS:
			resp.error = ResponseError.new(stop_reason, STOP_REASON_ERRORS[stop_reason])

		var resp_type: String = data.get("type", "")
		if resp_type == "message":
			msg = Message.new(data.get("role", "assistant"), data.get("content", []))

			# If the turn was cut short, we need to remove any tool_use, because
			# we'll never respond to them, which will lead to any continuation of
			# the conversation to hit an API error.
			if stop_reason in STOP_REASON_ERRORS:
				msg.remove_tool_use()

		else:
			print("Unable to handle response type '%s': %s" % [resp_type, data])

	req.response_received.emit(resp)

	# Process the message into the chat.
	if msg and msg.content.size() > 0:
		# Add the message to the chat.
		req.chat.add_message(msg)

		if not complete:
			# Process any tools.
			var tool_results: Array[MessageContent]
			var tool_uses: Array[MessageContent]
			for content in msg.content:
				if content.get_type() == "tool_use":
					tool_uses.push_back(content)

			for i in tool_uses.size():
				var tool_id: String = tool_uses[i].data["id"]
				var tool_name: String = tool_uses[i].data["name"]
				var tool_input = tool_uses[i].data["input"]

				if req.is_cancel_requested():
					tool_results.push_back(_cancelled_tool_result(tool_id))
					continue

				var tool_obj: ToolManager.Tool = tools.tools.get(tool_name)
				if not tool_obj:
					tool_results.push_back(MessageContent.from_dict({
						type = "tool_result",
						tool_use_id = tool_id,
						content = "Unknown tool: %s" % tool_name,
						is_error = true,
					}))
					continue

				var allowed := await _authorize_tool_use(tool_name, tool_input)
				if req._done:
					return
				if req.is_cancel_requested():
					tool_results.push_back(_cancelled_tool_result(tool_id))
					continue

				var tool_result: ToolManager.ToolResult
				if allowed:
					tool_result = tool_obj.execute(tool_input)
					if not tool_result.is_done():
						var on_force := func ():
							var results := tool_results.duplicate()
							results.push_back(_interrupted_tool_result(tool_id))
							for j in range(i + 1, tool_uses.size()):
								results.push_back(_cancelled_tool_result(tool_uses[j].data["id"]))
							_finish_cancelled(req, results)
							tool_result.reject("The user cancelled the request.")

						req.cancel_forced.connect(on_force)
						await tool_result.completed
						req.cancel_forced.disconnect(on_force)

						if req._done:
							return
				else:
					tool_result = ToolAuth.denied_result(tool_name)

				tool_results.push_back(MessageContent.from_dict({
					type = "tool_result",
					tool_use_id = tool_id,
					content = tool_result.get_content_as_string(),
					is_error = tool_result.is_error(),
				}))
				tool_use_completed.emit(tool_name, tool_result)

			if req.is_cancel_requested():
				_finish_cancelled(req, tool_results)
				return

			if tool_results.size() > 0:
				req.chat.add_message(Message.new("user", tool_results))

			# Submit the chat again.
			_submit_request(req)

	elif not complete:
		complete = true
		resp.error = ResponseError.new("invalid_response", "The response ended mid-turn without any content to continue from.")

	if complete:
		req.resolve(resp)


## The API says which option it rejected in the message; there's no field or
## code for it.
func _get_rejected_option(p_message: String) -> String:
	var message := p_message.to_lower()

	if message.contains("thinking.type.adaptive") \
		or (message.contains("adaptive thinking") and message.contains("not supported")):
		return OPTION_ADAPTIVE_THINKING

	if message.contains("effort parameter") and message.contains("not support"):
		return OPTION_EFFORT

	return ""


func dropped_options(p_model: String) -> Array:
	return _unsupported_options.get(p_model, {}).keys()


func _supports(p_model: String, p_option: String) -> bool:
	return not _unsupported_options.get(p_model, {}).has(p_option)


func _mark_unsupported(p_model: String, p_option: String) -> void:
	push_warning("Model '%s' doesn't support %s: retrying without it." % [p_model, p_option])
	if not _unsupported_options.has(p_model):
		_unsupported_options[p_model] = {}
	_unsupported_options[p_model][p_option] = true


func _authorize_tool_use(p_name: String, p_input) -> bool:
	if not tool_use_authorizer.is_valid():
		return true

	var request = tool_use_authorizer.call(p_name, p_input)
	if not request.is_done():
		await request.completed
	return request.allowed
