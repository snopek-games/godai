extends GutTest

const ChatSessionStore = preload("res://addons/godai/chat/chat_session_store.gd")
const ExternalSessionRecorder = preload("res://addons/godai/chat/external_session_recorder.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")

const TOOL := "get_current_project"

var _temp_dir: String
var _store: ChatSessionStore
var _recorder: ExternalSessionRecorder


func before_each() -> void:
	_temp_dir = OS.get_cache_dir() + "/godai-test-mcp-recorder-%d" % Time.get_ticks_usec()
	_store = ChatSessionStore.new(_temp_dir)
	_recorder = ExternalSessionRecorder.new(_store)


func after_each() -> void:
	_recorder = null
	_store = null
	var dir := DirAccess.open(_temp_dir)
	if dir:
		dir.list_dir_begin()
		var fn := dir.get_next()
		while fn != "":
			if not dir.current_is_dir():
				dir.remove(fn)
			fn = dir.get_next()
		DirAccess.remove_absolute(_temp_dir)


func test_cli_tool_calls_record_to_one_session() -> void:
	watch_signals(_recorder)

	_recorder.record_tool_use("mcp:1", TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	_recorder.record_tool_result("mcp:1", {ok = true})
	_recorder.record_tool_use("mcp:2", TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	_store.flush()

	var session = _recorder.get_session()
	assert_true(session.is_external())
	assert_eq(session.client_kind, ChatSessionStore.ClientKind.CLI)
	assert_eq(session.chat.messages.size(), 3)
	assert_signal_emit_count(_recorder, "session_started", 1)
	assert_true(FileAccess.file_exists(_store.session_file_path(session.id)))


func test_each_mcp_client_gets_its_own_session() -> void:
	_recorder.on_client_state_changed(MCPServer.ClientState.CONNECTED)
	_recorder.record_tool_use("mcp:1", TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	var first_session = _recorder.get_session()

	_recorder.record_tool_use("mcp:2", TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	assert_eq(_recorder.get_session(), first_session)

	_recorder.on_client_state_changed(MCPServer.ClientState.NOT_CONNECTED)
	_recorder.on_client_state_changed(MCPServer.ClientState.CONNECTED)
	_recorder.record_tool_use("mcp:3", TOOL, {}, MCPServer.CLIENT_KIND_MCP)

	assert_ne(_recorder.get_session(), first_session)
	assert_eq(_recorder.get_session().client_kind, ChatSessionStore.ClientKind.MCP)
	assert_eq(_recorder.get_session().chat.messages.size(), 1)


func test_cli_tool_calls_start_fresh_after_an_mcp_client_session_ends() -> void:
	_recorder.on_client_state_changed(MCPServer.ClientState.CONNECTED)
	_recorder.record_tool_use("mcp:1", TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	var client_session = _recorder.get_session()

	_recorder.on_client_state_changed(MCPServer.ClientState.NOT_CONNECTED)
	_recorder.record_tool_use("mcp:2", TOOL, {}, MCPServer.CLIENT_KIND_CLI)

	assert_ne(_recorder.get_session(), client_session)
	assert_eq(_recorder.get_session().client_kind, ChatSessionStore.ClientKind.CLI)


func test_session_dropped_when_the_mcp_client_disconnects() -> void:
	watch_signals(_recorder)
	_recorder.on_client_state_changed(MCPServer.ClientState.CONNECTED)
	_recorder.record_tool_use("mcp:1", TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	var session = _recorder.get_session()

	_recorder.on_client_state_changed(MCPServer.ClientState.NOT_CONNECTED)

	assert_null(_recorder.get_session())
	assert_signal_emitted_with_parameters(_recorder, "session_dropped", [session])


func test_cli_session_dropped_when_an_mcp_client_starts_recording() -> void:
	watch_signals(_recorder)
	_recorder.record_tool_use("mcp:1", TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	var cli_session = _recorder.get_session()

	_recorder.on_client_state_changed(MCPServer.ClientState.CONNECTED)
	_recorder.record_tool_use("mcp:2", TOOL, {}, MCPServer.CLIENT_KIND_MCP)

	assert_eq(_recorder.get_session().client_kind, ChatSessionStore.ClientKind.MCP)
	assert_signal_emitted_with_parameters(_recorder, "session_dropped", [cli_session])


func test_internal_tool_calls_are_not_recorded() -> void:
	_recorder.record_tool_use("mcp:1", TOOL, {}, "")
	_recorder.record_tool_result("mcp:1", {ok = true})

	assert_null(_recorder.get_session())


func test_late_result_lands_in_the_session_that_started_it() -> void:
	_recorder.on_client_state_changed(MCPServer.ClientState.CONNECTED)
	_recorder.record_tool_use("mcp:1", TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	var client_session = _recorder.get_session()

	_recorder.on_client_state_changed(MCPServer.ClientState.NOT_CONNECTED)
	_recorder.record_tool_result("mcp:1", {ok = true})

	assert_null(_recorder.get_session())
	assert_eq(client_session.chat.messages.size(), 2)
	assert_eq(client_session.chat.messages[1].content[0].data["tool_use_id"], "mcp:1")
