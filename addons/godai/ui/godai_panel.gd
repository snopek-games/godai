@tool
extends Control

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const Utils = preload("res://addons/godai/utils.gd")

const Chat = preload("res://addons/godai/chat/chat.gd")
const ChatClient = preload("res://addons/godai/chat/client.gd")
const Provider = preload("res://addons/godai/chat/provider.gd")
const EvalRun = preload("res://addons/godai/eval_run.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")
const DefaultToolsLoader = preload("res://addons/godai/tools/default/loader.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")
const MCPInstance = preload("res://addons/godai/mcp/mcp_instance.gd")
const ChatSessionStore = preload("res://addons/godai/chat/chat_session_store.gd")
const ExternalSessionRecorder = preload("res://addons/godai/chat/external_session_recorder.gd")
const ToolUseAuthDialog = preload("res://addons/godai/ui/tool_use_auth_dialog.gd")
const ToolAuthQueue = preload("res://addons/godai/ui/tool_auth_queue.gd")
const ChatView = preload("res://addons/godai/ui/chat_view.gd")
const WelcomeNote = preload("res://addons/godai/ui/welcome_note.gd")
const SettingsDialog = preload("res://addons/godai/ui/settings_dialog.gd")
const ModelCatalog = preload("res://addons/godai/chat/model_catalog.gd")
const Profiles = preload("res://addons/godai/chat/profiles.gd")

const InfoIcon = preload("res://addons/godai/ui/icons/status_info.svg")
const WarningIcon = preload("res://addons/godai/ui/icons/status_warning.svg")
const ErrorIcon = preload("res://addons/godai/ui/icons/status_error.svg")
const SuccessIcon = preload("res://addons/godai/ui/icons/status_success.svg")

@onready var expand_sidebar_button: Button = %ExpandSidebarButton
@onready var sidebar_container: Control = %SidebarContainer
@onready var session_list: ItemList = %SessionList
@onready var mcp_button: Button = %MCPButton
@onready var settings_button: Button = %SettingsButton
@onready var mcp_error_message: Control = %MCPErrorMessage
@onready var not_configured_message: Control = %NotConfiguredMessage
@onready var offline_message: Control = %OfflineMessage
@onready var settings_dialog: SettingsDialog = %SettingsDialog
@onready var mcp_stopping_timer: Timer = %MCPStoppingTimer
@onready var mcp_dialog: AcceptDialog = %MCPDialog
@onready var start_mcp_button: Button = mcp_dialog.add_button("Start MCP Server", true)
@onready var stop_mcp_button: Button = mcp_dialog.add_button("Stop MCP Server", true)
@onready var chat_view: ChatView = %ChatView
@onready var welcome_note: WelcomeNote = chat_view.welcome_note
@onready var prompt_bar: Control = %PromptBar
@onready var prompt: TextEdit = %Prompt
@onready var submit_button: Button = %SubmitButton
@onready var cancel_button: Button = %CancelButton
@onready var clear_button: Button = %ClearButton
@onready var tool_use_auth_dialog: ToolUseAuthDialog = %ToolUseAuthDialog
@onready var cancel_chat_dialog: ConfirmationDialog = %CancelChatDialog
@onready var _default_cancel_chat_text: String = cancel_chat_dialog.dialog_text

signal chat_started(chat: Chat, request: ChatClient.Request)
signal _current_request_changed

var chat_client: ChatClient
var tools: ToolManager = ToolManager.new()
var tool_auth: ToolAuth = ToolAuth.new()
var mcp_server: MCPServer

var _session_store := ChatSessionStore.new()
var _current_session: ChatSessionStore.ChatSession
var _current_request: ChatClient.Request
var _pending_cancel_action: Callable
var _cancel_confirmed := false
var _headless := DisplayServer.get_name() == "headless"
var _api_configured := true
var _online := true

var _resume_retry_interval := 0.25
var _resume_retry_timeout := 30.0

var _external_recorder := ExternalSessionRecorder.new(_session_store)

var _tool_auth_queue: ToolAuthQueue

var _eval_run: EvalRun

var _model_catalog := ModelCatalog.new(Profiles.models_dev_ids())

