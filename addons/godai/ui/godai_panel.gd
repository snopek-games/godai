@tool
extends Control

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const DefaultToolsLoader = preload("res://addons/godai/tools/default/loader.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")

const UserChatScene = preload("res://addons/godai/ui/user_chat.tscn")
const AssistantChatScene = preload("res://addons/godai/ui/assistant_chat.tscn")
const ToolChatScene = preload("res://addons/godai/ui/tool_chat.tscn")
const ErrorChatScene = preload("res://addons/godai/ui/error_chat.tscn")

@onready var mcp_status_label: Label = %MCPStatusLabel
@onready var start_mcp_button: Button = %StartMCPButton
@onready var stop_mcp_button: Button = %StopMCPButton
@onready var mcp_stopping_timer: Timer = %MCPStoppingTimer
@onready var chat_panel: PanelContainer = %ChatPanel
@onready var chat_scroll: ScrollContainer = %ChatScroll
@onready var chat_container: VBoxContainer = %ChatContainer
@onready var loading_label: Label = %LoadingLabel
@onready var prompt_bar: Control = %PromptBar
@onready var prompt: TextEdit = %Prompt
@onready var submit_button: Button = %SubmitButton
@onready var clear_button: Button = %ClearButton
@onready var tool_use_info_dialog: AcceptDialog = %ToolUseInfoDialog

var claude_client: ClaudeClient
var current_chat: ClaudeClient.Chat
var tools: ToolManager = ToolManager.new()
var mcp_server: MCPServer

var _pending_tool_chats: Dictionary
var _mcp_instance_id: String
var _mcp_instance_secret: String
var _mcp_transport: MCPServer.Transport = GodaiEditorSettings.MCP_TRANSPORT_DEFAULT
var _mcp_base_port: int = GodaiEditorSettings.MCP_BASE_PORT_DEFAULT
var _mcp_port_count: int = GodaiEditorSettings.MCP_PORT_COUNT_DEFAULT

const MCP_INSTANCE_USER_PATH := "godai-mcp/instances"
const MCP_INSTANCE_PROJECT_FILE := ".godot/godai-mcp-instance.json"


func _ready() -> void:
	_update_panel_theme()

	claude_client = ClaudeClient.new()
	add_child(claude_client)

	clear_button.disabled = true

	DefaultToolsLoader.load_default_tools(tools)
	claude_client.tools = tools

	# Generate MCP instance ID and secret.
	const MCP_TOKEN_LENGTH := 32
	_mcp_instance_id = _generate_string(MCP_TOKEN_LENGTH)
	_mcp_instance_secret = _generate_string(MCP_TOKEN_LENGTH)

	mcp_server = MCPServer.new(tools, _mcp_instance_secret)
	add_child(mcp_server)
	mcp_server.server_state_changed.connect(_on_mcp_server_state_changed)
	mcp_server.client_state_changed.connect(_on_mcp_client_state_changed)
	mcp_server.tool_use_requested.connect(_add_tool_use_to_chat)
	mcp_server.tool_use_completed.connect(_add_tool_result_to_chat)

	if Engine.is_editor_hint():
		var settings: EditorSettings = EditorInterface.get_editor_settings()
		settings.settings_changed.connect(_update_from_editor_settings.bind(settings))
		_update_from_editor_settings(settings)

	_update_mcp_status_bar()
	_start_mcp()


func _generate_string(p_len: int) -> String:
	const CHARS := "abcdefghijklmnopqrstuvwxyz0123456789"

	var rng := RandomNumberGenerator.new()
	rng.randomize()

	var ret: String
	for i in range(p_len):
		ret += CHARS[rng.randi_range(0, CHARS.length() - 1)]
	return ret


func show_panel() -> void:
	if not prompt.has_focus():
		prompt.grab_focus()


func _update_from_editor_settings(p_settings: EditorSettings) -> void:
	claude_client.api_key = p_settings.get_setting(GodaiEditorSettings.ANTHROPIC_API_KEY_SETTING)
	mcp_server.skip_secret_check = GodaiEditorSettings.get_mcp_skip_secret_check()
	_mcp_transport = GodaiEditorSettings.get_mcp_transport() as MCPServer.Transport
	_mcp_base_port = GodaiEditorSettings.get_mcp_base_port()
	_mcp_port_count = GodaiEditorSettings.get_mcp_port_count()


func _update_panel_theme() -> void:
	if chat_panel:
		var rich_label_normal_stylebox: StyleBox = get_theme_stylebox("normal", "RichTextLabel")
		chat_panel.add_theme_stylebox_override("panel", rich_label_normal_stylebox)


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_THEME_CHANGED:
			_update_panel_theme()

		NOTIFICATION_EXIT_TREE:
			_delete_mcp_instance_user_file()
			_release_project_for_mcp_instance()


