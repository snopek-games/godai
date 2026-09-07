@tool
extends PanelContainer

const Chat = preload("res://addons/godai/chat/chat.gd")
const ChatClient = preload("res://addons/godai/chat/client.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const LoadingLabel = preload("res://addons/godai/ui/loading_label.gd")
const WelcomeNote = preload("res://addons/godai/ui/welcome_note.gd")

const UserChatScene = preload("res://addons/godai/ui/user_chat.tscn")
const AssistantChatScene = preload("res://addons/godai/ui/assistant_chat.tscn")
const ToolChatScene = preload("res://addons/godai/ui/tool_chat.tscn")
const ErrorChatScene = preload("res://addons/godai/ui/error_chat.tscn")

@onready var chat_scroll: ScrollContainer = %ChatScroll
@onready var chat_container: VBoxContainer = %ChatContainer
@onready var loading_label: LoadingLabel = %LoadingLabel
@onready var tool_use_info_dialog: AcceptDialog = %ToolUseInfoDialog
@onready var welcome_note: WelcomeNote = %WelcomeNote

var tools: ToolManager

var _pending_tool_items: Dictionary
var _scroll_queued := false


func _ready() -> void:
	if is_part_of_edited_scene():
		return

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


func show_chat(p_chat: Chat) -> void:
	clear()
	welcome_note.visible = p_chat == null
	if p_chat:
		for msg in p_chat.messages:
			show_message(msg)


func show_message(p_msg: Chat.Message) -> void:
	welcome_note.visible = false
	for content in p_msg.content:
		if content is Chat.TextContent:
			if content.text:
				if p_msg.role == Chat.Role.USER:
					if content.text == ChatClient.CANCELLED_MESSAGE:
						_add_cancelled_to_chat()
					else:
						var chat = UserChatScene.instantiate()
						chat_container.add_child(chat)
						chat.setup_user_chat(content.text)
				elif p_msg.role == Chat.Role.ASSISTANT:
					var chat = AssistantChatScene.instantiate()
					chat_container.add_child(chat)
					chat.setup_assistant_chat(content.text)

			scroll_to_bottom()

		elif content is Chat.ToolUseContent:
			_add_tool_use_to_chat(content.id, content.name, content.input)

		elif content is Chat.ToolResultContent:
			_add_tool_result_to_chat(content.tool_use_id, content.content, content.is_error)


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


func _add_tool_result_to_chat(p_id: String, p_content, p_is_error: bool) -> void:
	var chat = _pending_tool_items.get(p_id)
	if chat:
		chat.set_tool_output(p_content, p_is_error)
		_pending_tool_items.erase(p_id)

	if tool_use_info_dialog.visible and tool_use_info_dialog.tool_use_id == p_id:
		tool_use_info_dialog.update_output(p_content, p_is_error)


func _add_cancelled_to_chat() -> void:
	var label := Label.new()
	label.text = "Cancelled by user"
	label.modulate = Color(1, 1, 1, 0.5)
	chat_container.add_child(label)
	scroll_to_bottom()


func _show_tool_info(p_id: String, p_name: String, p_input, p_output, p_is_error: bool) -> void:
	tool_use_info_dialog.popup_centered_ratio(0.6)
	tool_use_info_dialog.setup_tool_info(p_id, p_name, p_input, p_output, p_is_error, tools.get_tool(p_name))
