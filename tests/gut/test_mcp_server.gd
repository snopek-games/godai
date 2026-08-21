extends GutTest

const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const JSONRPCDispatcher = preload("res://addons/godai/mcp/jsonrpc_dispatcher.gd")
const Utils = preload("res://addons/godai/utils.gd")


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