func _on_mcp_server_state_changed(p_server_state: MCPServer.ServerState) -> void:
	_update_mcp_status_bar()

	if p_server_state == MCPServer.ServerState.STOPPED:
		mcp_stopping_timer.stop()
		_delete_mcp_instance_user_file()
		_release_project_for_mcp_instance()


func _on_mcp_client_state_changed(p_client_state: MCPServer.ClientState) -> void:
	_update_mcp_status_bar()

	_stop_current_chat()
	if p_client_state == MCPServer.ClientState.CONNECTED:
		prompt_bar.visible = false
	else:
		prompt_bar.visible = true


func _update_mcp_status_bar() -> void:
	var server_state: MCPServer.ServerState = mcp_server.get_server_state()
	var client_state: MCPServer.ClientState = mcp_server.get_client_state()

	if server_state in [MCPServer.ServerState.STOPPED, MCPServer.ServerState.ERROR]:
		start_mcp_button.visible = true
		stop_mcp_button.visible = false
	else:
		start_mcp_button.visible = false
		stop_mcp_button.visible = true

	if server_state == MCPServer.ServerState.STOPPED:
		mcp_status_label.text = "MCP server stopped."
	elif server_state == MCPServer.ServerState.STOPPING:
		mcp_status_label.text = "MCP server stopping..."
	elif server_state == MCPServer.ServerState.ERROR:
		mcp_status_label.text = "MCP server was unable to start."
	else:
		var transport: String = "WebSocket" if mcp_server.get_transport() == MCPServer.Transport.WEBSOCKET else "HTTP"
		var status: String = "MCP server listening on port %d (%s transport)." % [mcp_server.get_port(), transport]

		if client_state == MCPServer.ClientState.NOT_CONNECTED:
			status += " Not connected."
		else:
			var client_info := mcp_server.get_client_info()
			if client_info.size() > 0:
				status += " Connected to %s version %s." % [client_info['name'], client_info['version']]
			else:
				status += " Connected."

		mcp_status_label.text = status


static func _get_project_path() -> String:
	return ProjectSettings.globalize_path("res://").simplify_path()

func _get_mcp_instance_user_file() -> String:
	var path := OS.get_cache_dir() + "/" + MCP_INSTANCE_USER_PATH
	if not DirAccess.dir_exists_absolute(path):
		DirAccess.make_dir_recursive_absolute(path)
	return path + "/" + _mcp_instance_id + ".json"


func _get_mcp_instance_project_file() -> String:
	return _get_project_path() + "/" + MCP_INSTANCE_PROJECT_FILE


static func _is_pid_running(p_pid: int) -> bool:
	if p_pid <= 0:
		return false

	if OS.get_name() == "Windows":
		var output := []
		OS.execute("tasklist", ["/FI", "PID eq %d" % p_pid, "/NH", "/FO", "CSV"], output)
		if output.is_empty():
			return false
		# A match is one CSV row: "image.exe","1234","Console","1","12,345 K"
		# PID is always the 2nd column. Split on the quote-comma-quote delimiter.
		var fields: PackedStringArray = output[0].strip_edges().trim_prefix("\"").split("\",\"")
		return fields.size() > 1 and fields[1] == str(p_pid)

	if OS.get_name() == "Linux":
		return DirAccess.dir_exists_absolute("/proc/%d" % p_pid)

	# MacOS or other UNIX-y systems.
	var output := []
	OS.execute("ps", ["-p", str(p_pid), "-o", "pid="], output)
	return not output.is_empty() and output[0].strip_edges() == str(p_pid)


func _claim_project_for_mcp_instance() -> Error:
	var instance_file_path = _get_mcp_instance_project_file()

	# If there's an existing project instance file, check if it's valid.
	if FileAccess.file_exists(instance_file_path):
		var content := FileAccess.get_file_as_string(instance_file_path)
		var data = JSON.parse_string(content)
		if data is Dictionary:
			var old_instance_id = data.get("instance_id", "")
			var old_pid = int(data.get("pid", 0))

			# If this is our instance, then we're good!
			if old_instance_id == _mcp_instance_id and old_pid == OS.get_process_id():
				return OK

			# Check if the other instance is still running, and if so, abort.
			if _is_pid_running(old_pid):
				return ERR_ALREADY_IN_USE

		# If we made it this far, then the old instance is invalid, so remove it.
		var err = DirAccess.remove_absolute(instance_file_path)
		if err != OK:
			return err

	# Write a new project instance file.
	var f := FileAccess.open(instance_file_path, FileAccess.WRITE)
	if not f:
		return FileAccess.get_open_error()

	var data := {
		instance_id = _mcp_instance_id,
		pid = OS.get_process_id(),
	}
	f.store_string(JSON.stringify(data))
	f.flush()

	return OK