var _mcp_instance := MCPInstance.new()
var _mcp_transport: MCPServer.Transport = GodaiEditorSettings.MCP_TRANSPORT_DEFAULT
var _mcp_base_port: int = GodaiEditorSettings.MCP_BASE_PORT_DEFAULT
var _mcp_port_count: int = GodaiEditorSettings.MCP_PORT_COUNT_DEFAULT

const NEW_CHAT_SESSION_NAME := "<new>"

const RESUME_FILE := ".godot/godai-resume-session.json"
const RESUME_MAX_AGE_SECONDS := 300

const FIX_BOTH_TEXT := "In order to use this chat box, you'll need to configure an LLM API and go online:"
const FIX_NOT_CONFIGURED_TEXT := "In order to use this chat box, you'll need to configure an LLM API:"
const FIX_OFFLINE_TEXT := "In order to use this chat box, you'll need to go online:"
const MCP_CONNECTED_CANCEL_TEXT := "The current chat is still in progress. The MCP client's tool calls will wait for it to finish. Would you like to cancel it now?"
const PROMPT_PLACEHOLDER_TEXT := "Type your prompt here!"
const PROMPT_UNAVAILABLE_TEXT := "Cannot chat with MCP or CLI sessions."


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	chat_client = ChatClient.new()
	add_child(chat_client)

	sidebar_container.visible = false

	DefaultToolsLoader.load_default_tools(tools)
	chat_view.tools = tools
	_tool_auth_queue = ToolAuthQueue.new(tools, tool_auth, tool_use_auth_dialog,
		func (): return _current_request != null, _current_request_changed)
	chat_client.tools = tools
	chat_client.tool_use_authorizer = _tool_auth_queue.authorize
	chat_client.tool_use_completed.connect(_on_chat_tool_use_completed)

	start_mcp_button.pressed.connect(_on_start_mcp_button_pressed)
	stop_mcp_button.pressed.connect(_on_stop_mcp_button_pressed)

	mcp_server = MCPServer.new(tools, _mcp_instance.secret)
	mcp_server.tool_use_authorizer = _tool_auth_queue.authorize_when_idle
	add_child(mcp_server)
	mcp_server.server_state_changed.connect(_on_mcp_server_state_changed)
	mcp_server.client_state_changed.connect(_on_mcp_client_state_changed)
	mcp_server.update_available_changed.connect(_on_mcp_update_available_changed)
	mcp_server.tool_use_requested.connect(_on_mcp_tool_use_requested)
	mcp_server.tool_use_completed.connect(_on_mcp_tool_use_completed)

	_external_recorder.session_started.connect(_on_external_session_started)
	_external_recorder.session_dropped.connect(_on_external_session_dropped)
	_external_recorder.message_recorded.connect(_on_external_message_recorded)

	settings_dialog.closed.connect(_on_settings_dialog_closed)
	welcome_note.settings_requested.connect(_on_settings_button_pressed)
	welcome_note.go_online_requested.connect(_on_go_online_requested)

	add_child(_model_catalog)
	settings_dialog.catalog = _model_catalog

	if Engine.is_editor_hint():
		tools.is_busy = EditorInterface.get_resource_filesystem().is_scanning
		settings_button.icon = EditorInterface.get_editor_theme().get_icon("Tools", "EditorIcons")

		_model_catalog.set_cache_dir(EditorInterface.get_editor_paths().get_cache_dir().path_join("godai"))
		_model_catalog.updated.connect(_update_from_editor_settings)

		var settings: EditorSettings = EditorInterface.get_editor_settings()
		settings.settings_changed.connect(_update_from_editor_settings)
		_update_from_editor_settings()

	_model_catalog.load_from_disk()
	if Engine.is_editor_hint() and GodaiEditorSettings.is_network_online():
		_model_catalog.refresh_if_stale()

	_update_status_messages()
	_update_prompt_bar()
	_load_chat_sessions()
	_update_mcp_status()
	_start_mcp()
	await _resume_restarted_chat()

	# This should always happen last, so everything is ready.
	if EvalRun.should_run():
		_eval_run = EvalRun.start(self)


