extends GutTest

const GodaiPanelScene = preload("res://addons/godai/ui/godai_panel.tscn")
const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const EvalRun = preload("res://addons/godai/eval_run.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")

const READ_ONLY_TOOL := "get_current_project"
const WRITE_TOOL := "set_project_settings"


class StubClient extends ClaudeClient:
	var submitted := 0

	func _do_http_request(p_request: Request, p_method: int, p_url: String, p_payload: Dictionary = {}) -> void:
		submitted += 1
		var http_request := HTTPRequest.new()
		add_child(http_request)
		p_request._http_request = http_request


var _panels: Array[Control]
var _session_files_to_delete: PackedStringArray


func after_each() -> void:
	for panel in _panels:
		if is_instance_valid(panel):
			panel._session_store.flush()
	_panels.clear()

	for fn in _session_files_to_delete:
		if FileAccess.file_exists(fn):
			DirAccess.remove_absolute(fn)
	_session_files_to_delete.clear()


func _make_panel() -> Control:
	var panel: Control = GodaiPanelScene.instantiate()
	add_child_autofree(panel)
	_panels.push_back(panel)
	return panel


func _make_request() -> ClaudeClient.Request:
	return ClaudeClient.Request.new(ClaudeClient.Chat.new())


func _track_session_file(panel: Control) -> void:
	_session_files_to_delete.push_back(
		panel._session_store.session_file_path(panel._external_recorder.get_session().id))


func _install_stub_client(panel: Control) -> StubClient:
	var stub := StubClient.new()
	stub.tools = panel.tools
	stub.tool_use_authorizer = panel.claude_client.tool_use_authorizer
	panel.add_child(stub)
	panel.claude_client = stub
	return stub


func _submit_and_track(panel: Control, p_text: String) -> void:
	panel.submit_prompt(p_text)
	_session_files_to_delete.push_back(
		panel._session_store.session_file_path(panel._current_session.id))


func _respond(stub: StubClient, req: ClaudeClient.Request, p_content: Array, p_stop_reason: String) -> void:
	var data := {
		type = "message",
		role = "assistant",
		stop_reason = p_stop_reason,
		content = p_content,
	}
	stub._on_request_completed(HTTPRequest.RESULT_SUCCESS, 200, PackedStringArray(),
		JSON.stringify(data).to_utf8_buffer(), req, req._http_request)


func _chat_items(panel: Control) -> Array:
	return panel.chat_view.chat_container.get_children().filter(
		func (c): return not c.is_queued_for_deletion())


func test_mcp_tool_use_waits_for_current_request() -> void:
	var panel := _make_panel()
	panel._set_current_request(_make_request())

	var gated = panel._tool_auth_queue.authorize_when_idle(READ_ONLY_TOOL, {})
	await wait_frames(2)
	assert_false(gated.is_done())

	panel._set_current_request(null)
	await wait_frames(2)

	assert_true(gated.is_done())
	assert_true(gated.allowed)