func _release_project_for_mcp_instance() -> void:
	var instance_file_path = _get_mcp_instance_project_file()

	if FileAccess.file_exists(instance_file_path):
		var content := FileAccess.get_file_as_string(instance_file_path)
		var data = JSON.parse_string(content)
		if data is Dictionary:
			var old_instance_id = data.get("instance_id", "")
			var old_pid = int(data.get("pid", 0))
			# This is our instance file, so delete it.
			if old_instance_id == _mcp_instance_id and old_pid == OS.get_process_id():
				DirAccess.remove_absolute(instance_file_path)


func _write_mcp_instance_user_file() -> Error:
	var instance_file_path = _get_mcp_instance_user_file()

	var f := FileAccess.open(instance_file_path, FileAccess.WRITE)
	if not f:
		return FileAccess.get_open_error()

	var data := {
		instance_id = _mcp_instance_id,
		pid = OS.get_process_id(),
		project_path = _get_project_path(),
		secret = _mcp_instance_secret,
		port = mcp_server.get_port(),
	}
	f.store_string(JSON.stringify(data))
	f.flush()

	return OK


func _delete_mcp_instance_user_file() -> void:
	var instance_file_path = _get_mcp_instance_user_file()
	if FileAccess.file_exists(instance_file_path):
		DirAccess.remove_absolute(instance_file_path)


func _start_mcp() -> void:
	var err: Error

	# Claim this project for this MCP instance.
	err = _claim_project_for_mcp_instance()
	if err != OK:
		var msg: String = "Cannot start MCP server: "
		if err == ERR_ALREADY_IN_USE:
			msg += "another Godot editor is already running an MCP server for this project"
		else:
			msg += "error claiming project: " + error_string(err)
		_add_error_to_chat(msg)
		return

	# Actually start the MCP server.
	err = mcp_server.start_server(_mcp_base_port, _mcp_port_count, _mcp_transport)
	if err != OK:
		_add_error_to_chat("Cannot start MCP server: " + error_string(err))
		return

	# Write the instance file.
	err = _write_mcp_instance_user_file()
	if err != OK:
		_add_error_to_chat("Cannot start MCP server: error writing instance file: " + error_string(err))
		return


func _on_start_mcp_button_pressed() -> void:
	_start_mcp()


func _on_stop_mcp_button_pressed() -> void:
	mcp_server.stop_server()
	if mcp_server.get_server_state() == MCPServer.ServerState.STOPPING:
		mcp_stopping_timer.start()


func _on_mcp_stopping_timer_timeout() -> void:
	if mcp_server.get_server_state() == MCPServer.ServerState.STOPPING:
		mcp_server.stop_server(true)


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

				_scroll_chat_to_bottom()

			"tool_use":
				_add_tool_use_to_chat(data['id'], data['name'], data['input'])

			"tool_result":
				_add_tool_result_to_chat(data['tool_use_id'], data['content'])


func _scroll_chat_to_bottom() -> void:
	await get_tree().process_frame
	var scrollbar: VScrollBar = chat_scroll.get_v_scroll_bar()
	chat_scroll.scroll_vertical = scrollbar.max_value


func _add_tool_use_to_chat(p_id: String, p_name: String, p_input: Dictionary) -> void:
	var chat = ToolChatScene.instantiate()
	chat_container.add_child(chat)
	var tool_obj = tools.get_tool(p_name)
	chat.setup_tool_chat(p_id, p_name, tool_obj.title if tool_obj else p_name, p_input)
	chat.info_requested.connect(_show_tool_info)
	_pending_tool_chats[p_id] = chat
	_scroll_chat_to_bottom()


func _add_tool_result_to_chat(p_id: String, p_content) -> void:
	var chat = _pending_tool_chats.get(p_id)
	if chat:
		chat.set_tool_output(p_content)
		_pending_tool_chats.erase(p_id)

	if tool_use_info_dialog.visible and tool_use_info_dialog.tool_use_id == p_id:
		tool_use_info_dialog.update_output(p_content)


func _add_error_to_chat(p_msg: String) -> void:
	var chat = ErrorChatScene.instantiate()
	chat_container.add_child(chat)
	chat.setup_error_chat(p_msg)


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

	_pending_tool_chats.clear()

	for chat in chat_container.get_children():
		chat.queue_free()

	prompt.clear()
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
		var error := resp.get_error()
		_add_error_to_chat("Error (%s): %s" % [error.type, error.message])
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
