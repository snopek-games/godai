extends "res://addons/godai/chat/provider.gd"

const DEFAULT_URL = "https://api.anthropic.com/v1/"
const DEFAULT_MODEL = "claude-sonnet-5"
const API_VERSION = "2023-06-01"

## Request options that only some models take, and which the model name (typed
## into a setting by the user) or the catalog doesn't tell us.
const OPTION_ADAPTIVE_THINKING = "adaptive thinking"
const OPTION_EFFORT = "effort"

const STOP_REASONS := {
	end_turn = StopReason.END_TURN,
	stop_sequence = StopReason.END_TURN,
	tool_use = StopReason.TOOL_USE,
	pause_turn = StopReason.PAUSE_TURN,
	max_tokens = StopReason.MAX_TOKENS,
	refusal = StopReason.REFUSAL,
}


static func get_reasoning_options() -> PackedStringArray:
	return ["effort", "toggle", "budget_tokens"]


func build_request(p_chat: Chat, p_options: RequestOptions) -> WebRequest:
	var payload := {
		model = model,
		max_tokens = p_options.max_tokens,
		messages = p_chat.messages.map(_message_to_dict),
	}
	if not p_chat.system.is_empty():
		payload['system'] = p_chat.system

	var info := p_options.model_info
	if not p_options.thinking and (info == null or info.thinking_toggle):
		payload['thinking'] = {type = "disabled"}
	elif p_options.budget_tokens > 0 and (info == null or info.supports_budget_tokens()):
		var budget := p_options.budget_tokens
		if info:
			budget = maxi(budget, info.budget_tokens_min)
		# The API rejects a budget that doesn't leave room for the answer.
		payload['thinking'] = {type = "enabled", budget_tokens = mini(budget, p_options.max_tokens - 1)}
	elif (info and info.supports_effort()) or (info == null and not p_options.effort.is_empty()):
		if not OPTION_ADAPTIVE_THINKING in p_options.dropped_options:
			payload['thinking'] = {type = "adaptive"}
		if not p_options.effort.is_empty() and (info == null or info.accepts_effort(p_options.effort)) and not OPTION_EFFORT in p_options.dropped_options:
			payload['output_config'] = {effort = p_options.effort}

	if p_options.tools.size() > 0:
		payload['tools'] = p_options.tools.map(_tool_to_dict)

	var request := WebRequest.new()
	request.url = url.trim_suffix("/") + "/messages"
	request.headers = PackedStringArray([
		"x-api-key: " + api_key,
		"anthropic-version: " + API_VERSION,
		"content-type: application/json",
	])
	request.payload = payload
	return request


func parse_response(p_code: int, p_data: Dictionary) -> Response:
	if p_code < 200 or p_code >= 300 or p_data.get("type") == "error":
		var error = p_data.get("error")
		if not error is Dictionary:
			error = {}
		return Response.failed(str(error.get("type", "unknown_error")), str(error.get("message", "")))

	var response := Response.new()
	response.raw = p_data
	response.stop_reason = STOP_REASONS.get(p_data.get("stop_reason"), StopReason.UNKNOWN)

	var usage = p_data.get("usage")
	if usage is Dictionary:
		response.usage.input_tokens = int(usage.get("input_tokens", 0))
		response.usage.output_tokens = int(usage.get("output_tokens", 0))
		response.usage.cache_creation_input_tokens = int(usage.get("cache_creation_input_tokens", 0))
		response.usage.cache_read_input_tokens = int(usage.get("cache_read_input_tokens", 0))

	if p_data.get("type") == "message":
		response.message = _message_from_dict(p_data)
	else:
		print("Unable to handle Anthropic response type '%s': %s" % [p_data.get("type", ""), p_data])

	return response


## The API says which option it rejected in the message; there's no field or
## code for it.
func get_rejected_option(p_error: ResponseError) -> String:
	var message := p_error.message.to_lower()

	if message.contains("thinking.type.adaptive") \
		or (message.contains("adaptive thinking") and message.contains("not supported")):
		return OPTION_ADAPTIVE_THINKING

	if message.contains("effort parameter") and message.contains("not support"):
		return OPTION_EFFORT

	return ""


func _message_to_dict(p_msg: Chat.Message) -> Dictionary:
	return {
		role = Chat.ROLE_NAMES[p_msg.role],
		content = p_msg.content.map(_content_to_dict),
	}


func _content_to_dict(p_content: Chat.MessageContent) -> Dictionary:
	if p_content is Chat.TextContent:
		return {type = "text", text = p_content.text}
	if p_content is Chat.ToolUseContent:
		return {type = "tool_use", id = p_content.id, name = p_content.name, input = p_content.input}
	if p_content is Chat.ToolResultContent:
		return {type = "tool_result", tool_use_id = p_content.tool_use_id, content = p_content.content, is_error = p_content.is_error}
	if p_content is Chat.ThinkingContent:
		return {type = "thinking", thinking = p_content.thinking, signature = p_content.signature}
	return p_content.to_dict()


func _message_from_dict(p_data: Dictionary) -> Chat.Message:
	var role = Chat.ROLE_NAMES.find_key(p_data.get("role", "assistant"))
	if role == null:
		role = Chat.Role.ASSISTANT

	var content: Array[Chat.MessageContent]
	var blocks = p_data.get("content", [])
	if blocks is Array:
		for block in blocks:
			if block is Dictionary:
				content.push_back(_content_from_dict(block))

	return Chat.Message.new(role as Chat.Role, content)


func _content_from_dict(p_block: Dictionary) -> Chat.MessageContent:
	match str(p_block.get("type", "")):
		"text":
			return Chat.TextContent.new(str(p_block.get("text", "")))
		"tool_use":
			var input = p_block.get("input")
			return Chat.ToolUseContent.new(str(p_block.get("id", "")), str(p_block.get("name", "")), input if input is Dictionary else {})
		"tool_result":
			return Chat.ToolResultContent.new(str(p_block.get("tool_use_id", "")), p_block.get("content", ""), bool(p_block.get("is_error", false)))
		"thinking":
			return Chat.ThinkingContent.new(str(p_block.get("thinking", "")), str(p_block.get("signature", "")))
	return Chat.UnknownContent.new(p_block)


func _tool_to_dict(p_tool: ToolManager.Tool) -> Dictionary:
	var data := {name = p_tool.name}
	if p_tool.description.length() > 0:
		data['description'] = p_tool.description
	if p_tool.input_schema.size() > 0:
		data['input_schema'] = p_tool.input_schema
	return data
