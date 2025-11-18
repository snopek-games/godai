extends RefCounted

# Note for future generations:
#
# We can't use Godot's `JSONRPC` class, because we need to support asynchronous results
# that we can `await`, in order to support asynchronous tools.

class AsyncResult extends RefCounted:
	signal completed(content)

	func resolve(p_content) -> void:
		completed.emit(p_content)


enum ErrorCode {
	PARSE_ERROR = -32700,
	INVALID_REQUEST_ERROR = -32600,
	METHOD_NOT_FOUND_ERROR = -32601,
	INVALID_PARAMS_ERROR = -32602,
	INTERNAL_ERROR = -32603,
	SERVER_ERROR_MIN = -32000,
	SERVER_ERROR_MAX = -32099,
}


class ResponseError extends RefCounted:
	var code: ErrorCode
	var message: String
	var data

	func _init(p_code: ErrorCode, p_msg: String, p_data = null) -> void:
		code = p_code
		message = p_msg
		data = p_data

	func to_dict() -> Dictionary:
		var error := {
			code = code,
			message = message,
		}
		if data != null:
			error['data'] = data
		return error

	static func invalid_params(p_data = null) -> ResponseError:
		return ResponseError.new(ErrorCode.INVALID_PARAMS_ERROR, "Invalid params", p_data)


var _methods: Dictionary[String, Callable]


func set_method(p_name: String, p_callback: Callable) -> void:
	_methods[p_name] = p_callback


func process_string(p_string: String):
	var requests := []
	var is_batch := _parse_requests(p_string, requests)

	if requests.size() == 0:
		return JSON.stringify(_create_error_response(null, ErrorCode.PARSE_ERROR, "Parse error"))

	var responses := []

	for req in requests:
		if req == null:
			responses.push_back(_create_error_response(null, ErrorCode.INVALID_REQUEST_ERROR, "Invalid request"))
		else:
			var resp = await _handle_one(req)
			if req.has('id') and str(req['id']).length() > 0:
				responses.push_back(resp)

	if responses.size() == 0:
		return ""

	if is_batch:
		return JSON.stringify(responses)

	return JSON.stringify(responses[0])

# The return value indicates if this is a batch or not
func _parse_requests(p_string: String, r_requests: Array) -> bool:
	if p_string == "":
		return false

	var json := JSON.new()
	var err := json.parse(p_string)
	if err != OK:
		return false
	var data = json.data

	var is_batch: bool

	if data is Array:
		if data.size() == 0:
			r_requests.push_back(null)
		else:
			is_batch = true
			for req in data:
				if req is Dictionary and _is_valid_request(req):
					r_requests.push_back(req)
				else:
					r_requests.push_back(null)
	elif data is Dictionary:
		if _is_valid_request(data):
			r_requests.push_back(data)
		else:
			r_requests.push_back(null)
	else:
		return false

	return is_batch


func _is_valid_request(p_request: Dictionary) -> bool:
	if not p_request.has("jsonrpc") or p_request['jsonrpc'] != "2.0":
		return false

	if not p_request.has("method") or not p_request['method'] is String:
		return false

	return true


func _handle_one(p_request: Dictionary) -> Dictionary:
	var resp := {
		jsonrpc = "2.0",
		id = p_request.get('id'),
	}

	var cb: Callable = _methods.get(p_request['method'], Callable())
	if not cb.is_valid():
		resp['error'] = _create_error(ErrorCode.METHOD_NOT_FOUND_ERROR, "Method not found")
		return resp

	var result = cb.call(p_request.get('params', {}))
	if result is ResponseError:
		resp['error'] = result.to_dict()
	if result is AsyncResult:
		resp['result'] = await result.completed
	else:
		resp['result'] = result

	return resp


func _create_error(p_code: ErrorCode, p_msg: String, p_data = null) -> Dictionary:
	var error := {
		code = p_code,
		message = p_msg
	}
	if p_data != null:
		error['data'] = p_data
	return error


func _create_error_response(p_id, p_code: ErrorCode, p_msg: String, p_data = null) -> Dictionary:
	return {
		jsonrpc = "2.0",
		id = p_id,
		error = ResponseError.new(p_code, p_msg, p_data).to_dict(),
	}