func test_mcp_session_is_read_only() -> void:
	var panel := _make_panel()

	panel._on_mcp_tool_use_requested("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	_track_session_file(panel)

	assert_false(panel.prompt_bar.visible)

	panel._set_current_session(null)
	assert_true(panel.prompt_bar.visible)


func test_mcp_recording_does_not_switch_chats_while_a_request_is_active() -> void:
	var panel := _make_panel()
	panel._set_current_request(_make_request())

	panel._on_mcp_tool_use_requested("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	panel._session_store.flush()
	_track_session_file(panel)

	var mcp_session = panel._external_recorder.get_session()
	assert_ne(panel._current_session, mcp_session)
	assert_eq(mcp_session.chat.messages.size(), 1)
	assert_true(FileAccess.file_exists(_session_files_to_delete[0]))

	panel._set_current_request(null)
	panel._on_mcp_tool_use_completed("mcp:1", {ok = true})

	assert_eq(panel._current_session, mcp_session)


func test_mcp_session_appears_in_the_session_list() -> void:
	var panel := _make_panel()

	panel._on_mcp_tool_use_requested("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	_track_session_file(panel)

	var session = panel._external_recorder.get_session()
	assert_eq(panel.session_list.get_item_metadata(1), session.id)
	assert_eq(panel._current_session, session)


func test_client_disconnect_denies_pending_tool_auth() -> void:
	var panel := _make_panel()
	panel._tool_auth_queue._headless = false
	var request = panel.mcp_server.tool_use_authorizer.call(WRITE_TOOL, {})
	assert_false(request.is_done())

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.NOT_CONNECTED)

	assert_true(request.is_done())
	assert_false(request.allowed)
	assert_engine_error("spawned at invalid position", "popping the dialog headless is fine")


func test_client_disconnect_denies_auth_gated_behind_a_busy_chat() -> void:
	var panel := _make_panel()
	panel._tool_auth_queue._headless = false
	panel._set_current_request(_make_request())

	var gated = panel.mcp_server.tool_use_authorizer.call(WRITE_TOOL, {})
	assert_false(gated.is_done())

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.NOT_CONNECTED)

	assert_true(gated.is_done())
	assert_false(gated.allowed)

	panel._set_current_request(null)
	await wait_frames(2)
	assert_false(panel.tool_use_auth_dialog.visible, "no dialog pops for the disconnected client")


func test_client_disconnect_keeps_the_editor_chats_pending_auth() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	panel._tool_auth_queue._headless = false
	_submit_and_track(panel, "hello")
	var request = stub.tool_use_authorizer.call(WRITE_TOOL, {})
	assert_false(request.is_done())

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.NOT_CONNECTED)

	assert_false(request.is_done())
	assert_engine_error("spawned at invalid position", "popping the dialog headless is fine")


func test_prompt_bar_returns_after_the_mcp_client_disconnects() -> void:
	var panel := _make_panel()
	panel.mcp_server.tool_use_requested.emit("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	_track_session_file(panel)
	assert_false(panel.prompt_bar.visible)

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.NOT_CONNECTED)

	assert_true(panel.prompt_bar.visible)
	assert_null(panel._current_session)


func test_cli_session_stays_selected_after_each_command_disconnects() -> void:
	var panel := _make_panel()
	panel.mcp_server.tool_use_requested.emit("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_CLI)
	_track_session_file(panel)
	var session = panel._external_recorder.get_session()
	assert_eq(panel._current_session, session)

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.NOT_CONNECTED)

	assert_eq(panel._current_session, session)
	assert_eq(panel.session_list.get_item_metadata(panel.session_list.get_selected_items()[0]), session.id)
	assert_false(panel.prompt_bar.visible)

	panel.mcp_server.tool_use_requested.emit("mcp:2", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_CLI)

	assert_eq(panel._external_recorder.get_session(), session)
	assert_eq(panel._current_session, session)
	assert_eq(session.chat.messages.size(), 2)


func test_unadopted_external_session_is_unloaded_when_dropped() -> void:
	var panel := _make_panel()
	panel._set_current_request(_make_request())

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.CONNECTED)
	panel.mcp_server.tool_use_requested.emit("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	_track_session_file(panel)
	var session = panel._external_recorder.get_session()
	assert_ne(panel._current_session, session)
	assert_eq(panel._session_store.load_session(session.id), session, "loading returns the live session")

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.NOT_CONNECTED)

	assert_ne(panel._session_store.load_session(session.id), session, "the dropped session was unloaded")


func test_submit_button_follows_text_changes_from_any_source() -> void:
	var panel := _make_panel()
	assert_true(panel.submit_button.disabled)

	panel.prompt.text = "hello"
	assert_false(panel.submit_button.disabled)

	panel.prompt.text = " "
	assert_true(panel.submit_button.disabled)


func test_status_dialog_keeps_server_info_when_an_update_is_available() -> void:
	var panel := _make_panel()
	panel.mcp_server._server_state = MCPServer.ServerState.STARTED
	panel.mcp_server._client_state = MCPServer.ClientState.CONNECTED
	panel.mcp_server._client_info = {name = "godai", version = "1.2.3"}
	panel.mcp_server._update_available = {latest_version = "9.9.9", install_command = "go install godai"}

	panel._update_mcp_status()

	assert_string_contains(panel.mcp_dialog.dialog_text, "MCP server listening on port")
	assert_string_contains(panel.mcp_dialog.dialog_text, "Connected to godai version 1.2.3.")
	assert_string_contains(panel.mcp_dialog.dialog_text, "9.9.9")


