extends "res://addons/godai/chat/provider.gd"

const DEFAULT_URL = "https://api.openai.com/v1/"
const DEFAULT_MODEL = "gpt-5"

## Request options that only some servers and models take, and which the model
## name (typed into a setting by the user) doesn't tell us.
const OPTION_MAX_COMPLETION_TOKENS = "max_completion_tokens"
const OPTION_REASONING_EFFORT = "reasoning_effort"
const REJECTABLE_OPTIONS := [OPTION_MAX_COMPLETION_TOKENS, OPTION_REASONING_EFFORT]

const FINISH_REASONS := {
	stop = StopReason.END_TURN,
	tool_calls = StopReason.TOOL_USE,
	function_call = StopReason.TOOL_USE,
	length = StopReason.MAX_TOKENS,
	content_filter = StopReason.REFUSAL,
}

const EMPTY_PARAMETERS := {type = "object", properties = {}}


static func get_reasoning_options() -> PackedStringArray:
	return ["effort", "toggle"]


func build_request(p_chat: Chat, p_options: RequestOptions) -> WebRequest:
	var messages := []
	for msg in p_chat.messages:
		messages.append_array(_message_to_dicts(msg))

	var payload := {
		model = model,
		messages = messages,
	}

	if OPTION_MAX_COMPLETION_TOKENS in p_options.dropped_options:
		payload['max_tokens'] = p_options.max_tokens
	else:
		payload['max_completion_tokens'] = p_options.max_tokens

	var info := p_options.model_info
	if OPTION_REASONING_EFFORT in p_options.dropped_options:
		pass
	elif not p_options.thinking and (info == null or info.thinking_toggle):
		payload['reasoning_effort'] = "none"
	elif not p_options.effort.is_empty() and (info == null or info.accepts_effort(p_options.effort)):
		payload['reasoning_effort'] = p_options.effort

	if p_options.tools.size() > 0:
		payload['tools'] = p_options.tools.map(_tool_to_dict)

	var request := WebRequest.new()
	request.url = url.trim_suffix("/") + "/chat/completions"
	request.headers = PackedStringArray([
		"authorization: Bearer " + api_key,
		"content-type: application/json",
	])
	request.payload = payload
	return request


func parse_response(p_code: int, p_data: Dictionary) -> Response:
	var error = p_data.get("error")
	if p_code < 200 or p_code >= 300 or error is Dictionary:
		if not error is Dictionary:
			error = {}
		var type = error.get("type")
		if not (type is String) or type.is_empty():
			type = error.get("code", "unknown_error")
		var param = error.get("param")
		return Response.failed(str(type), str(error.get("message", "")), param if param is String else "")

	var response := Response.new()
	response.raw = p_data

	var usage = p_data.get("usage")
	if usage is Dictionary:
		var prompt_details = usage.get("prompt_tokens_details")
		var cached := int(prompt_details.get("cached_tokens", 0)) if prompt_details is Dictionary else 0
		response.usage.input_tokens = maxi(0, int(usage.get("prompt_tokens", 0)) - cached)
		response.usage.output_tokens = int(usage.get("completion_tokens", 0))
		response.usage.cache_read_input_tokens = cached

	var choices = p_data.get("choices")
	if not (choices is Array) or choices.is_empty() or not (choices[0] is Dictionary):
		print("OpenAI Chat Completions response has no choices: %s" % p_data)
		return response

	var choice: Dictionary = choices[0]
	response.stop_reason = FINISH_REASONS.get(choice.get("finish_reason"), StopReason.UNKNOWN)

	var message = choice.get("message")
	if message is Dictionary:
		response.message = _message_from_dict(message)
		# Some servers report "stop" even when they asked for tools; the calls
		# still need answering or the next request is rejected.
		if response.stop_reason != StopReason.MAX_TOKENS \
			and response.message.content.any(func (c): return c is Chat.ToolUseContent):
			response.stop_reason = StopReason.TOOL_USE

	return response


## The API names the parameter it rejected in `param`; compatible servers often
## leave that out and only word it into the message.
func get_rejected_option(p_error: ResponseError) -> String:
	var message := p_error.message.to_lower()
	if not (message.contains("unsupported") or message.contains("unrecognized") \
		or message.contains("unknown") or message.contains("not supported") or message.contains("invalid value")):
		return ""
	for option in REJECTABLE_OPTIONS:
		if option == p_error.param or (p_error.param.is_empty() and message.contains(option)):
			return option
	return ""


func _message_to_dicts(p_msg: Chat.Message) -> Array:
	var texts := PackedStringArray()
	var tool_calls := []
	var tool_messages := []

	for content in p_msg.content:
		if content is Chat.TextContent:
			texts.push_back(content.text)
		elif content is Chat.ToolUseContent:
			tool_calls.push_back({
				id = content.id,
				type = "function",
				function = {name = content.name, arguments = JSON.stringify(content.input)},
			})
		elif content is Chat.ToolResultContent:
			tool_messages.push_back({
				role = "tool",
				tool_call_id = content.tool_use_id,
				content = _tool_result_text(content),
			})

	var result := tool_messages
	if p_msg.role == Chat.Role.ASSISTANT:
		var assistant := {role = "assistant"}
		if texts.size() > 0 or tool_calls.is_empty():
			assistant['content'] = "\n\n".join(texts)
		if tool_calls.size() > 0:
			assistant['tool_calls'] = tool_calls
		result.push_back(assistant)
	elif texts.size() > 0:
		result.push_back({role = "user", content = "\n\n".join(texts)})

	return result


func _tool_result_text(p_content: Chat.ToolResultContent) -> String:
	var text: String = p_content.content if p_content.content is String else JSON.stringify(p_content.content)
	if p_content.is_error:
		return "Error: " + text
	return text


func _message_from_dict(p_message: Dictionary) -> Chat.Message:
	var content: Array[Chat.MessageContent]

	for key in ["reasoning_content", "reasoning"]:
		var reasoning = p_message.get(key)
		if reasoning is String and not reasoning.is_empty():
			content.push_back(Chat.ThinkingContent.new(reasoning))
			break

	var text = p_message.get("content")
	if text is String and not text.is_empty():
		content.push_back(Chat.TextContent.new(text))

	var tool_calls = p_message.get("tool_calls")
	if tool_calls is Array:
		for tool_call in tool_calls:
			if not tool_call is Dictionary:
				continue
			var function = tool_call.get("function")
			if not function is Dictionary:
				continue
			content.push_back(Chat.ToolUseContent.new(str(tool_call.get("id", "")), str(function.get("name", "")),
				_parse_arguments(str(function.get("arguments", "")))))

	return Chat.Message.new(Chat.Role.ASSISTANT, content)


func _parse_arguments(p_arguments: String) -> Dictionary:
	var json := JSON.new()
	if json.parse(p_arguments) == OK and json.data is Dictionary:
		return json.data
	return {}


func _tool_to_dict(p_tool: ToolManager.Tool) -> Dictionary:
	var function := {
		name = p_tool.name,
		parameters = p_tool.input_schema if p_tool.input_schema.size() > 0 else EMPTY_PARAMETERS,
	}
	if p_tool.description.length() > 0:
		function['description'] = p_tool.description
	return {type = "function", function = function}
