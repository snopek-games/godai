@tool
extends Control

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const DefaultTools = preload("res://addons/godai/tools/default_tools.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")

const UserChatScene = preload("res://addons/godai/ui/user_chat.tscn")
const AssistantChatScene = preload("res://addons/godai/ui/assistant_chat.tscn")
const ToolChatScene = preload("res://addons/godai/ui/tool_chat.tscn")
const ErrorChatScene = preload("res://addons/godai/ui/error_chat.tscn")

const ANTHROPIC_API_KEY_SETTING = "godai/anthropic_api_key"
const MCP_SERVER_PORT = 9080

@onready var chat_panel: PanelContainer = %ChatPanel
@onready var chat_scroll: ScrollContainer = %ChatScroll
@onready var chat_container: VBoxContainer = %ChatContainer
@onready var loading_label: Label = %LoadingLabel
@onready var prompt: TextEdit = %Prompt
@onready var submit_button: Button = %SubmitButton
@onready var clear_button: Button = %ClearButton
@onready var tool_use_info_dialog: AcceptDialog = %ToolUseInfoDialog

var claude_client: ClaudeClient
var current_chat: ClaudeClient.Chat
var tools: ToolManager = ToolManager.new()
var mcp_server: MCPServer

var _pending_tool_chats: Dictionary


func _ready() -> void:
	_update_panel_theme()

	claude_client = ClaudeClient.new()
	add_child(claude_client)

	if Engine.is_editor_hint():
		var settings: EditorSettings = EditorInterface.get_editor_settings()
		settings.settings_changed.connect(_update_from_editor_settings.bind(settings))
		_update_from_editor_settings(settings)

	clear_button.disabled = true

	DefaultTools.register(tools)
	claude_client.tools = tools

	mcp_server = MCPServer.new(tools)
	add_child(mcp_server)
	# @todo Make the transport configurable
	mcp_server.start_server(MCP_SERVER_PORT, MCPServer.Transport.WEBSOCKET)
	#mcp_server.start_server(MCP_SERVER_PORT, MCPServer.Transport.HTTP)


func show_panel() -> void:
	if not prompt.has_focus():
		prompt.grab_focus()


func _update_from_editor_settings(p_settings: EditorSettings) -> void:
	claude_client.api_key = p_settings.get_setting(ANTHROPIC_API_KEY_SETTING)


func _update_panel_theme() -> void:
	if chat_panel:
		var rich_label_normal_stylebox: StyleBox = get_theme_stylebox("normal", "RichTextLabel")
		chat_panel.add_theme_stylebox_override("panel", rich_label_normal_stylebox)


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_THEME_CHANGED:
			_update_panel_theme()


func _on_current_chat_message_added(p_msg: ClaudeClient.Message) -> void:
	var updated := false

	for content in p_msg.content:
		var data: Dictionary = content.data
		match content.get_type():
			"text":
				var text: String = data.get("text", "")
				if text:
					if p_msg.role == "user":
						var chat = UserChatScene.instantiate()
						chat_container.add_child(chat)
						chat.setup_user_chat(text)
					elif p_msg.role == "assistant":
						var chat = AssistantChatScene.instantiate()
						chat_container.add_child(chat)
						chat.setup_assistant_chat(text)
				updated = true

			"tool_use":
				var chat = ToolChatScene.instantiate()
				chat_container.add_child(chat)
				var tool_obj = tools.get_tool(data['name'])
				chat.setup_tool_chat(data['id'], data['name'], tool_obj.title if tool_obj else data['name'], data['input'])
				chat.info_requested.connect(_show_tool_info)
				_pending_tool_chats[data['id']] = chat
				updated = true

			"tool_result":
				var tool_use_id: String = data['tool_use_id']

				var chat = _pending_tool_chats.get(tool_use_id)
				if chat:
					chat.set_tool_output(data['content'])
					_pending_tool_chats.erase(tool_use_id)

				if tool_use_info_dialog.visible and tool_use_info_dialog.tool_use_id == tool_use_id:
					tool_use_info_dialog.update_output(data['content'])

	if updated:
		# Scroll to the bottom
		await get_tree().process_frame
		var scrollbar: VScrollBar = chat_scroll.get_v_scroll_bar()
		chat_scroll.scroll_vertical = scrollbar.max_value


func _show_tool_info(p_id: String, p_name: String, p_input, p_output) -> void:
	tool_use_info_dialog.popup_centered_ratio(0.6)
	tool_use_info_dialog.setup_tool_info(p_id, p_name, p_input, p_output)


func _start_new_chat() -> void:
	_stop_current_chat()

	# @todo We can give Claude some context here if we want
	current_chat = ClaudeClient.Chat.new()
	current_chat.message_added.connect(_on_current_chat_message_added)

	clear_button.disabled = false


func _stop_current_chat() -> void:
	if current_chat:
		current_chat.message_added.disconnect(_on_current_chat_message_added)
	current_chat = null

	clear_button.disabled = true
	loading_label.visible = false


func _submit_message() -> void:
	var content := prompt.text
	prompt.clear()

	if current_chat == null:
		_start_new_chat()
	current_chat.add_message(ClaudeClient.Message.new("user", content))

	submit_button.disabled = true
	loading_label.visible = true
	prompt.editable = false

	var resp: ClaudeClient.Response = await claude_client.submit_chat(current_chat).completed

	submit_button.disabled = false
	loading_label.visible = false
	prompt.editable = true

	if resp.is_error():
		var chat = ErrorChatScene.instantiate()
		chat_container.add_child(chat)
		var error := resp.get_error()
		chat.setup_error_chat(error.type, error.message)
		return


func _on_prompt_gui_input(p_event: InputEvent) -> void:
	if p_event is InputEventKey:
		var key_event: InputEventKey = p_event
		if key_event.keycode == KEY_ENTER and key_event.pressed and not key_event.ctrl_pressed and not key_event.shift_pressed and not submit_button.disabled:
			_submit_message()
			accept_event()


func _on_submit_button_pressed() -> void:
	_submit_message()


func _on_clear_button_pressed() -> void:
	_stop_current_chat()
	_pending_tool_chats.clear()
	for chat in chat_container.get_children():
		chat.queue_free()
	prompt.clear()