func test_reparenting_the_panel_keeps_the_eval_stream_open() -> void:
	var panel: Control = GodaiPanelScene.instantiate()
	add_child(panel)

	var stream_path := OS.get_cache_dir() + "/godai-test-eval-stream-%d.jsonl" % Time.get_ticks_usec()
	var eval_run := EvalRun.new()
	eval_run._file = FileAccess.open(stream_path, FileAccess.WRITE)
	panel._eval_run = eval_run

	remove_child(panel)
	add_child(panel)
	assert_not_null(eval_run._file)  # floating the bottom panel must not tear it down

	remove_child(panel)
	panel.free()
	assert_null(eval_run._file)
	assert_string_contains(FileAccess.get_file_as_string(stream_path), '"subtype":"editor_teardown"')

	DirAccess.remove_absolute(stream_path)


func test_submitting_a_prompt_round_trips_through_the_ui() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)

	_submit_and_track(panel, "hello there")

	assert_false(panel.submit_button.visible)
	assert_true(panel.cancel_button.visible)
	assert_false(panel.prompt.editable)
	assert_true(panel.chat_view.loading_label.visible)
	assert_eq(panel.session_list.get_item_metadata(0), panel._current_session.id, "the <new> item becomes the session")
	assert_false(panel.clear_button.disabled)

	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")

	assert_null(panel._current_request)
	assert_true(panel.submit_button.visible)
	assert_false(panel.cancel_button.visible)
	assert_true(panel.prompt.editable)
	assert_false(panel.chat_view.loading_label.visible)
	assert_eq(panel._current_session.chat.messages.size(), 2)

	var items := _chat_items(panel)
	assert_eq(items.size(), 2)
	assert_eq(items[0].label.text, "hello there")
	assert_eq(items[1].text, "hi!")


func test_api_error_shows_in_the_chat_view() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "hello")

	var data := {type = "error", error = {type = "overloaded_error", message = "Try again"}}
	stub._on_request_completed(HTTPRequest.RESULT_SUCCESS, 529, PackedStringArray(),
		JSON.stringify(data).to_utf8_buffer(), panel._current_request, panel._current_request._http_request)

	assert_true(panel.prompt.editable)
	var items := _chat_items(panel)
	assert_eq(items.size(), 2)
	assert_eq(items[1].label.text, "Error (overloaded_error): Try again")


func test_cancel_button_cancels_and_shows_the_notice() -> void:
	var panel := _make_panel()
	_install_stub_client(panel)
	_submit_and_track(panel, "hello")

	panel.cancel_button.pressed.emit()

	assert_null(panel._current_request)
	assert_true(panel.submit_button.visible)
	assert_false(panel.cancel_button.visible)
	assert_true(panel.prompt.editable)
	assert_eq(panel._current_session.chat.messages.size(), 2, "the prompt plus the cancellation note")

	var items := _chat_items(panel)
	assert_eq(items.size(), 2)
	assert_eq(items[1].text, "Cancelled by user")


func test_headless_mcp_connect_cancels_the_chat_instead_of_asking() -> void:
	var panel := _make_panel()
	_install_stub_client(panel)
	_submit_and_track(panel, "hello")
	var session = panel._current_session

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.CONNECTED)

	assert_false(panel.cancel_chat_dialog.visible)
	assert_null(panel._current_request)
	assert_eq(panel._current_session, session, "the cancelled chat stays open")
	assert_eq(panel.session_list.get_item_metadata(panel.session_list.get_selected_items()[0]), session.id)


func test_confirming_the_mcp_connect_cancel_keeps_the_chat_until_a_tool_use_arrives() -> void:
	var panel := _make_panel()
	panel._headless = false
	_install_stub_client(panel)
	_submit_and_track(panel, "hello")
	var session = panel._current_session

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.CONNECTED)
	panel.cancel_chat_dialog.confirmed.emit()

	assert_null(panel._current_request)
	assert_eq(panel._current_session, session, "the cancelled chat stays open")
	assert_eq(panel.session_list.get_item_metadata(panel.session_list.get_selected_items()[0]), session.id)

	panel.mcp_server.tool_use_requested.emit("mcp:1", READ_ONLY_TOOL, {}, MCPServer.CLIENT_KIND_MCP)
	_track_session_file(panel)

	var external_session = panel._external_recorder.get_session()
	assert_eq(panel._current_session, external_session)
	assert_eq(panel.session_list.get_item_metadata(panel.session_list.get_selected_items()[0]), external_session.id)


