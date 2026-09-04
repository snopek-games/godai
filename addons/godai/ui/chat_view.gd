@tool
extends PanelContainer

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const LoadingLabel = preload("res://addons/godai/ui/loading_label.gd")

const UserChatScene = preload("res://addons/godai/ui/user_chat.tscn")
const AssistantChatScene = preload("res://addons/godai/ui/assistant_chat.tscn")
const ToolChatScene = preload("res://addons/godai/ui/tool_chat.tscn")
const ErrorChatScene = preload("res://addons/godai/ui/error_chat.tscn")

@onready var chat_scroll: ScrollContainer = %ChatScroll
@onready var chat_container: VBoxContainer = %ChatContainer
@onready var loading_label: LoadingLabel = %LoadingLabel
@onready var tool_use_info_dialog: AcceptDialog = %ToolUseInfoDialog

var tools: ToolManager

var _pending_tool_items: Dictionary
var _scroll_queued := false


func _ready() -> void:
	_update_panel_theme()


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_THEME_CHANGED:
			_update_panel_theme()


func _update_panel_theme() -> void:
	if is_part_of_edited_scene():
		return

	var stylebox: StyleBox = get_theme_stylebox("normal", "RichTextLabel")
	# Overriding our own stylebox re-triggers THEME_CHANGED, so bail if it
	# already matches to avoid infinite recursion.
	if get_theme_stylebox("panel") == stylebox:
		return
	add_theme_stylebox_override("panel", stylebox)


func show_chat(p_chat: ClaudeClient.Chat) -> void:
	clear()
	if p_chat:
		for msg in p_chat.messages:
			show_message(msg)


func show_message(p_msg: ClaudeClient.Message) -> void:
	for content in p_msg.content:
		var data: Dictionary = content.data
		match content.get_type():
			"text":
				var text: String = data.get("text", "")
				if text:
					if p_msg.role == "user":
						if text == ClaudeClient.CANCELLED_MESSAGE:
							_add_cancelled_to_chat()
						else:
							var chat = UserChatScene.instantiate()
							chat_container.add_child(chat)
							chat.setup_user_chat(text)
					elif p_msg.role == "assistant":
						var chat = AssistantChatScene.instantiate()
						chat_container.add_child(chat)
						chat.setup_assistant_chat(text)

				scroll_to_bottom()

			"tool_use":
				var input = data.get("input")
				_add_tool_use_to_chat(str(data.get("id", "")), str(data.get("name", "")),
					input if input is Dictionary else {})

			"tool_result":
				_add_tool_result_to_chat(str(data.get("tool_use_id", "")), data.get("content", ""))


func show_error(p_msg: String) -> void:
	var chat = ErrorChatScene.instantiate()
	chat_container.add_child(chat)
	chat.setup_error_chat(p_msg)


func clear() -> void:
	_pending_tool_items.clear()
	for chat in chat_container.get_children():
		chat.queue_free()


func scroll_to_bottom() -> void:
	if _scroll_queued:
		return
	_scroll_queued = true
	await get_tree().process_frame
	_scroll_queued = false
	var scrollbar: VScrollBar = chat_scroll.get_v_scroll_bar()
	chat_scroll.scroll_vertical = scrollbar.max_value


func set_loading(p_visible: bool, p_text := "Thinking") -> void:
	loading_label.visible = p_visible
	loading_label.base_text = p_text


func _add_tool_use_to_chat(p_id: String, p_name: String, p_input: Dictionary) -> void:
	var chat = ToolChatScene.instantiate()
	chat_container.add_child(chat)
	var tool_obj = tools.get_tool(p_name)
	chat.setup_tool_chat(p_id, p_name, tool_obj.title if tool_obj else p_name, p_input)
	chat.info_requested.connect(_show_tool_info)
	_pending_tool_items[p_id] = chat
	scroll_to_bottom()


func _add_tool_result_to_chat(p_id: String, p_content) -> void:
	var chat = _pending_tool_items.get(p_id)
	if chat:
		chat.set_tool_output(p_content)
		_pending_tool_items.erase(p_id)

	if tool_use_info_dialog.visible and tool_use_info_dialog.tool_use_id == p_id:
		tool_use_info_dialog.update_output(p_content)


func _add_cancelled_to_chat() -> void:
	var label := Label.new()
	label.text = "Cancelled by user"
	label.modulate = Color(1, 1, 1, 0.5)
	chat_container.add_child(label)
	scroll_to_bottom()


func _show_tool_info(p_id: String, p_name: String, p_input, p_output) -> void:
	tool_use_info_dialog.popup_centered_ratio(0.6)
	tool_use_info_dialog.setup_tool_info(p_id, p_name, p_input, p_output)
