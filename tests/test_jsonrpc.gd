extends Node2D

const JSONRPCDispatcher = preload("res://addons/godai/jsonrpc_dispatcher.gd")

const TESTS = [
	# These first tests are taken from the JSONRPC 2.0 spec.
	{
		name = "PositionalParams1",
		request = '{"jsonrpc": "2.0", "method": "subtract", "params": [42, 23], "id": 1}',
		response = '{"jsonrpc": "2.0", "result": 19, "id": 1}',
	},
	{
		name = "PositionalParams2",
		request = '{"jsonrpc": "2.0", "method": "subtract", "params": [23, 42], "id": 2}',
		response = '{"jsonrpc": "2.0", "result": -19, "id": 2}',
	},
	{
		name = "NamedParams1",
		request = '{"jsonrpc": "2.0", "method": "subtract", "params": {"subtrahend": 23, "minuend": 42}, "id": 3}',
		response = '{"jsonrpc": "2.0", "result": 19, "id": 3}',
	},
	{
		name = "NamedParams2",
		request = '{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4}',
		response = '{"jsonrpc": "2.0", "result": 19, "id": 4}',
	},
	{
		name = "Notification1",
		request = '{"jsonrpc": "2.0", "method": "update", "params": [1,2,3,4,5]}',
		response = "",
	},
	{
		name = "Notification2",
		request = '{"jsonrpc": "2.0", "method": "foobar"}',
		response = "",
	},
	{
		name = "NonExistentMethod",
		request = '{"jsonrpc": "2.0", "method": "foobar", "id": "1"}',
		response = '{"jsonrpc": "2.0", "error": {"code": -32601, "message": "Method not found"}, "id": "1"}',
	},
	{
		name = "InvalidJSON",
		request = '{"jsonrpc": "2.0", "method": "foobar, "params": "bar", "baz]',
		response = '{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}, "id": null}',
	},
	{
		name = "InvalidRequest",
		request = '{"jsonrpc": "2.0", "method": 1, "params": "bar"}',
		response = '{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}',
	},
	{
		name = "BatchInvalidJSON",
		request = '[
			{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
			{"jsonrpc": "2.0", "method"
		]',
		response = '{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}, "id": null}',
	},
	{
		name = "BatchEmptyArray",
		request = '[]',
		response = '{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}',
	},
	{
		name = "BatchInvalidRequest1",
		request = '[1]',
		response = '[
			{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}
		]',
	},
	{
		name = "BatchInvalidRequest2",
		request = '[1,2,3]',
		response = '[
			{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null},
			{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null},
			{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null}
		]',
	},
	{
		name = "BatchNormal",
		request = '[
			{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
			{"jsonrpc": "2.0", "method": "notify_hello", "params": [7]},
			{"jsonrpc": "2.0", "method": "subtract", "params": [42,23], "id": "2"},
			{"foo": "boo"},
			{"jsonrpc": "2.0", "method": "foo.get", "params": {"name": "myself"}, "id": "5"},
			{"jsonrpc": "2.0", "method": "get_data", "id": "9"}
		]',
		response = '[
			{"jsonrpc": "2.0", "result": 7, "id": "1"},
			{"jsonrpc": "2.0", "result": 19, "id": "2"},
			{"jsonrpc": "2.0", "error": {"code": -32600, "message": "Invalid request"}, "id": null},
			{"jsonrpc": "2.0", "error": {"code": -32601, "message": "Method not found"}, "id": "5"},
			{"jsonrpc": "2.0", "result": ["hello", 5], "id": "9"}
		]',
	},
	{
		name = "BatchAllNotifications",
		request = '[
			{"jsonrpc": "2.0", "method": "sum", "params": [1,2,4]},
			{"jsonrpc": "2.0", "method": "notify_hello", "params": [7]}
		]',
		response = "",
	},

	# These are for testing async results.
	{
		name = "Async",
		request = '{"jsonrpc": "2.0", "method": "async_result", "params": {"value": 27}, "id": 10}',
		response = '{"jsonrpc": "2.0", "result": "Value is 27", "id": 10}',
	},
	{
		name = "BatchAsync",
		request = '[
			{"jsonrpc": "2.0", "method": "async_result", "params": {"value": 9}, "id": 11},
			{"jsonrpc": "2.0", "method": "async_result", "params": {"value": 11}, "id": 12}
		]',
		response = '[
			{"jsonrpc": "2.0", "result": "Value is 9", "id": 11},
			{"jsonrpc": "2.0", "result": "Value is 11", "id": 12}
		]',
	},


]

func _ready() -> void:
	var d := JSONRPCDispatcher.new()
	d.set_method("subtract", func (p_params):
		if p_params is Array:
			var nums := []
			for p in p_params:
				if p is int or p is float:
					nums.push_back(p)
				else:
					return JSONRPCDispatcher.ResponseError.invalid_params()
			if nums.size() < 2:
				return JSONRPCDispatcher.ResponseError.invalid_params()

			var result = nums[0]
			for i in range(1, nums.size()):
				result -= nums[i]
			return result

		elif p_params is Dictionary:
			if not p_params.has('subtrahend') or not p_params.has('minuend'):
				return JSONRPCDispatcher.ResponseError.invalid_params()
			return p_params['minuend'] - p_params['subtrahend']

		return JSONRPCDispatcher.ResponseError.invalid_params()
	)
	d.set_method("sum", func(p_params):
		var nums := []
		for p in p_params:
			if p is int or p is float:
				nums.push_back(p)
			else:
				return JSONRPCDispatcher.ResponseError.invalid_params()
		if nums.size() < 2:
			return JSONRPCDispatcher.ResponseError.invalid_params()

		var result = nums[0]
		for i in range(1, nums.size()):
			result += nums[i]
		return result
	)
	d.set_method("notify_hello", func(_params):
		return "hello"
	)
	d.set_method("get_data", func(_params):
		return ["hello", 5]
	)
	d.set_method("async_result", func(p_params):
		var result = JSONRPCDispatcher.AsyncResult.new()
		var cb = func ():
			result.resolve("Value is %d" % p_params['value'])
		cb.call_deferred()
		return result
	)

	var passes := 0
	var fails := 0
	for test in TESTS:
		var response = await d.process_string(test['request'])

		if _assert_equal(test['name'], _normalize_json(response), _normalize_json(test['response'])):
			passes += 1
		else:
			fails += 1

	print("PASSES: %d out of %d" % [passes, passes + fails])


func _assert_equal(p_name: String, p_result: String, p_expected) -> bool:
	if p_result != p_expected:
		print("FAIL (%s): '%s' does not equal '%s'" % [p_name, p_result, p_expected])
		return false
	return true


func _normalize_json(p_input: String) -> String:
	if p_input == "":
		return ""
	var json := JSON.new()
	var err = json.parse(p_input)
	if err == OK:
		return JSON.stringify(json.data)

	print("ERROR NORMALIZING JSON: ", p_input)
	return ""