func test_gui_mcp_connect_asks_before_cancelling_the_chat() -> void:
	var panel := _make_panel()
	panel._headless = false
	_install_stub_client(panel)
	_submit_and_track(panel, "hello")

	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.CONNECTED)

	assert_true(panel.cancel_chat_dialog.visible)
	assert_eq(panel.cancel_chat_dialog.dialog_text, panel.MCP_CONNECTED_CANCEL_TEXT)
	assert_not_null(panel._current_request)


func test_confirming_the_cancel_dialog_cancels_and_runs_the_action() -> void:
	var panel := _make_panel()
	_install_stub_client(panel)
	_submit_and_track(panel, "hello")

	panel._on_clear_button_pressed()
	assert_true(panel.cancel_chat_dialog.visible)

	panel.cancel_chat_dialog.confirmed.emit()

	assert_null(panel._current_request)
	assert_null(panel._current_session, "the pending clear ran after the cancel")
	assert_true(panel.session_list.is_selected(0))


func test_natural_completion_while_the_cancel_dialog_is_open_runs_no_action() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "hello")

	panel._on_clear_button_pressed()
	assert_true(panel.cancel_chat_dialog.visible)

	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")

	assert_false(panel.cancel_chat_dialog.visible)
	assert_false(panel._pending_cancel_action.is_valid())
	assert_not_null(panel._current_session, "the finished chat is kept")
	assert_eq(_chat_items(panel).size(), 2)


func test_natural_completion_restores_the_session_list_selection() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "first chat")
	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")
	var first_id: String = panel._current_session.id
	panel.clear_button.pressed.emit()

	# Chat ids have millisecond resolution, so let the clock tick.
	await wait_seconds(0.05)

	_submit_and_track(panel, "second chat")
	var second_id: String = panel._current_session.id
	assert_ne(second_id, first_id)

	var first_index: int = panel._session_list_index_of(first_id)
	panel.session_list.select(first_index)
	panel.session_list.item_selected.emit(first_index)
	assert_true(panel.cancel_chat_dialog.visible)

	_respond(stub, panel._current_request, [{type = "text", text = "hi again!"}], "end_turn")
	await wait_frames(2)

	assert_eq(panel._current_session.id, second_id, "the finished chat is kept")
	assert_true(panel.session_list.is_selected(panel._session_list_index_of(second_id)))


func test_a_second_confirm_request_keeps_the_first_pending_action() -> void:
	var panel := _make_panel()
	_install_stub_client(panel)
	_submit_and_track(panel, "hello")

	var first_ran := []
	panel._confirm_cancel_current_request(func (): first_ran.push_back(true))
	panel._confirm_cancel_current_request(panel._stop_current_chat, "different text")

	assert_eq(panel.cancel_chat_dialog.dialog_text, panel._default_cancel_chat_text)

	panel.cancel_chat_dialog.confirmed.emit()

	assert_eq(first_ran.size(), 1)
	assert_not_null(panel._current_session, "the second action never ran")


func test_a_second_confirm_while_a_confirmed_cancel_drains_keeps_the_action() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	var slow_result := ToolManager.ToolResult.new()
	panel.tools.register_tool(ToolManager.CallbackTool.new(
		"slow_tool", "slow_tool", "A test tool.", func (_input): return slow_result))
	panel.tool_auth.set_tool_for_session("slow_tool", true)
	_submit_and_track(panel, "hello")
	_respond(stub, panel._current_request, [
		{type = "tool_use", id = "toolu_1", name = "slow_tool", input = {}},
	], "tool_use")

	panel._on_clear_button_pressed()
	panel.cancel_chat_dialog.hide()
	panel.cancel_chat_dialog.confirmed.emit()
	assert_not_null(panel._current_request, "the cancel waits for the running tool")

	var second_ran := []
	panel._confirm_cancel_current_request(func (): second_ran.push_back(true))
	assert_false(panel.cancel_chat_dialog.visible,
		"no second dialog while the confirmed cancel drains")

	slow_result.resolve("slow output")

	assert_null(panel._current_session, "the confirmed clear still ran")
	assert_eq(second_ran.size(), 0)


