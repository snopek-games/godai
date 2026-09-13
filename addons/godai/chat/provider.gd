@abstract
extends RefCounted

const Chat = preload("res://addons/godai/chat/chat.gd")
const ModelInfo = preload("res://addons/godai/chat/model_info.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")

const FULL_SIZE_ATTACHMENT_RESULTS := 2
const OLDER_ATTACHMENT_MAX_LONG_EDGE := 1000

enum StopReason {
	END_TURN,
	TOOL_USE,
	PAUSE_TURN,
	MAX_TOKENS,
	REFUSAL,
	UNKNOWN,
}


class Usage extends RefCounted:
	var input_tokens := 0
	var output_tokens := 0
	var cache_creation_input_tokens := 0
	var cache_read_input_tokens := 0

	func add(p_other: Usage) -> void:
		input_tokens += p_other.input_tokens
		output_tokens += p_other.output_tokens
		cache_creation_input_tokens += p_other.cache_creation_input_tokens
		cache_read_input_tokens += p_other.cache_read_input_tokens

	func to_dict() -> Dictionary:
		return {
			input_tokens = input_tokens,
			output_tokens = output_tokens,
			cache_creation_input_tokens = cache_creation_input_tokens,
			cache_read_input_tokens = cache_read_input_tokens,
		}


class ResponseError extends RefCounted:
	var type: String
	var message: String
	var param: String

	func _init(p_type: String, p_message: String, p_param := "") -> void:
		type = p_type
		message = p_message
		param = p_param


class Response extends RefCounted:
	var message: Chat.Message
	var stop_reason := StopReason.UNKNOWN
	var usage := Usage.new()
	var raw: Dictionary
	var error: ResponseError

	static func failed(p_type: String, p_message: String, p_param := "") -> Response:
		var resp := Response.new()
		resp.error = ResponseError.new(p_type, p_message, p_param)
		return resp

	func is_error() -> bool:
		return error != null

	func is_success() -> bool:
		return not is_error()

	func get_error() -> ResponseError:
		return error


class WebRequest extends RefCounted:
	var url: String
	var headers: PackedStringArray
	var payload: Dictionary


class RequestOptions extends RefCounted:
	var max_tokens: int
	## Empty means the model's default. Only sent when the model is known to take it.
	var effort: String
	var thinking := true
	## 0 means the model's default.
	var budget_tokens := 0
	## What models.dev says about the model; null for a model nobody has a record of.
	var model_info: ModelInfo
	var tools: Array[ToolManager.Tool]
	## Options this provider already rejected for the model, so they aren't sent again.
	var dropped_options: Array[String]


var url: String
var api_key: String
var model: String


func _init(p_url: String, p_api_key: String, p_model: String) -> void:
	url = p_url
	api_key = p_api_key
	model = p_model


@abstract
func build_request(p_chat: Chat, p_options: RequestOptions) -> WebRequest


## Gets attachments to send, shrinking images beyond the most recent ones.
static func attachments_to_send(p_chat: Chat) -> Dictionary:
	var result := {}
	var full_size_left := FULL_SIZE_ATTACHMENT_RESULTS
	for i in range(p_chat.messages.size() - 1, -1, -1):
		var content: Array = p_chat.messages[i].content
		for j in range(content.size() - 1, -1, -1):
			var c = content[j]
			if not (c is Chat.ToolResultContent) or c.attachments.is_empty():
				continue
			if full_size_left > 0:
				full_size_left -= 1
				result[c] = c.attachments
			else:
				result[c] = c.attachments.map(func (a): return a.scaled_to(OLDER_ATTACHMENT_MAX_LONG_EDGE))
	return result


@abstract
func parse_response(p_code: int, p_data: Dictionary) -> Response


## The available reasoning settings this API can use (named per models.dev).
static func get_reasoning_options() -> PackedStringArray:
	return PackedStringArray()


## The name of the request option the API refused, so the client can retry without it.
func get_rejected_option(_p_error: ResponseError) -> String:
	return ""


static func stop_reason_name(p_reason: StopReason) -> String:
	return StopReason.keys()[p_reason].to_lower()