func show_panel() -> void:
	if not prompt.has_focus():
		prompt.grab_focus()
	chat_view.scroll_to_bottom.call_deferred()


func _update_from_editor_settings() -> void:
	var provider_name := GodaiEditorSettings.get_api_provider()
	var url := GodaiEditorSettings.get_api_url()
	var model := GodaiEditorSettings.get_api_model()
	chat_client.provider = ChatClient.create_provider(provider_name, url, GodaiEditorSettings.get_api_key(), model)
	chat_client.model_info = _model_catalog.get_model(Profiles.models_dev_id(Profiles.find(provider_name, url)), model)
	chat_client.effort = GodaiEditorSettings.get_api_effort()
	chat_client.thinking = GodaiEditorSettings.get_api_thinking()
	chat_client.budget_tokens = GodaiEditorSettings.get_api_budget_tokens()
	mcp_server.skip_secret_check = GodaiEditorSettings.get_mcp_skip_secret_check()
	_mcp_transport = GodaiEditorSettings.get_mcp_transport() as MCPServer.Transport
	_mcp_base_port = GodaiEditorSettings.get_mcp_base_port()
	_mcp_port_count = GodaiEditorSettings.get_mcp_port_count()
	_set_chat_availability(GodaiEditorSettings.is_api_configured(), GodaiEditorSettings.can_api_connect())


func _set_chat_availability(p_api_configured: bool, p_online: bool) -> void:
	_api_configured = p_api_configured
	_online = p_online
	_update_status_messages()
	_update_prompt_bar()


func _update_status_messages() -> void:
	var editor_chat := _current_session != null and not _current_session.is_external()
	not_configured_message.visible = editor_chat and not _api_configured
	offline_message.visible = editor_chat and _api_configured and not _online

	welcome_note.fix_section.visible = not _api_configured or not _online
	welcome_note.get_started_label.visible = _api_configured and _online
	welcome_note.settings_button.visible = not _api_configured
	welcome_note.go_online_button.visible = not _online
	if not _api_configured and not _online:
		welcome_note.fix_label.text = FIX_BOTH_TEXT
	elif not _api_configured:
		welcome_note.fix_label.text = FIX_NOT_CONFIGURED_TEXT
	else:
		welcome_note.fix_label.text = FIX_OFFLINE_TEXT


func _load_chat_sessions() -> void:
	session_list.clear()
	session_list.add_item(NEW_CHAT_SESSION_NAME)
	session_list.set_item_metadata(0, NEW_CHAT_SESSION_NAME)
	for item_id in _session_store.list_session_ids():
		var item_index := session_list.add_item(ChatSessionStore.chat_id_to_label(item_id))
		session_list.set_item_metadata(item_index, item_id)
	session_list.select(0)


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_PREDELETE:
			if _eval_run:
				_eval_run.flush_teardown_result()
			_session_store.flush()
			_mcp_instance.delete_user_file()
			_mcp_instance.release_project()


func _on_mcp_server_state_changed(p_server_state: MCPServer.ServerState) -> void:
	_update_mcp_status()

	if p_server_state == MCPServer.ServerState.STOPPED:
		mcp_stopping_timer.stop()
		_mcp_instance.delete_user_file()
		_mcp_instance.release_project()


func _on_mcp_client_state_changed(p_client_state: MCPServer.ClientState) -> void:
	_log_diagnostic("MCP client %s" % ("connected" if p_client_state == MCPServer.ClientState.CONNECTED else "disconnected"))

	# "For this session" lasts as long as the client stays connected.
	tool_auth.clear_session()

	_external_recorder.on_client_state_changed(p_client_state)

	_update_mcp_status()
	if p_client_state == MCPServer.ClientState.CONNECTED and _current_request:
		if _headless:
			_cancel_current_request()
		else:
			_confirm_cancel_current_request(_sync_session_selection, MCP_CONNECTED_CANCEL_TEXT)
	elif p_client_state == MCPServer.ClientState.NOT_CONNECTED:
		_tool_auth_queue.cancel_external()
		if _current_session and _current_session.client_kind == ChatSessionStore.ClientKind.MCP:
			_stop_current_chat()
	_update_prompt_bar()