func test_clear_button_returns_to_a_new_chat() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "hello")
	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")

	panel.clear_button.pressed.emit()

	assert_null(panel._current_session)
	assert_true(panel.clear_button.disabled)
	assert_eq(panel.session_list.get_item_metadata(0), panel.NEW_CHAT_SESSION_NAME)
	assert_true(panel.session_list.is_selected(0))
	assert_eq(_chat_items(panel).size(), 0)


func test_selecting_a_session_reloads_it_from_disk() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "first chat")
	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")
	var first_id: String = panel._current_session.id

	panel.clear_button.pressed.emit()

	var index: int = panel._session_list_index_of(first_id)
	assert_ne(index, -1)
	panel.session_list.select(index)
	panel.session_list.item_selected.emit(index)

	assert_eq(panel._current_session.id, first_id)
	assert_eq(panel._current_session.chat.messages.size(), 2)
	assert_eq(_chat_items(panel).size(), 2)


func test_selecting_a_saved_session_enables_the_clear_button() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "hello")
	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")
	var first_id: String = panel._current_session.id

	panel.clear_button.pressed.emit()
	assert_true(panel.clear_button.disabled)

	var index: int = panel._session_list_index_of(first_id)
	panel.session_list.select(index)
	panel.session_list.item_selected.emit(index)

	assert_false(panel.clear_button.disabled)


func test_mcp_connect_during_a_chat_keeps_the_cancel_ui() -> void:
	var panel := _make_panel()
	panel._headless = false
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "hello")

	panel.mcp_server._client_state = MCPServer.ClientState.CONNECTED
	panel.mcp_server.client_state_changed.emit(MCPServer.ClientState.CONNECTED)
	panel.cancel_chat_dialog.hide()
	panel.cancel_chat_dialog.canceled.emit()

	assert_true(panel.prompt_bar.visible, "the running chat keeps its cancel button")
	assert_true(panel.cancel_button.visible)

	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")

	assert_false(panel.prompt_bar.visible, "the bar yields to the client once the chat ends")


func test_declining_a_session_switch_leaves_the_other_session_unloaded() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_submit_and_track(panel, "first chat")
	_respond(stub, panel._current_request, [{type = "text", text = "hi!"}], "end_turn")
	var first_id: String = panel._current_session.id
	panel.clear_button.pressed.emit()

	# Chat ids have millisecond resolution, so let the clock tick.
	await wait_seconds(0.05)

	_submit_and_track(panel, "second chat")

	var first_index: int = panel._session_list_index_of(first_id)
	panel.session_list.select(first_index)
	panel.session_list.item_selected.emit(first_index)
	assert_true(panel.cancel_chat_dialog.visible)
	assert_false(panel._session_store._sessions.has(first_id), "not loaded until the switch is confirmed")

	panel.cancel_chat_dialog.hide()
	panel.cancel_chat_dialog.canceled.emit()

	assert_false(panel._session_store._sessions.has(first_id))
	assert_not_null(panel._current_request, "the chat keeps running")


func test_stopping_a_chat_with_a_pending_request_unloads_the_session_later() -> void:
	var panel := _make_panel()
	panel._start_new_chat()
	var session = panel._current_session
	var request := ClaudeClient.Request.new(session.chat)
	panel._set_current_request(request)

	panel._stop_current_chat()

	assert_true(panel._session_store._sessions.has(session.id),
		"kept while the request can still record into it")

	request.resolve(ClaudeClient.Response.new(null,
		ClaudeClient.ResponseError.new("cancelled", "The request was cancelled.")))

	assert_false(panel._session_store._sessions.has(session.id))


func test_shift_enter_does_not_submit() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	panel.prompt.text = "hello"

	var event := InputEventKey.new()
	event.keycode = KEY_ENTER
	event.pressed = true
	event.shift_pressed = true
	panel._on_prompt_gui_input(event)

	assert_null(panel._current_request)
	assert_eq(stub.submitted, 0)


