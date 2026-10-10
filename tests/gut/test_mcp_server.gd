extends GutTest

const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const JSONRPCDispatcher = preload("res://addons/godai/mcp/jsonrpc_dispatcher.gd")
const Utils = preload("res://addons/godai/utils.gd")
const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")

const UNFOCUSED_SLEEP_USEC = 100000


func _make_server_with_tool() -> Node:
	var tools := ToolManager.new()
	tools.register_tool(ToolManager.CallbackTool.new(
		"test_tool", "Test tool", "A test tool",
		func (_input): return ToolManager.ToolResult.resolved("ok")))
	return autofree(MCPServer.new(tools, "secret"))


# Every one-shot `godai editor-tool` is its own connection, so a client
# going away can't forget which scripts have been read: read_script and
# write_script would never be the same connection, leaving write_script unusable
# from the CLI.
func test_client_disconnect_keeps_script_reads() -> void:
	var server: Node = autofree(MCPServer.new(ToolManager.new(), "secret"))
	server._transport = MCPServer.Transport.WEBSOCKET

	Utils.clear_script_reads()
	Utils.record_script_read("res://disconnect.gd", "extends Node\n")

	server._remove_peer(MCPServer.Peer.new(1))

	assert_false(Utils.check_script_writable("res://disconnect.gd", "extends Node\n").has("error"))

	Utils.clear_script_reads()


func _make_unfocused_server() -> Node:
	var server: Node = autofree(MCPServer.new(ToolManager.new(), "secret"))
	server._transport = MCPServer.Transport.WEBSOCKET
	OS.low_processor_usage_mode_sleep_usec = UNFOCUSED_SLEEP_USEC
	server.notification(NOTIFICATION_APPLICATION_FOCUS_OUT)
	return server


func test_unfocused_editor_runs_at_focused_rate_while_client_connected() -> void:
	var original_sleep_usec := OS.low_processor_usage_mode_sleep_usec
	var server := _make_unfocused_server()
	assert_eq(OS.low_processor_usage_mode_sleep_usec, UNFOCUSED_SLEEP_USEC)

	var peer := MCPServer.Peer.new(1)
	server._add_peer(peer)
	assert_eq(OS.low_processor_usage_mode_sleep_usec, GodaiEditorSettings.get_focused_low_processor_sleep_usec())

	server._remove_peer(peer)
	assert_eq(OS.low_processor_usage_mode_sleep_usec, UNFOCUSED_SLEEP_USEC)

	OS.low_processor_usage_mode_sleep_usec = original_sleep_usec


func test_focus_lost_while_client_connected_keeps_focused_rate() -> void:
	var original_sleep_usec := OS.low_processor_usage_mode_sleep_usec
	var server: Node = autofree(MCPServer.new(ToolManager.new(), "secret"))
	server._transport = MCPServer.Transport.WEBSOCKET
	var peer := MCPServer.Peer.new(1)
	server._add_peer(peer)

	OS.low_processor_usage_mode_sleep_usec = UNFOCUSED_SLEEP_USEC
	server.notification(NOTIFICATION_APPLICATION_FOCUS_OUT)
	assert_eq(OS.low_processor_usage_mode_sleep_usec, GodaiEditorSettings.get_focused_low_processor_sleep_usec())

	server._remove_peer(peer)
	assert_eq(OS.low_processor_usage_mode_sleep_usec, UNFOCUSED_SLEEP_USEC)

	OS.low_processor_usage_mode_sleep_usec = original_sleep_usec


func test_focused_editor_sleep_is_left_to_the_editor() -> void:
	var original_sleep_usec := OS.low_processor_usage_mode_sleep_usec
	var server := _make_unfocused_server()
	server.notification(NOTIFICATION_APPLICATION_FOCUS_IN)

	server._add_peer(MCPServer.Peer.new(1))
	assert_eq(OS.low_processor_usage_mode_sleep_usec, UNFOCUSED_SLEEP_USEC)

	OS.low_processor_usage_mode_sleep_usec = original_sleep_usec


func test_tool_call_allowed_when_meta_godai_version_matches() -> void:
	var server := _make_server_with_tool()
	server._rpc_initialize({
		clientInfo = {name = "claude-code", version = "2.0.1"},
		_meta = {MCPServer.GODAI_VERSION_META_KEY: MCPServer.GODAI_VERSION},
	})

	var result = server._rpc_call_tool({name = "test_tool", arguments = {}})
	assert_false(result is JSONRPCDispatcher.ResponseError)


func test_tool_call_rejected_when_meta_godai_version_mismatches() -> void:
	var server := _make_server_with_tool()
	server._rpc_initialize({
		clientInfo = {name = "Godai", version = "0.0.1"},
		_meta = {MCPServer.GODAI_VERSION_META_KEY: "0.0.1"},
	})

	var result = server._rpc_call_tool({name = "test_tool", arguments = {}})
	assert_true(result is JSONRPCDispatcher.ResponseError)


# An MCP client's own version (e.g. Claude Code's) says nothing about which
# godai it came through, so without the meta key there's no version check.
func test_tool_call_allowed_when_no_godai_version_reported() -> void:
	var server := _make_server_with_tool()
	server._rpc_initialize({
		clientInfo = {name = "claude-code", version = "2.0.1"},
	})

	var result = server._rpc_call_tool({name = "test_tool", arguments = {}})
	assert_false(result is JSONRPCDispatcher.ResponseError)


func test_image_attachments_become_image_content_blocks() -> void:
	var image := Image.create(2, 2, false, Image.FORMAT_RGB8)
	var tools := ToolManager.new()
	tools.register_tool(ToolManager.CallbackTool.new(
		"shot", "Shot", "Takes a picture",
		func (_input): return ToolManager.ToolResult.resolved_with_attachments("ok", [image, "not an image"])))
	var server: Node = autofree(MCPServer.new(tools, "secret"))

	var result: Dictionary = server._rpc_call_tool({name = "shot", arguments = {}})

	assert_eq(result['content'].size(), 2)
	assert_eq(result['content'][0], {type = "text", text = "ok"})
	assert_eq(result['content'][1]['type'], "image")
	assert_eq(result['content'][1]['mimeType'], "image/png")
	assert_eq(Marshalls.base64_to_raw(result['content'][1]['data']), image.save_png_to_buffer())


func test_tool_use_completed_carries_the_whole_result() -> void:
	var image := Image.create(2, 2, false, Image.FORMAT_RGB8)
	var tools := ToolManager.new()
	tools.register_tool(ToolManager.CallbackTool.new(
		"shot", "Shot", "Takes a picture",
		func (_input): return ToolManager.ToolResult.resolved_with_attachments("ok", [image])))
	var server: Node = autofree(MCPServer.new(tools, "secret"))
	watch_signals(server)

	server._rpc_call_tool({name = "shot", arguments = {}})
	await wait_process_frames(1)

	assert_signal_emit_count(server, "tool_use_completed", 1)
	var result: ToolManager.ToolResult = get_signal_parameters(server, "tool_use_completed")[1]
	assert_eq(result.content, "ok")
	assert_eq(result.attachments, [image])