func _on_mcp_update_available_changed(_p_update_available: Dictionary) -> void:
	_update_mcp_status()


func _update_mcp_status() -> void:
	var server_state: MCPServer.ServerState = mcp_server.get_server_state()
	var client_state: MCPServer.ClientState = mcp_server.get_client_state()

	if server_state in [MCPServer.ServerState.STOPPED, MCPServer.ServerState.ERROR]:
		start_mcp_button.visible = true
		stop_mcp_button.visible = false
	else:
		start_mcp_button.visible = false
		stop_mcp_button.visible = true

	var short_status: String
	var long_status: String
	var tooltip: String
	var icon: Texture = InfoIcon

	if server_state == MCPServer.ServerState.STOPPED:
		short_status = "MCP: Stopped"
		long_status = "MCP server stopped."
	elif server_state == MCPServer.ServerState.STOPPING:
		short_status = "MCP: Stopping"
		long_status = "MCP server stopping..."
		icon = WarningIcon
	elif server_state == MCPServer.ServerState.ERROR:
		short_status = "MCP: Error"
		long_status = "MCP server was unable to start."
		icon = ErrorIcon
	else:
		var transport: String = "WebSocket" if mcp_server.get_transport() == MCPServer.Transport.WEBSOCKET else "HTTP"
		long_status = "MCP server listening on port %d (%s transport)." % [mcp_server.get_port(), transport]

		if client_state == MCPServer.ClientState.NOT_CONNECTED:
			short_status = "MCP: Not connected"
			long_status += " Not connected"
			icon = WarningIcon
		else:
			short_status = "MCP: Connected"
			icon = SuccessIcon

			var client_info := mcp_server.get_client_info()
			if client_info.size() > 0:
				var connection_info := "Connected to %s version %s." % [client_info['name'], client_info['version']]
				tooltip = connection_info
				long_status += "\n" + connection_info
			else:
				long_status += " Connected."

			var update_available := mcp_server.get_update_available()
			if update_available.size() > 0:
				short_status += " (update available)"
				var install_command: String = update_available.get('install_command', '')
				if install_command.is_empty():
					long_status += "\ngodai %s is available." % update_available['latest_version']
				else:
					long_status += "\nRun '%s' in a terminal to install godai %s." % [install_command, update_available['latest_version']]

	mcp_button.icon = icon
	mcp_button.text = short_status
	mcp_button.tooltip_text = tooltip
	mcp_dialog.dialog_text = long_status


func _start_mcp() -> void:
	var err: Error
	mcp_error_message.visible = false

	# Claim this project for this MCP instance.
	err = _mcp_instance.claim_project()
	if err != OK:
		var msg: String = "Cannot start MCP server: "
		if err == ERR_ALREADY_IN_USE:
			msg += "another Godot editor is already running an MCP server for this project"
		else:
			msg += "error claiming project: " + error_string(err)
		_report_mcp_error(msg)
		return

	# Actually start the MCP server.
	err = mcp_server.start_server(_mcp_base_port, _mcp_port_count, _mcp_transport)
	if err != OK:
		_report_mcp_error("Cannot start MCP server (ports %d-%d): %s" % [
			_mcp_base_port, _mcp_base_port + _mcp_port_count - 1, error_string(err)])
		return

	# Write the instance file.
	err = _mcp_instance.write_user_file(mcp_server.get_port())
	if err != OK:
		_report_mcp_error("Cannot start MCP server: error writing instance file: " + error_string(err))
		return

	_log_diagnostic("MCP server listening on port %d, instance file %s" % [mcp_server.get_port(), _mcp_instance.get_user_file_path()])


func _report_mcp_error(p_msg: String) -> void:
	printerr("godai: " + p_msg)
	mcp_error_message.text = p_msg
	mcp_error_message.visible = true


func _log_diagnostic(p_msg: String) -> void:
	if OS.has_environment("GODAI_EDITOR_LOG"):
		print("godai: " + p_msg)


func _on_mcp_button_pressed() -> void:
	mcp_dialog.popup_centered()