func _write_resume_file(panel: Control, p_data: Dictionary) -> void:
	var f := FileAccess.open(panel._resume_file_path(), FileAccess.WRITE)
	f.store_string(JSON.stringify(p_data))
	f.close()


func test_successful_restart_tool_writes_resume_file() -> void:
	var panel := _make_panel()
	_session_files_to_delete.push_back(panel._resume_file_path())
	panel._start_new_chat()

	panel._on_claude_tool_use_completed("restart_editor", ToolManager.ToolResult.resolved({success = true}))

	var data = JSON.parse_string(FileAccess.get_file_as_string(panel._resume_file_path()))
	assert_eq(data["session_id"], panel._current_session.id)
	assert_eq(int(data["pid"]), OS.get_process_id())


func test_declined_restart_tool_does_not_write_resume_file() -> void:
	var panel := _make_panel()
	_session_files_to_delete.push_back(panel._resume_file_path())
	panel._start_new_chat()

	panel._on_claude_tool_use_completed("restart_editor",
		ToolManager.ToolResult.rejected({errors = ["The user declined to restart the editor"]}))

	assert_false(FileAccess.file_exists(panel._resume_file_path()))


func test_take_resume_session_id_is_one_shot() -> void:
	var panel := _make_panel()
	_session_files_to_delete.push_back(panel._resume_file_path())

	_write_resume_file(panel, {session_id = "abc", pid = 999999999,
		timestamp = int(Time.get_unix_time_from_system())})

	assert_eq(panel._take_resume_session_id(), "abc")
	assert_false(FileAccess.file_exists(panel._resume_file_path()))
	assert_eq(panel._take_resume_session_id(), "")


func test_take_resume_session_id_rejects_stale_or_live_writer() -> void:
	var panel := _make_panel()
	_session_files_to_delete.push_back(panel._resume_file_path())

	_write_resume_file(panel, {session_id = "abc", pid = 999999999,
		timestamp = int(Time.get_unix_time_from_system()) - panel.RESUME_MAX_AGE_SECONDS - 1})
	assert_eq(panel._take_resume_session_id(), "")
	assert_false(FileAccess.file_exists(panel._resume_file_path()), "a stale marker is discarded")

	_write_resume_file(panel, {session_id = "abc", pid = OS.get_process_id(),
		timestamp = int(Time.get_unix_time_from_system())})
	assert_eq(panel._take_resume_session_id(), "")
	assert_true(FileAccess.file_exists(panel._resume_file_path()),
		"kept for the real restart replacement to retry")


func test_resume_retries_until_the_old_editor_exits() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_session_files_to_delete.push_back(panel._resume_file_path())

	var session = panel._session_store.create_session()
	session.chat.add_message(ClaudeClient.Message.new("user", "resume me"))
	_session_files_to_delete.push_back(panel._session_store.session_file_path(session.id))

	_write_resume_file(panel, {session_id = session.id, pid = OS.get_process_id(),
		timestamp = int(Time.get_unix_time_from_system())})

	panel._resume_retry_interval = 0.05
	panel._resume_restarted_chat()

	_write_resume_file(panel, {session_id = session.id, pid = 999999999,
		timestamp = int(Time.get_unix_time_from_system())})
	await wait_seconds(0.2)

	assert_eq(panel._current_session, session)
	assert_not_null(panel._current_request)
	assert_eq(stub.submitted, 1)


func test_resume_gives_up_when_the_old_editor_never_exits() -> void:
	var panel := _make_panel()
	var stub := _install_stub_client(panel)
	_session_files_to_delete.push_back(panel._resume_file_path())

	_write_resume_file(panel, {session_id = "abc", pid = OS.get_process_id(),
		timestamp = int(Time.get_unix_time_from_system())})

	panel._resume_retry_interval = 0.05
	panel._resume_retry_timeout = 0.15
	await panel._resume_restarted_chat()

	assert_null(panel._current_session)
	assert_eq(stub.submitted, 0)
	assert_true(FileAccess.file_exists(panel._resume_file_path()),
		"kept for a later attempt")
