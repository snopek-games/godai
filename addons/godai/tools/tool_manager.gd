extends RefCounted

const INPUT_SCHEMA_EMPTY = {type = "object", properties = {}}
const OUTPUT_SCHEMA_STRING = {type = "string"}


class ToolResult extends RefCounted:
	var content

	var _done := false
	var _error := false

	signal completed(content)

	func resolve(p_content) -> void:
		if _done:
			return
		_done = true
		content = p_content
		completed.emit(content)

	func reject(p_content) -> void:
		if _done:
			return
		_done = true
		_error = true
		content = p_content
		completed.emit(content)

	func is_done() -> bool:
		return _done

	func is_error() -> bool:
		return _error

	func get_content_as_string() -> String:
		if content is String:
			return content
		return JSON.stringify(content)

	static func resolved(p_content) -> ToolResult:
		var result := ToolResult.new()
		result.resolve(p_content)
		return result

	static func rejected(p_content) -> ToolResult:
		var result := ToolResult.new()
		result.reject(p_content)
		return result

@abstract
class Tool extends RefCounted:
	var name: String
	var title: String
	var description: String
	var input_schema: Dictionary
	var output_schema: Dictionary

	@abstract
	func execute(p_input) -> ToolResult

	func to_dict() -> Dictionary:
		var data := {
			name = name
		}
		if description.length() > 0:
			data['description'] = description
		if input_schema.size() > 0:
			data['input_schema'] = input_schema
		return data


class CallbackTool extends Tool:
	var callback: Callable

	func _init(p_name: String, p_title: String, p_description: String, p_callback: Callable, p_input_schema: Dictionary = INPUT_SCHEMA_EMPTY, p_output_schema: Dictionary = OUTPUT_SCHEMA_STRING) -> void:
		name = p_name
		title = p_title
		description = p_description
		input_schema = p_input_schema
		callback = p_callback

	func execute(p_input) -> ToolResult:
		return callback.call(p_input)

## Base class for the built-in tools: fills in the tool's name, title,
## description and input schema from its entry in default_tools.json.
@abstract
class DefaultTool extends Tool:
	func _init(p_data: Dictionary) -> void:
		name = p_data['name']
		title = p_data.get('title', name)

		var raw_desc = p_data['description']
		if raw_desc is Array:
			description = "\n".join(raw_desc)
		else:
			description = raw_desc

		input_schema = p_data.get("inputSchema", INPUT_SCHEMA_EMPTY)
		output_schema = p_data.get("outputSchema", {})

class QueueItem extends RefCounted:
	var tool_obj: Tool
	var input
	var result_proxy: ToolResult

	func _init(p_tool: Tool, p_input, p_result_proxy: ToolResult) -> void:
		tool_obj = p_tool
		input = p_input
		result_proxy = p_result_proxy


var tools: Dictionary[String, Tool]

var _current: ToolResult
var _queue: Array[QueueItem]


func register_tool(p_tool: Tool) -> void:
	if p_tool.name == "":
		push_error("Cannot register tool without name")
		return

	if p_tool.input_schema.size() == 0:
		push_error("Cannot register tool without input_schema")
		return

	if p_tool is CallbackTool and p_tool.callback.is_null():
		push_error("Cannot register CallbackTool without a valid callback")
		return

	tools[p_tool.name] = p_tool


func get_tool(p_name: String) -> Tool:
	return tools.get(p_name)


func get_tools() -> Array[Tool]:
	var ret: Array[Tool]
	ret.assign(tools.values())
	return ret


func has_tool(p_name: String) -> bool:
	return tools.has(p_name)


func is_executing() -> bool:
	return _current != null


func execute_tool(p_name: String, p_input) -> ToolResult:
	if not tools.has(p_name):
		return null

	var tool_obj: Tool = tools[p_name]

	if not _current:
		var result: ToolResult = tool_obj.execute(p_input)
		if result.is_done():
			return result
		_current = result
		_current.completed.connect(_handle_result.bind(null), CONNECT_ONE_SHOT)
		return _current
	else:
		var proxy_result := ToolResult.new()
		var queue_item := QueueItem.new(tool_obj, p_input, proxy_result)
		_queue.push_back(queue_item)
		return proxy_result


func _handle_result(p_content, p_proxy_result: ToolResult) -> void:
	_current = null
	if p_proxy_result:
		p_proxy_result.resolve(p_content)
	_pump_queue.call_deferred()


func _pump_queue() -> void:
	if _queue.size() == 0:
		return
	if _current:
		return

	var queue_item: QueueItem = _queue.pop_front()

	var result: ToolResult = queue_item.tool_obj.execute(queue_item.input)
	if result.is_done():
		queue_item.result_proxy.resolve(result.content)
		_pump_queue.call_deferred()
		return

	_current = result
	_current.completed.connect(_handle_result.bind(queue_item.result_proxy), CONNECT_ONE_SHOT)
