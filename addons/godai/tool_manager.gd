extends RefCounted

const INPUT_SCHEMA_EMPTY = {type = "object", properties = {}}
const OUTPUT_SCHEMA_STRING = {type = "string"}


class ToolResult extends RefCounted:
	var content

	var _done := false

	signal completed(content)

	func resolve(p_content) -> void:
		if _done:
			return
		_done = true
		content = p_content
		completed.emit(content)

	func resolve_json(p_content) -> void:
		var content = p_content
		if not content is String:
			content = JSON.stringify(content)
		return resolve(content)

	func is_done() -> bool:
		return _done

	static func resolved(p_content) -> ToolResult:
		var result := ToolResult.new()
		result.resolve(p_content)
		return result

	static func resolved_json(p_content) -> ToolResult:
		var result := ToolResult.new()
		result.resolve_json(p_content)
		return result


@abstract
class Tool extends RefCounted:
	var name: String
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


class ToolCallback extends Tool:
	var callback: Callable

	func _init(p_name: String, p_description: String, p_callback: Callable, p_input_schema: Dictionary = INPUT_SCHEMA_EMPTY, p_output_schema: Dictionary = OUTPUT_SCHEMA_STRING) -> void:
		name = p_name
		description = p_description
		input_schema = p_input_schema
		callback = p_callback

	func execute(p_input) -> ToolResult:
		return callback.call(p_input)


var tools: Dictionary[String, Tool]


func register_tool(p_tool: Tool) -> void:
	if p_tool.name == "":
		push_error("Cannot register tool without name")
		return

	if p_tool.input_schema.size() == 0:
		push_error("Cannot register tool without input_schema")
		return

	if p_tool is ToolCallback and p_tool.callback.is_null():
		push_error("Cannot register ToolCallback without a valid callback")
		return

	tools[p_tool.name] = p_tool

