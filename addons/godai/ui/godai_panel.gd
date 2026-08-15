@tool
extends Control

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const EvalRun = preload("res://addons/godai/eval_run.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")
const DefaultToolsLoader = preload("res://addons/godai/tools/default/loader.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")
const ToolUseAuthDialog = preload("res://addons/godai/ui/tool_use_auth_dialog.gd")

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
@onready var tool_use_auth_dialog: ToolUseAuthDialog = %ToolUseAuthDialog

signal chat_started(chat: ClaudeClient.Chat, request: ClaudeClient.Request)

var claude_client: ClaudeClient
var current_chat: ClaudeClient.Chat
var tools: ToolManager = ToolManager.new()
var tool_auth: ToolAuth = ToolAuth.new()
var mcp_server: MCPServer

var _pending_tool_chats: Dictionary
var _pending_auth_requests: Array[ToolAuth.Request]
var _shown_auth_request: ToolAuth.Request
var _updating_auth_queue := false
var _current_request: ClaudeClient.Request

var _eval_run: EvalRun

var _mcp_instance_id: String
var _mcp_instance_secret: String
var _mcp_transport: MCPServer.Transport = GodaiEditorSettings.MCP_TRANSPORT_DEFAULT
var _mcp_base_port: int = GodaiEditorSettings.MCP_BASE_PORT_DEFAULT
var _mcp_port_count: int = GodaiEditorSettings.MCP_PORT_COUNT_DEFAULT

const MCP_INSTANCE_USER_PATH := "godai/instances"
const MCP_INSTANCE_PROJECT_FILE := ".godot/godai-instance.json"


func _ready() -> void:
	_update_panel_theme()

	claude_client = ClaudeClient.new()
	add_child(claude_client)

	clear_button.disabled = true

	DefaultToolsLoader.load_default_tools(tools)
	claude_client.tools = tools
	claude_client.tool_use_authorizer = _authorize_tool_use

	tool_use_auth_dialog.tool_use_allowed.connect(_on_tool_use_allowed)
	tool_use_auth_dialog.tool_use_denied.connect(_on_tool_use_denied)

	# Generate MCP instance ID and secret.
	const MCP_TOKEN_LENGTH := 32
	_mcp_instance_id = _generate_string(MCP_TOKEN_LENGTH)
	_mcp_instance_secret = _generate_string(MCP_TOKEN_LENGTH)

	mcp_server = MCPServer.new(tools, _mcp_instance_secret)
	mcp_server.tool_use_authorizer = _authorize_tool_use
	add_child(mcp_server)
	mcp_server.server_state_changed.connect(_on_mcp_server_state_changed)
	mcp_server.client_state_changed.connect(_on_mcp_client_state_changed)
	mcp_server.update_available_changed.connect(_on_mcp_update_available_changed)
	mcp_server.tool_use_requested.connect(_add_tool_use_to_chat)
	mcp_server.tool_use_completed.connect(_add_tool_result_to_chat)

	if Engine.is_editor_hint():
		var settings: EditorSettings = EditorInterface.get_editor_settings()
		settings.settings_changed.connect(_update_from_editor_settings)
		_update_from_editor_settings()

	_update_mcp_status_bar()
	_start_mcp()

	if EvalRun.should_run():
		_eval_run = EvalRun.start(self)


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


func _update_from_editor_settings() -> void:
	claude_client.api_key = GodaiEditorSettings.get_anthropic_api_key()
	claude_client.model = GodaiEditorSettings.get_anthropic_model()
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

	# Must go first: it cancels the request before releasing the approvals that
	# would otherwise let the client resume.
	_stop_current_chat()

	# "For this session" lasts as long as the client stays connected.
	tool_auth.clear_session()
	if p_client_state == MCPServer.ClientState.CONNECTED:
		prompt_bar.visible = false
	else:
		prompt_bar.visible = true


func _on_mcp_update_available_changed(_p_update_available: Dictionary) -> void:
	_update_mcp_status_bar()


func _update_mcp_status_bar() -> void:
	var server_state: MCPServer.ServerState = mcp_server.get_server_state()
	var client_state: MCPServer.ClientState = mcp_server.get_client_state()

	if server_state in [MCPServer.ServerState.STOPPED, MCPServer.ServerState.ERROR]:
		start_mcp_button.visible = true
		stop_mcp_button.visible = false
	else:
		start_mcp_button.visible = false
		stop_mcp_button.visible = true

	var tooltip := ""

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

			var update_available := mcp_server.get_update_available()
			if update_available.size() > 0:
				status += " (godai %s is available)" % update_available['latest_version']
				tooltip = "Run 'godai self-update' in a terminal to install godai %s." % update_available['latest_version']

		mcp_status_label.text = status

	mcp_status_label.tooltip_text = tooltip


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


