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
				return TextContent.from_dict(p_data)
			"image":
				return ImageContent.from_dict(p_data)
			"tool_use":
				return ToolUseContent.from_dict(p_data)
			"tool_result":
				return ToolResultContent.from_dict(p_data)
			"thinking":
				return ThinkingContent.from_dict(p_data)

		return UnknownContent.from_dict(p_data)


class TextContent extends MessageContent:
	var text: String

	func _init(p_text: String) -> void:
		text = p_text

	func to_dict() -> Dictionary:
		return {type = "text", text = text}

	static func from_dict(p_data: Dictionary) -> MessageContent:
		return TextContent.new(str(p_data.get("text", "")))


class ImageContent extends MessageContent:
	const MAX_LONG_EDGE := 2000

	var format: String
	var data: PackedByteArray

	var _scaled := {}

	func _init(p_format: String, p_data: PackedByteArray) -> void:
		format = p_format
		data = p_data

	func to_dict() -> Dictionary:
		return {type = "image", format = format, data = to_base64()}

	func mime_type() -> String:
		return "image/" + format

	func to_base64() -> String:
		return Marshalls.raw_to_base64(data)

	static func from_dict(p_data: Dictionary) -> MessageContent:
		var base64_data = str(p_data.get("data", ""))
		return ImageContent.new(str(p_data.get("format", "")), Marshalls.base64_to_raw(base64_data))

	static func from_image(p_image: Image, p_format: String = "png") -> ImageContent:
		var image := _fit_to_long_edge(p_image, MAX_LONG_EDGE)
		var data
		match p_format:
			"png":
				data = image.save_png_to_buffer()
			"jpeg":
				data = image.save_jpg_to_buffer()
			"webp":
				data = image.save_webp_to_buffer()

		if data == null or data.is_empty():
			return null

		return ImageContent.new(p_format, data)

	func scaled_to(p_max_long_edge: int) -> ImageContent:
		if _scaled.has(p_max_long_edge):
			return _scaled[p_max_long_edge]

		var scaled: ImageContent = self
		var image := to_image()
		if image != null and _long_edge(image) > p_max_long_edge:
			scaled = from_image(_fit_to_long_edge(image, p_max_long_edge), format)
			if scaled == null:
				scaled = self

		_scaled[p_max_long_edge] = scaled
		return scaled

	static func _long_edge(p_image: Image) -> int:
		return maxi(p_image.get_width(), p_image.get_height())

	static func _fit_to_long_edge(p_image: Image, p_max_long_edge: int) -> Image:
		var long_edge := _long_edge(p_image)
		if long_edge <= p_max_long_edge:
			return p_image

		var scale := float(p_max_long_edge) / long_edge
		var image: Image = p_image.duplicate()
		image.resize(maxi(1, roundi(p_image.get_width() * scale)), maxi(1, roundi(p_image.get_height() * scale)), Image.INTERPOLATE_LANCZOS)
		return image

	func to_image() -> Image:
		var img := Image.new()
		var err: Error = FAILED
		match format:
			"png":
				err = img.load_png_from_buffer(data)
			"jpeg":
				err = img.load_jpg_from_buffer(data)
			"webp":
				err = img.load_webp_from_buffer(data)
		if err != OK:
			img = null
		return img


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

	static func from_dict(p_data: Dictionary) -> MessageContent:
		var input = p_data.get("input")
		return ToolUseContent.new(str(p_data.get("id", "")), str(p_data.get("name", "")), input if input is Dictionary else {})


class ToolResultContent extends MessageContent:
	var tool_use_id: String
	var content
	var is_error: bool
	var attachments: Array[MessageContent]

	func _init(p_tool_use_id: String, p_content = "", p_is_error := false) -> void:
		tool_use_id = p_tool_use_id
		content = p_content
		is_error = p_is_error

	func to_dict() -> Dictionary:
		var d = {type = "tool_result", tool_use_id = tool_use_id, content = content, is_error = is_error}

		if attachments.size() > 0:
			d['attachments'] = []
			for a in attachments:
				d['attachments'].push_back(a.to_dict())

		return d

	static func from_dict(p_data: Dictionary) -> MessageContent:
		var c := ToolResultContent.new(str(p_data.get("tool_use_id", "")), p_data.get("content", ""), bool(p_data.get("is_error", false)))

		var attachment_data = p_data.get("attachments")
		if attachment_data is Array and attachment_data.size() > 0:
			for ad in attachment_data:
				c.add_attachment(MessageContent.from_dict(ad))

		return c

	func add_attachment(p_content: MessageContent) -> void:
		if not p_content is ImageContent:
			return
		attachments.push_back(p_content)

	func add_attachments(p_attachments: Array) -> void:
		for attachment in p_attachments:
			if attachment is Image:
				add_attachment(ImageContent.from_image(attachment))


class ThinkingContent extends MessageContent:
	var thinking: String
	var signature: String

	func _init(p_thinking: String, p_signature := "") -> void:
		thinking = p_thinking
		signature = p_signature

	func to_dict() -> Dictionary:
		return {type = "thinking", thinking = thinking, signature = signature}

	static func from_dict(p_data: Dictionary) -> MessageContent:
		return ThinkingContent.new(str(p_data.get("thinking", "")), str(p_data.get("signature", "")))


class UnknownContent extends MessageContent:
	var data: Dictionary

	func _init(p_data: Dictionary) -> void:
		data = p_data

	func to_dict() -> Dictionary:
		return data

	static func from_dict(p_data: Dictionary) -> MessageContent:
		return UnknownContent.new(p_data)


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

var system: String
var messages: Array[Message]


func add_message(p_msg: Message) -> void:
	messages.push_back(p_msg)
	message_added.emit(p_msg)


func to_dict() -> Dictionary:
	return {
		system = system,
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
	if not system.is_empty():
		print({system = system})
	for msg in messages:
		print(msg.to_dict())
