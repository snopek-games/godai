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

const HTTP_REQUEST_META = 'godai_request'


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
		if not p_data.has("type") and p_data["type"] is String:
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
				content.push_back(MessageContent.create_text(p_content))
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

	var _cancelled := false
	var _done := false

	## Emitted when any response is received. May be emitted multiple times.
	signal response_received(response: Response)

	## Emitted when the final response is received. The `response_received`
	## signal will have been emitted 1 or more times before this one.
	signal completed(response: Response)

	func _init(p_chat: Chat) -> void:
		chat = p_chat

	func resolve(p_response: Response) -> void:
		if _done:
			return
		_done = true
		completed.emit(p_response)

	## Gives up on the request: no more tools run and no follow-up is submitted.
	## Resolves right away so whoever is awaiting it isn't left hanging - anything
	## still in flight is thrown away when it arrives.
	func cancel() -> void:
		if _cancelled:
			return
		_cancelled = true
		resolve(Response.new(null, ResponseError.new("cancelled", "The request was cancelled.")))

	func is_cancelled() -> bool:
		return _cancelled


var api_key: String
var model := DEFAULT_CLAUDE_MODEL
var max_tokens := DEFAULT_MAX_TOKENS
var effort := "high"

var tools: ToolManager

## Optional hook, called as `tool_use_authorizer.call(name, input)` before a tool
## runs. Returns a ToolAuth.Request. When unset, every tool runs unauthorized.
var tool_use_authorizer: Callable


func _init() -> void:
	pass


func submit_chat(p_chat: Chat) -> Request:
	var req := Request.new(p_chat)
	req.model = model
	req.max_tokens = max_tokens
	req.effort = effort
	_submit_request(req)
	return req


func _submit_request(p_request: Request) -> void:
	if p_request.is_cancelled():
		return

	var data: Dictionary = p_request.chat.to_dict()
	data['model'] = p_request.model
	data['max_tokens'] = p_request.max_tokens

	if not p_request.effort.is_empty():
		data['thinking'] = {type = "adaptive"}
		data['output_config'] = {effort = p_request.effort}

	if tools and tools.tools.size() > 0:
		data['tools'] = tools.tools.values().map(func (v): return v.to_dict())

	_do_http_request(p_request, HTTPClient.METHOD_POST, "messages", data)


func _do_http_request(p_request: Request, p_method: int, p_url: String, p_payload: Dictionary = {}) -> void:
	var http_request := HTTPRequest.new()
	add_child(http_request)

	http_request.set_meta(HTTP_REQUEST_META, p_request)
	http_request.request_completed.connect(_on_request_completed.bind(http_request))

	var headers := PackedStringArray()
	headers.resize(3)
	headers[0] = 'X-API-Key: ' + api_key
	headers[1] = 'Anthropic-Version: ' + ANTHROPIC_VERSION
	headers[2] = 'Content-Type: application/json'

	var payload: String
	if p_method != HTTPClient.METHOD_GET and p_method != HTTPClient.METHOD_HEAD:
		payload = JSON.stringify(p_payload)

	http_request.request(ANTHROPIC_BASE_URL + p_url, headers, p_method, payload)


func _on_request_completed(p_result: int, p_code: int, p_headers: PackedStringArray, p_body: PackedByteArray, p_http_request: HTTPRequest) -> void:
	var req: Request = p_http_request.get_meta(HTTP_REQUEST_META)
	remove_child(p_http_request)

	if req.is_cancelled():
		return

	var data = JSON.parse_string(p_body.get_string_from_utf8())

	var msg: Message
	var complete := true

	var resp := Response.new()
	if p_result != OK:
		resp.error = ResponseError.new("http_request_error", "Unable to make HTTP request")
	elif p_code < 200 or p_code >= 300 or data.get("type") == "error":
		var error: Dictionary = data.get("error", {})
		resp.error = ResponseError.new(error.get("type", "unknown_error"), error.get("message", ""))
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
			for content in msg.content:
				var type: String = content.get_type()
				if type == "tool_use":
					var tool_id: String = content.data["id"]
					var tool_name: String = content.data["name"]
					var tool_input = content.data["input"]

					var tool_obj: ToolManager.Tool = tools.tools.get(tool_name)
					if not tool_obj:
						var new_resp = Response.new(null, ResponseError.new("internal_error", "Unknown tool: %s" % tool_name))
						req.resolve(new_resp)
						return

					var allowed := await _authorize_tool_use(tool_name, tool_input)
					if req.is_cancelled():
						return

					var tool_result: ToolManager.ToolResult
					if allowed:
						tool_result = tool_obj.execute(tool_input)
						if not tool_result.is_done():
							await tool_result.completed
							if req.is_cancelled():
								return
					else:
						tool_result = ToolAuth.denied_result(tool_name)

					tool_results.push_back(MessageContent.from_dict({
						type = "tool_result",
						tool_use_id = tool_id,
						content = tool_result.get_content_as_string(),
						is_error = tool_result.is_error(),
					}))

			if tool_results.size() > 0:
				req.chat.add_message(Message.new("user", tool_results))

			# Submit the chat again.
			_submit_request(req)

	if complete:
		req.resolve(resp)


func _authorize_tool_use(p_name: String, p_input) -> bool:
	if not tool_use_authorizer.is_valid():
		return true

	var request = tool_use_authorizer.call(p_name, p_input)
	if not request.is_done():
		await request.completed
	return request.allowed