## Decides whether a tool may run, asking the user when we have no standing
## answer. Both the MCP server and the API client call this through their
## `tool_use_authorizer` hook.
func _authorize_tool_use(p_name: String, p_input) -> ToolAuth.Request:
	var tool_obj := tools.get_tool(p_name)
	if not ToolAuth.needs_authorization(tool_obj):
		return ToolAuth.Request.resolved(p_name, p_input, true)

	var headless := DisplayServer.get_name() == "headless"
	if headless and p_name in ToolAuth.HEADLESS_ALWAYS_ALLOWED_TOOLS:
		return ToolAuth.Request.resolved(p_name, p_input, true)

	match tool_auth.get_decision(p_name):
		ToolAuth.Decision.ALLOW:
			return ToolAuth.Request.resolved(p_name, p_input, true)
		ToolAuth.Decision.DENY:
			return ToolAuth.Request.resolved(p_name, p_input, false)

	if GodaiEditorSettings.get_auto_approve_tools():
		return ToolAuth.Request.resolved(p_name, p_input, true)

	if headless:
		push_warning("Denying use of the '%s' tool: running headless, and %s is not set."
			% [p_name, GodaiEditorSettings.AUTO_APPROVE_TOOLS_ENV])
		return ToolAuth.Request.resolved(p_name, p_input, false)

	var request := ToolAuth.Request.new(p_name, p_input)
	request.completed.connect(_on_auth_request_completed)
	_pending_auth_requests.push_back(request)
	_update_auth_queue()
	return request


func _on_auth_request_completed(_p_allowed: bool) -> void:
	_update_auth_queue()


func _update_auth_queue() -> void:
	# Resolving re-enters here, so this has to be the only loop walking the queue.
	if _updating_auth_queue:
		return
	_updating_auth_queue = true

	while _pending_auth_requests.size() > 0:
		var request: ToolAuth.Request = _pending_auth_requests[0]

		# The MCP server resolves these on its own when they time out.
		if request.is_done():
			_pending_auth_requests.pop_front()
			continue

		# Check tool_auth again, in case we recorded an allow/deny for this tool
		# earlier in the queue.
		var decision := tool_auth.get_decision(request.tool_name)
		if decision == ToolAuth.Decision.ASK:
			break

		_pending_auth_requests.pop_front()
		request.resolve(decision == ToolAuth.Decision.ALLOW)

	if _pending_auth_requests.size() > 0:
		var request: ToolAuth.Request = _pending_auth_requests[0]
		if _shown_auth_request != request:
			_shown_auth_request = request
			tool_use_auth_dialog.setup_tool_use_auth_dialog(request.tool_name, request.input)
			tool_use_auth_dialog.popup_centered()
	else:
		_shown_auth_request = null
		tool_use_auth_dialog.hide()

	_updating_auth_queue = false


func _resolve_current_auth_request(p_type: ToolUseAuthDialog.AllowDenyType, p_allowed: bool) -> void:
	var request := _shown_auth_request
	if request == null:
		return

	match p_type:
		ToolUseAuthDialog.AllowDenyType.TOOL_FOR_SESSION:
			tool_auth.set_tool_for_session(request.tool_name, p_allowed)
		ToolUseAuthDialog.AllowDenyType.TOOL_ALWAYS:
			tool_auth.set_tool_always(request.tool_name, p_allowed)
		ToolUseAuthDialog.AllowDenyType.ALL_FOR_SESSION:
			# Only makes sense for allow - denying all for this session isn't a thing.
			if p_allowed:
				tool_auth.set_allow_all_for_session(true)

	request.resolve(p_allowed)


func _on_tool_use_allowed(p_type: ToolUseAuthDialog.AllowDenyType) -> void:
	_resolve_current_auth_request(p_type, true)


func _on_tool_use_denied(p_type: ToolUseAuthDialog.AllowDenyType) -> void:
	_resolve_current_auth_request(p_type, false)


## Denies everything still waiting, for when there's no longer anyone to answer
## for (the MCP client went away, or the chat was cleared).
func _cancel_pending_auth_requests() -> void:
	var pending := _pending_auth_requests.duplicate()
	_pending_auth_requests.clear()
	_shown_auth_request = null
	tool_use_auth_dialog.hide()

	for request in pending:
		request.resolve(false)


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

	# Cancel before denying the pending approvals: denying them lets the client
	# carry on, and without the cancel it would submit a follow-up request for
	# the chat we're throwing away. Clearing the member first, because cancelling
	# resolves the request and resumes whoever is awaiting it right here.
	if _current_request:
		var request := _current_request
		_current_request = null
		request.cancel()

	_pending_tool_chats.clear()
	_cancel_pending_auth_requests()

	for chat in chat_container.get_children():
		chat.queue_free()

	prompt.clear()
	prompt.editable = true
	submit_button.disabled = false
	clear_button.disabled = true
	loading_label.visible = false


func submit_prompt(p_text: String) -> void:
	prompt.text = p_text
	_submit_message()


func _submit_message() -> void:
	var content := prompt.text
	prompt.clear()

	if current_chat == null:
		_start_new_chat()
	current_chat.add_message(ClaudeClient.Message.new("user", content))

	submit_button.disabled = true
	loading_label.visible = true
	prompt.editable = false

	var request := claude_client.submit_chat(current_chat)
	_current_request = request

	chat_started.emit(current_chat, request)

	var resp: ClaudeClient.Response = await request.completed

	# The chat was cleared (or restarted) while we were waiting, so there's
	# nothing left to report this into.
	if _current_request != request:
		return
	_current_request = null

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
