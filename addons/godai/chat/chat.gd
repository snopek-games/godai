extends RefCounted

enum Role {
	USER,
	ASSISTANT,
}

const ROLE_NAMES := {
	Role.USER: "user",
	Role.ASSISTANT: "assistant",
}

const DANGLING_TOOL_USE_MESSAGE = "Something went wrong and this tool didn't record its result. It may or may not have taken effect."


@abstract
class MessageContent extends RefCounted:
	@abstract
	func to_dict() -> Dictionary

	static func from_dict(p_data: Dictionary) -> MessageContent:
		var type = p_data.get("type")
		if not (type is String):
			push_error("MessageContent is missing type: %s" % p_data)
			return null

		match type:
			"text":
				return TextContent.new(str(p_data.get("text", "")))
			"tool_use":
				var input = p_data.get("input")
				return ToolUseContent.new(str(p_data.get("id", "")), str(p_data.get("name", "")), input if input is Dictionary else {})
			"tool_result":
				return ToolResultContent.new(str(p_data.get("tool_use_id", "")), p_data.get("content", ""), bool(p_data.get("is_error", false)))
			"thinking":
				return ThinkingContent.new(str(p_data.get("thinking", "")), str(p_data.get("signature", "")))

		return UnknownContent.new(p_data)


class TextContent extends MessageContent:
	var text: String

	func _init(p_text: String) -> void:
		text = p_text

	func to_dict() -> Dictionary:
		return {type = "text", text = text}


class ToolUseContent extends MessageContent:
	var id: String
	var name: String
	var input: Dictionary

	func _init(p_id: String, p_name: String, p_input: Dictionary = {}) -> void:
		id = p_id
		name = p_name
		input = p_input

	func to_dict() -> Dictionary:
		return {type = "tool_use", id = id, name = name, input = input}


class ToolResultContent extends MessageContent:
	var tool_use_id: String
	var content
	var is_error: bool

	func _init(p_tool_use_id: String, p_content = "", p_is_error := false) -> void:
		tool_use_id = p_tool_use_id
		content = p_content
		is_error = p_is_error

	func to_dict() -> Dictionary:
		return {type = "tool_result", tool_use_id = tool_use_id, content = content, is_error = is_error}


class ThinkingContent extends MessageContent:
	var thinking: String
	var signature: String

	func _init(p_thinking: String, p_signature := "") -> void:
		thinking = p_thinking
		signature = p_signature

	func to_dict() -> Dictionary:
		return {type = "thinking", thinking = thinking, signature = signature}


class UnknownContent extends MessageContent:
	var data: Dictionary

	func _init(p_data: Dictionary) -> void:
		data = p_data

	func to_dict() -> Dictionary:
		return data


class Message extends RefCounted:
	var role: Role
	var content: Array[MessageContent]

	func _init(p_role: Role, p_content = []) -> void:
		role = p_role

		var items: Array = p_content if p_content is Array else [p_content]
		for item in items:
			if item is String:
				content.push_back(TextContent.new(item))
			elif item is MessageContent:
				content.push_back(item)
			else:
				push_error("Invalid message content: %s" % [item])

	func remove_tool_use() -> void:
		var kept: Array[MessageContent]
		for c in content:
			if not (c is ToolUseContent):
				kept.push_back(c)
		content = kept

	func to_dict() -> Dictionary:
		return {
			role = ROLE_NAMES[role],
			content = content.map(func (c): return c.to_dict()),
		}

	static func from_dict(p_data: Dictionary) -> Message:
		if not (p_data.get("role") is String) or not p_data.has("content"):
			push_error("Message is missing role or content: %s" % p_data)
			return null

		var role_key = ROLE_NAMES.find_key(p_data["role"])
		if role_key == null:
			push_error("Message has an unknown role: %s" % p_data)
			return null

		var raw_content = p_data["content"]
		var items: Array = raw_content if raw_content is Array else [raw_content]
		var parsed: Array[MessageContent]
		for item in items:
			if item is String:
				parsed.push_back(TextContent.new(item))
			elif item is Dictionary:
				var c := MessageContent.from_dict(item)
				if c:
					parsed.push_back(c)
			else:
				push_error("Invalid message content: %s" % [item])

		return Message.new(role_key as Role, parsed)


signal message_added(message: Message)

var messages: Array[Message]


func add_message(p_msg: Message) -> void:
	messages.push_back(p_msg)
	message_added.emit(p_msg)


func to_dict() -> Dictionary:
	return {
		messages = messages.map(func (v): return v.to_dict()),
	}


func repair_dangling_tool_use() -> void:
	var answered := {}
	for msg in messages:
		for c in msg.content:
			if c is ToolResultContent:
				answered[c.tool_use_id] = true

	var i := 0
	while i < messages.size():
		var results: Array[MessageContent]
		for c in messages[i].content:
			if c is ToolUseContent and not answered.has(c.id):
				results.push_back(ToolResultContent.new(c.id, DANGLING_TOOL_USE_MESSAGE, true))
		i += 1
		if results.is_empty():
			continue
		if i < messages.size() and messages[i].role == Role.USER:
			results.append_array(messages[i].content)
			messages[i].content = results
		else:
			messages.insert(i, Message.new(Role.USER, results))
			i += 1


func print_debug() -> void:
	print(" === CHAT:")
	for msg in messages:
		print(msg.to_dict())
