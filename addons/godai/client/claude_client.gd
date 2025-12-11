extends Node

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")

const ANTHROPIC_BASE_URL = "https://api.anthropic.com/v1/"
const ANTHROPIC_VERSION = "2023-06-01"
const DEFAULT_CLAUDE_MODEL = "claude-sonnet-4-5"
const DEFAULT_MAX_TOKENS = 1024

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

	## Emitted when any response is received. May be emitted multiple times.
	signal response_received(response: Response)

	## Emitted when the final response is received. The `response_received`
	## signal will have been emitted 1 or more times before this one.
	signal completed(response: Response)

	func _init(p_chat: Chat) -> void:
		chat = p_chat

	func resolve(p_response: Response) -> void:
		completed.emit(p_response)


var api_key: String
var tools: ToolManager


func _init() -> void:
	pass


func submit_chat(p_chat: Chat, p_max_tokens: int = DEFAULT_MAX_TOKENS, p_model: String = DEFAULT_CLAUDE_MODEL) -> Request:
	var req := Request.new(p_chat)
	req.model = p_model
	req.max_tokens = p_max_tokens
	_submit_request(req)
	return req


func _submit_request(p_request: Request) -> void:
	var data: Dictionary = p_request.chat.to_dict()
	data['model'] = p_request.model
	data['max_tokens'] = p_request.max_tokens

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

		# We're going to make a follow-up request!
		if data['stop_reason'] in ["tool_use", "pause_turn"]:
			complete = false

		var resp_type: String = data.get("type", "")
		if resp_type == "message":
			msg = Message.new(data.get("role", "assistant"), data.get("content", []))

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

					var tool_result: ToolManager.ToolResult = tool_obj.execute(tool_input)
					if not tool_result.is_done():
						await tool_result.completed

					tool_results.push_back(MessageContent.from_dict({
						type = "tool_result",
						tool_use_id = tool_id,
						content = tool_result.get_content_as_string(),
					}))

			if tool_results.size() > 0:
				req.chat.add_message(Message.new("user", tool_results))

			# Submit the chat again.
			_submit_request(req)

	if complete:
		req.resolve(resp)
