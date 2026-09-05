extends RefCounted

const ChatSessionStore = preload("res://addons/godai/chat/chat_session_store.gd")
const Chat = preload("res://addons/godai/chat/chat.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")

signal session_started(p_session: ChatSessionStore.ChatSession)
signal session_dropped(p_session: ChatSessionStore.ChatSession)
signal message_recorded(p_session: ChatSessionStore.ChatSession)

var _store: ChatSessionStore
var _session: ChatSessionStore.ChatSession
var _pending_results: Dictionary
var _new_client_pending := false


func _init(p_store: ChatSessionStore) -> void:
	_store = p_store


func get_session() -> ChatSessionStore.ChatSession:
	return _session


func on_client_state_changed(p_state: MCPServer.ClientState) -> void:
	if p_state == MCPServer.ClientState.CONNECTED:
		_new_client_pending = true
	elif _session and _session.client_kind == ChatSessionStore.ClientKind.MCP:
		_drop_session()


func _drop_session() -> void:
	if _session == null:
		return
	var dropped := _session
	_session = null
	session_dropped.emit(dropped)


func record_tool_use(p_id: String, p_name: String, p_input: Dictionary, p_client_kind: String) -> void:
	var kind := _kind_from_string(p_client_kind)
	if kind == ChatSessionStore.ClientKind.EDITOR:
		return

	if _session == null or (kind == ChatSessionStore.ClientKind.MCP and _new_client_pending):
		_drop_session()
		_session = _store.create_session(kind)
		session_started.emit(_session)
	if kind == ChatSessionStore.ClientKind.MCP:
		_new_client_pending = false

	_pending_results[p_id] = _session
	_record(_session, Chat.Message.new(Chat.Role.ASSISTANT, Chat.ToolUseContent.new(p_id, p_name, p_input)))


func record_tool_result(p_id: String, p_content) -> void:
	if not _pending_results.has(p_id):
		return
	var session: ChatSessionStore.ChatSession = _pending_results[p_id]
	_pending_results.erase(p_id)

	_record(session, Chat.Message.new(Chat.Role.USER, Chat.ToolResultContent.new(p_id, p_content)))


func _record(p_session: ChatSessionStore.ChatSession, p_msg: Chat.Message) -> void:
	p_session.chat.add_message(p_msg)
	message_recorded.emit(p_session)


static func _kind_from_string(p_kind: String) -> ChatSessionStore.ClientKind:
	if p_kind.is_empty():
		return ChatSessionStore.ClientKind.EDITOR
	if p_kind == MCPServer.CLIENT_KIND_MCP:
		return ChatSessionStore.ClientKind.MCP
	return ChatSessionStore.ClientKind.CLI