func _on_settings_button_pressed() -> void:
	settings_dialog.online = GodaiEditorSettings.is_network_online()
	settings_dialog.setup(GodaiEditorSettings.get_dialog_settings())
	settings_dialog.popup_centered()


func _on_settings_dialog_closed(p_values: Dictionary) -> void:
	GodaiEditorSettings.set_dialog_settings(p_values)


func _on_go_online_requested() -> void:
	GodaiEditorSettings.set_network_online()


func _on_start_mcp_button_pressed() -> void:
	_start_mcp()


func _on_stop_mcp_button_pressed() -> void:
	mcp_server.stop_server()
	if mcp_server.get_server_state() == MCPServer.ServerState.STOPPING:
		mcp_stopping_timer.start()


func _on_mcp_stopping_timer_timeout() -> void:
	if mcp_server.get_server_state() == MCPServer.ServerState.STOPPING:
		mcp_server.stop_server(true)


func _session_list_index_of(p_id: String) -> int:
	for i in range(session_list.item_count):
		if session_list.get_item_metadata(i) == p_id:
			return i
	return -1


func _on_current_chat_message_added(p_msg: Chat.Message) -> void:
	if _session_list_index_of(_current_session.id) == -1:
		assert(session_list.get_item_text(0) == NEW_CHAT_SESSION_NAME)
		session_list.set_item_text(0, ChatSessionStore.chat_id_to_label(_current_session.id))
		session_list.set_item_metadata(0, _current_session.id)

	chat_view.show_message(p_msg)


func _on_mcp_tool_use_requested(p_id: String, p_name: String, p_input: Dictionary, p_client_kind: String) -> void:
	_external_recorder.record_tool_use(p_id, p_name, p_input, p_client_kind)


func _on_mcp_tool_use_completed(p_id: String, p_content) -> void:
	_external_recorder.record_tool_result(p_id, p_content)


func _on_external_session_started(p_session: ChatSessionStore.ChatSession) -> void:
	var item_index := session_list.add_item(ChatSessionStore.chat_id_to_label(p_session.id))
	session_list.set_item_metadata(item_index, p_session.id)
	var top_is_new: bool = session_list.get_item_metadata(0) == NEW_CHAT_SESSION_NAME
	session_list.move_item(item_index, 1 if top_is_new else 0)
	_sync_session_selection()


func _on_external_session_dropped(p_session: ChatSessionStore.ChatSession) -> void:
	if p_session != _current_session:
		_session_store.unload_session(p_session.id)


func _on_external_message_recorded(p_session: ChatSessionStore.ChatSession) -> void:
	# Switching sessions cancels any in-flight request, so wait until the current request finishes.
	if _current_session != p_session and p_session == _external_recorder.get_session() and _current_request == null:
		_ensure_new_chat_item()
		_set_current_session(p_session)
		_sync_session_selection()


func _update_prompt_bar() -> void:
	var request_active := _current_request != null
	var client_connected := mcp_server.get_client_state() == MCPServer.ClientState.CONNECTED
	var chat_available := _api_configured and _online and not client_connected
	var viewing_external_session := _current_session != null and _current_session.is_external()

	prompt_bar.visible = request_active or chat_available
	prompt.editable = not request_active and not viewing_external_session
	prompt.placeholder_text = PROMPT_PLACEHOLDER_TEXT if prompt.editable else (PROMPT_UNAVAILABLE_TEXT if viewing_external_session else "")
	submit_button.visible = not request_active
	submit_button.disabled = not prompt.editable or prompt.text.strip_edges().is_empty()
	cancel_button.visible = request_active
	clear_button.disabled = _current_session == null


func _resume_file_path() -> String:
	return Utils.get_project_path() + "/" + RESUME_FILE


func _on_chat_tool_use_completed(p_name: String, p_result: ToolManager.ToolResult) -> void:
	if p_name != "restart_editor" or p_result.is_error():
		return
	if _current_session == null or _current_session.is_external():
		return

	var f := FileAccess.open(_resume_file_path(), FileAccess.WRITE)
	if f:
		f.store_string(JSON.stringify({
			session_id = _current_session.id,
			pid = OS.get_process_id(),
			timestamp = int(Time.get_unix_time_from_system()),
		}))
		f.flush()


