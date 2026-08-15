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

class ToolAnnotations extends RefCounted:
	var read_only_hint: bool = false
	var destructive_hint: bool = true
	var idempotent_hint: bool = false
	var open_world_hint: bool = true

	func from_dict(p_json: Dictionary) -> void:
		read_only_hint = p_json.get("readOnlyHint", false)
		destructive_hint = p_json.get("destructiveHint", true)
		idempotent_hint = p_json.get("idempotentHint", false)
		open_world_hint = p_json.get("openWorldHint", true)

	func to_dict() -> Dictionary:
		var d := {
			"readOnlyHint": read_only_hint,
			"openWorldHint": open_world_hint,
		}
		if not read_only_hint:
			d["destructiveHint"] = destructive_hint
			d["idempotentHint"] = idempotent_hint
		return d

@abstract
class Tool extends RefCounted:
	const JSON_TYPE_NAMES := {
		object = "an object",
		array = "an array",
		string = "a string",
		boolean = "a boolean",
		integer = "a number",
		number = "a number",
	}

	var name: String
	var title: String
	var description: String
	var input_schema: Dictionary
	var output_schema: Dictionary
	var annotations: ToolAnnotations

	@abstract
	func execute(p_input) -> ToolResult

	## Checks the input against the top level of the tool's own schema: that
	## everything in 'required' is there, that it isn't empty when the schema
	## sets a minimum size, and that whatever is there has the declared type.
	## Anything deeper is left to the tool itself, where the error message can
	## say something more useful than a schema path.
	##
	## Returns an error message, or "" when the input is usable.
	func check_input(p_input) -> String:
		if not p_input is Dictionary:
			return "Input must be %s, but got %s" % [JSON_TYPE_NAMES['object'], _json_type_name(p_input)]

		var properties: Dictionary = input_schema.get('properties', {})

		# An empty path, name or list of things to act on is no more of an
		# argument than a missing one, so both get the same message.
		for required_name in input_schema.get('required', []):
			if not p_input.has(required_name):
				return "'%s' is required" % required_name
			if _is_empty(p_input[required_name]) and _get_schema_minimum_size(properties.get(required_name, {})) > 0:
				return "'%s' is required" % required_name

		for input_name in p_input:
			if not properties.has(input_name):
				continue

			var expected_type: String = properties[input_name].get('type', '')
			if not JSON_TYPE_NAMES.has(expected_type):
				continue
			if _matches_json_type(p_input[input_name], expected_type):
				continue

			return "'%s' must be %s, but got %s" % [
				input_name,
				JSON_TYPE_NAMES[expected_type],
				_json_type_name(p_input[input_name]),
			]

		return ""

	## The smallest a property's schema lets its value be. Each JSON type spells
	## that its own way, and leaving it out means zero.
	func _get_schema_minimum_size(p_property: Dictionary) -> int:
		match p_property.get('type', ''):
			"string":
				return p_property.get('minLength', 0)
			"array":
				return p_property.get('minItems', 0)
			"object":
				return p_property.get('minProperties', 0)
		return 0

	func _is_empty(p_value) -> bool:
		match typeof(p_value):
			TYPE_NIL:
				return true
			TYPE_STRING, TYPE_STRING_NAME, TYPE_ARRAY, TYPE_DICTIONARY:
				return p_value.is_empty()
		return false

	func _matches_json_type(p_value, p_type: String) -> bool:
		match p_type:
			"object":
				return p_value is Dictionary
			"array":
				return p_value is Array
			"string":
				return p_value is String or p_value is StringName
			"boolean":
				return p_value is bool
			"integer", "number":
				return p_value is int or p_value is float
		return true

	func _json_type_name(p_value) -> String:
		for type_name in JSON_TYPE_NAMES:
			if _matches_json_type(p_value, type_name):
				return JSON_TYPE_NAMES[type_name]
		return "null" if p_value == null else "a %s" % type_string(typeof(p_value))

	func is_read_only() -> bool:
		return annotations and annotations.read_only_hint

	func is_destructive() -> bool:
		if annotations:
			return not annotations.read_only_hint and annotations.destructive_hint
		return true

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

		if p_data.has("annotations"):
			annotations = ToolAnnotations.new()
			annotations.from_dict(p_data["annotations"])

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

	var input_error := tool_obj.check_input(p_input)
	if not input_error.is_empty():
		return ToolResult.rejected({errors = [input_error]})

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
