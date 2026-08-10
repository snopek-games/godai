extends GutTest

const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const Utils = preload("res://addons/godai/utils.gd")


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