func _take_resume_session_id() -> String:
	var path := _resume_file_path()
	if not FileAccess.file_exists(path):
		return ""

	var data = JSON.parse_string(FileAccess.get_file_as_string(path))

	if not data is Dictionary \
			or Time.get_unix_time_from_system() - int(data.get("timestamp", 0)) > RESUME_MAX_AGE_SECONDS:
		DirAccess.remove_absolute(path)
		return ""
	if Utils.is_pid_running(int(data.get("pid", 0))):
		return ""

	DirAccess.remove_absolute(path)
	return str(data.get("session_id", ""))


func _resume_restarted_chat() -> void:
	var session_id := _take_resume_session_id()

	# Wait until the original editor has fully exited before resuming.
	var deadline := Time.get_ticks_msec() + int(_resume_retry_timeout * 1000)
	while session_id.is_empty() and FileAccess.file_exists(_resume_file_path()) \
			and Time.get_ticks_msec() < deadline:
		await get_tree().create_timer(_resume_retry_interval).timeout
		session_id = _take_resume_session_id()

	if session_id.is_empty():
		return

	var session := _session_store.load_session(session_id)
	if not session or session.is_external():
		return

	_set_current_session(session)
	_sync_session_selection()
	chat_view.scroll_to_bottom.call_deferred()
	_continue_chat()


func _start_new_chat() -> void:
	_set_current_session(_session_store.create_session(ChatSessionStore.ClientKind.EDITOR, GodaiEditorSettings.get_api_system_prompt()))


func _stop_current_chat() -> void:
	_set_current_session(null)
	prompt.clear()

	_ensure_new_chat_item()
	session_list.select(0)


func _ensure_new_chat_item() -> void:
	if session_list.item_count > 0 and session_list.get_item_metadata(0) == NEW_CHAT_SESSION_NAME:
		return
	var new_idx := session_list.add_item(NEW_CHAT_SESSION_NAME)
	session_list.set_item_metadata(new_idx, NEW_CHAT_SESSION_NAME)
	session_list.move_item(new_idx, 0)


func _set_current_session(p_session: ChatSessionStore.ChatSession) -> void:
	if _current_session == p_session:
		return

	var previous_session := _current_session
	if _current_session:
		_current_session.chat.message_added.disconnect(_on_current_chat_message_added)
	_current_session = null

	_cancel_current_request()

	if previous_session:
		if _current_request == null:
			_unload_if_inactive(previous_session)
		else:
			# Only unload the chat once the current request is completed.
			_current_request.completed.connect(
				func (_resp): _unload_if_inactive(previous_session), CONNECT_ONE_SHOT)

	if p_session:
		_current_session = p_session
		_current_session.chat.message_added.connect(_on_current_chat_message_added)

	chat_view.show_chat(_current_session.chat if _current_session else null)
	_update_status_messages()
	_update_prompt_bar()


func _unload_if_inactive(p_session: ChatSessionStore.ChatSession) -> void:
	if p_session != _current_session and p_session != _external_recorder.get_session():
		_session_store.unload_session(p_session.id)


func _set_current_request(p_request: ChatClient.Request) -> void:
	_current_request = p_request
	_current_request_changed.emit.call_deferred()
	_update_prompt_bar()


func _cancel_current_request() -> void:
	if not _current_request:
		return

	chat_view.set_loading(true, "Cancelling")

	# Cancel the current request before denying the pending approvals,
	# because normally denying an approval would continue the chat.
	_current_request.cancel()
	_tool_auth_queue.cancel_pending()


func _confirm_cancel_current_request(p_action: Callable, p_dialog_text: String = "") -> void:
	if not _current_request:
		p_action.call()
		return

	# Bail if the user is already looking at the cancel dialog.
	if cancel_chat_dialog.visible:
		return

	# Bail if cancel has already been confirmed (but not cleared yet).
	if _cancel_confirmed:
		return

	cancel_chat_dialog.dialog_text = p_dialog_text if not p_dialog_text.is_empty() else _default_cancel_chat_text
	_pending_cancel_action = p_action
	_cancel_confirmed = false
	if not _current_request.completed.is_connected(_on_pending_cancel_request_completed):
		_current_request.completed.connect(_on_pending_cancel_request_completed, CONNECT_ONE_SHOT)
	cancel_chat_dialog.popup_centered()


func _on_pending_cancel_request_completed(_p_resp: Provider.Response) -> void:
	var action := _pending_cancel_action
	var confirmed := _cancel_confirmed
	_pending_cancel_action = Callable()
	_cancel_confirmed = false
	cancel_chat_dialog.hide()
	if confirmed and action.is_valid():
		action.call()
	else:
		_sync_session_selection.call_deferred()


func _on_cancel_chat_dialog_confirmed() -> void:
	_cancel_confirmed = true
	_cancel_current_request()


func _on_cancel_chat_dialog_canceled() -> void:
	_pending_cancel_action = Callable()
	if _current_request and _current_request.completed.is_connected(_on_pending_cancel_request_completed):
		_current_request.completed.disconnect(_on_pending_cancel_request_completed)
	_sync_session_selection.call_deferred()


func _sync_session_selection() -> void:
	if _current_session:
		var index := _session_list_index_of(_current_session.id)
		if index != -1:
			session_list.select(index)
			return
	var new_index := _session_list_index_of(NEW_CHAT_SESSION_NAME)
	if new_index != -1:
		session_list.select(new_index)


func submit_prompt(p_text: String) -> void:
	prompt.text = p_text
	_submit_message()


func _submit_message() -> void:
	if _current_session and _current_session.is_external():
		return

	var content := prompt.text
	if content.strip_edges().is_empty():
		return

	prompt.clear()

	if _current_session == null:
		_start_new_chat()
	_current_session.chat.add_message(Chat.Message.new(Chat.Role.USER, content))

	_continue_chat()


func _continue_chat() -> void:
	chat_view.set_loading(true)

	var request := chat_client.submit_chat(_current_session.chat)
	_set_current_request(request)

	chat_started.emit(_current_session.chat, request)

	var resp: Provider.Response = await request.completed

	if _current_request != request:
		# If the request was cleared or changed while we were waiting, then bail.
		return
	_set_current_request(null)
	chat_view.set_loading(false)

	if resp.is_error():
		var error := resp.get_error()
		# A cancellation already shows up in the chat as its own notice.
		if error.type != "cancelled":
			chat_view.show_error("Error (%s): %s" % [error.type, error.message])
		return


func _on_prompt_text_changed() -> void:
	_update_prompt_bar()


func _on_prompt_gui_input(p_event: InputEvent) -> void:
	if p_event is InputEventKey:
		var key_event: InputEventKey = p_event
		if key_event.keycode == KEY_ENTER and key_event.pressed and not key_event.ctrl_pressed and not key_event.shift_pressed and not submit_button.disabled:
			_submit_message()
			accept_event()


func _on_submit_button_pressed() -> void:
	_submit_message()


func _on_cancel_button_pressed() -> void:
	_cancel_current_request()


func _on_clear_button_pressed() -> void:
	_confirm_cancel_current_request(_stop_current_chat)


func _on_expand_sidebar_button_toggled(p_toggled_on: bool) -> void:
	sidebar_container.visible = p_toggled_on


func _on_session_list_item_selected(p_index: int) -> void:
	var chat_id = session_list.get_item_metadata(p_index)

	var current_id: String = _current_session.id if _current_session else NEW_CHAT_SESSION_NAME
	if chat_id == current_id:
		return

	# The session is only loaded once the switch is confirmed, so a declined
	# switch doesn't leave it cached in the store.
	_confirm_cancel_current_request(func ():
		var session: ChatSessionStore.ChatSession
		if chat_id != NEW_CHAT_SESSION_NAME:
			session = _session_store.load_session(chat_id)
			if not session:
				printerr("Unable to load chat from ", _session_store.session_file_path(chat_id))
				_sync_session_selection.call_deferred()
				return
		_set_current_session(session)
		chat_view.scroll_to_bottom.call_deferred())
