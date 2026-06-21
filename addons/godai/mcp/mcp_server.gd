@tool
extends Node

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const JSONRPCDispatcher = preload("res://addons/godai/mcp/jsonrpc_dispatcher.gd")
const Utils = preload("res://addons/godai/utils.gd")

const PROTOCOL_VERSION = "2025-06-18"

static var GODAI_VERSION: String = _load_godai_version()

static func _load_godai_version() -> String:
	var config := ConfigFile.new()
	var err := config.load("res://addons/godai/plugin.cfg")
	if err != OK:
		push_error("Unable to load plugin.cfg to determine the Godai version")
		return "unknown"
	return config.get_value("plugin", "version", "unknown")

enum Transport {
	WEBSOCKET,
	HTTP,
}

enum ServerState {
	STARTED,
	STOPPING,
	STOPPED,
	ERROR,
}

enum ClientState {
	NOT_CONNECTED,
	CONNECTED,
}

class Peer extends RefCounted:
	var peer_id: int
	var tcp_peer: StreamPeerTCP
	var websocket_peer: WebSocketPeer
	var buffer: String

	func _init(p_peer_id: int) -> void:
		peer_id = p_peer_id

	func consume_buffer(p_amount: int) -> void:
		buffer = buffer.substr(p_amount)


var _tcp_server: TCPServer
var _peers: Dictionary[int, Peer]
var _base_port: int
var _port_count: int
var _port: int
var _server_state: ServerState = ServerState.STOPPED
var _transport: Transport = Transport.WEBSOCKET
var _client_state: ClientState = ClientState.NOT_CONNECTED
var _client_info: Dictionary
var _last_peer_id := 1
var _last_tool_id := 0

var _jsonrpc := JSONRPCDispatcher.new()

var tools: ToolManager

signal server_state_changed(state: ServerState)
signal client_state_changed(state: ClientState)
signal tool_use_requested(p_id: String, p_name: String, p_input: Dictionary)
signal tool_use_completed(p_id: String, p_content)


func _init(p_tools: ToolManager) -> void:
	tools = p_tools

	set_process(false)

	_jsonrpc.set_method("initialize", _rpc_initialize)
	_jsonrpc.set_method("notifications/initialized", _rpc_client_initialized)
	_jsonrpc.set_method("tools/list", _rpc_list_tools)
	_jsonrpc.set_method("tools/call", _rpc_call_tool)


func get_server_state() -> ServerState:
	return _server_state


func get_port() -> int:
	return _port


func get_transport() -> Transport:
	return _transport


func get_client_state() -> ClientState:
	return _client_state


func get_client_info() -> Dictionary:
	return _client_info


func start_server(p_base_port: int, p_port_count: int, p_transport: Transport) -> Error:
	if _server_state in [ServerState.STARTED, ServerState.STOPPING]:
		return ERR_ALREADY_IN_USE

	_base_port = p_base_port
	_port_count = p_port_count
	_port = 0
	_transport = p_transport
	_tcp_server = TCPServer.new()

	var err = _try_tcp_server_listen()
	set_process(err == OK)
	_server_state = ServerState.STARTED if err == OK else ServerState.ERROR
	server_state_changed.emit(_server_state)
	return err


func _try_tcp_server_listen() -> Error:
	var err: Error
	for port in range(_base_port, _base_port + _port_count):
		err = _tcp_server.listen(port, "127.0.0.1")
		if err == OK:
			_port = port
			return OK
		elif err != ERR_ALREADY_IN_USE:
			return err

	return ERR_ALREADY_IN_USE


func stop_server(p_force: bool = false) -> void:
	if not ((_server_state == ServerState.STARTED) or (p_force and _server_state == ServerState.STOPPING)):
		return

	_tcp_server.stop()

	for peer in _peers.values():
		if _transport == Transport.WEBSOCKET:
			peer.websocket_peer.close()
		if _transport == Transport.HTTP or p_force:
			# This happens too for forced WebSocket closures.
			peer.tcp_peer.disconnect_from_host()

	if _transport == Transport.WEBSOCKET and _peers.size() > 0 and not p_force:
		_server_state = ServerState.STOPPING
		server_state_changed.emit(_server_state)
	else:
		_peers.clear()
		_stop_server_complete()


func _stop_server_complete() -> void:
	set_process(false)
	_tcp_server = null
	_server_state = ServerState.STOPPED
	server_state_changed.emit(_server_state)


func _rpc_initialize(p_params: Dictionary):
	_client_info = p_params['clientInfo']
	_client_state = ClientState.CONNECTED
	client_state_changed.emit(_client_state)

	return {
		protocolVersion = PROTOCOL_VERSION,
		capabilities = {
			tools = {},
		},
		serverInfo = {
			name = "Godai",
			version = GODAI_VERSION,
		}
	}


func _rpc_client_initialized(_params: Dictionary):
	pass


func _rpc_list_tools(p_params: Dictionary):
	var result = []

	for tool_obj in tools.get_tools():
		var d := {
			name = tool_obj.name,
			title = tool_obj.title,
			description = tool_obj.description,
			inputSchema = tool_obj.input_schema,
		}

		if tool_obj.output_schema.size() > 0:
			d['outputSchema'] = tool_obj.output_schema

		result.push_back(d)

	return {tools = result}


func _rpc_call_tool(p_params: Dictionary):
	var name: String = p_params['name']
	var args: Dictionary = p_params['arguments']

	if not tools.has_tool(name):
		return JSONRPCDispatcher.ResponseError.new(
			JSONRPCDispatcher.ErrorCode.INVALID_PARAMS_ERROR, "Unknown tool: %s" % name)

	_last_tool_id += 1
	var id: String = "mcp:" + str(_last_tool_id)
	tool_use_requested.emit(id, name, args)

	var result: ToolManager.ToolResult = tools.execute_tool(name, args)
	if result.is_done():
		return _process_tool_result(id, result)

	# Handle async results.
	var async_result = JSONRPCDispatcher.AsyncResult.new()
	result.completed.connect(func (_content):
		async_result.resolve(_process_tool_result(id, result))
	)
	return async_result


func _process_tool_result(p_id: String, p_result: ToolManager.ToolResult):
	var ret := {}

	var content = p_result.content
	if p_result.is_error():
		ret['isError'] = true

	var s: String
	if content is String:
		s = content
	else:
		s = JSON.stringify(content)
		ret['structuredContent'] = content

	ret['content'] = [
		{
			type = "text",
			text = s,
		}
	]

	var emit_signal = func():
		tool_use_completed.emit(p_id, content)
	emit_signal.call_deferred()

	return ret


# @todo Should this use its own thread, so it's not affected by "low processor mode"?
func _process(_delta) -> void:
	while _tcp_server.is_connection_available():
		# With WebSockets, we only allow one connection at a time, so force disconnect.
		if _transport == Transport.WEBSOCKET and _peers.size() > 0:
			var tcp := _tcp_server.take_connection()
			tcp.disconnect_from_host()
			continue

		_last_peer_id += 1

		var peer := Peer.new(_last_peer_id)
		peer.tcp_peer = _tcp_server.take_connection()

		if _transport == Transport.WEBSOCKET:
			var ws := WebSocketPeer.new()
			# @todo Should this be configurable?
			# 2mb outbound buffer.
			ws.outbound_buffer_size = 1024 * 1024 * 2
			ws.accept_stream(peer.tcp_peer)
			peer.websocket_peer = ws

		_add_peer(peer)

	if _transport == Transport.WEBSOCKET:
		_process_websocket_peers()
	else:
		_process_http_peers()


func _process_websocket_peers() -> void:
	for peer_id in _peers.keys():
		var peer := _peers[peer_id]
		var ws: WebSocketPeer = peer.websocket_peer

		ws.poll()

		var peer_state := ws.get_ready_state()
		if peer_state == WebSocketPeer.STATE_OPEN:
			while ws.get_available_packet_count():
				var packet := ws.get_packet()
				if ws.was_string_packet():
					var packet_text = packet.get_string_from_utf8()
					var response = await _jsonrpc.process_string(packet_text)
					if response != "":
						ws.send_text(response)
		elif peer_state == WebSocketPeer.STATE_CLOSED:
			_remove_peer(peer)

			# If we are stopping and the last peer closed the last peer closed, then we can consider the whole
			# server closed.
			if _server_state == ServerState.STOPPING and _peers.size() == 0:
				_stop_server_complete()


func _add_peer(p_peer: Peer) -> void:
	#print("Add peer: ", p_peer.peer_id)
	_peers[p_peer.peer_id] = p_peer


func _remove_peer(p_peer: Peer) -> void:
	#print("Remove peer: ", p_peer.peer_id)
	_peers.erase(p_peer.peer_id)

	# @todo How to detect "disconnect" with the HTTP transport? Timeout?

	if _transport == Transport.WEBSOCKET:
		_client_info = {}
		_client_state = ClientState.NOT_CONNECTED
		client_state_changed.emit(_client_state)

		# The next client to connect could be a different AI agent, so make it
		# re-read scripts before it can overwrite them. We can't do this for
		# the HTTP transport, which doesn't keep a persistent connection.
		Utils.clear_script_reads()


func _process_http_peers() -> void:
	for peer_id in _peers.keys():
		var peer := _peers[peer_id]
		var tcp: StreamPeerTCP = peer.tcp_peer

		tcp.poll()

		if tcp.get_status() != StreamPeerTCP.STATUS_CONNECTED:
			_remove_peer(peer)
			continue

		var available_bytes := tcp.get_available_bytes()
		if available_bytes > 0:
			peer.buffer += tcp.get_utf8_string(available_bytes)
			_try_handle_http_request(peer)



func _try_handle_http_request(p_peer: Peer) -> void:
	#print(p_peer.buffer.replace("\r", ""))

	var header_end_pos := p_peer.buffer.find("\r\n\r\n")
	if header_end_pos == -1:
		return

	var headers_buf := p_peer.buffer.substr(0, header_end_pos)
	var headers_lines := headers_buf.split("\r\n")

	var req_parts := headers_lines[0].split(' ')
	if req_parts.size() != 3:
		_send_http_error_response(p_peer, 400, "Bad Request")
		return
	if req_parts[2] != "HTTP/1.1":
		_send_http_error_response(p_peer, 505, "HTTP Version Not Supported")
		return
	var method: String = req_parts[0]
	if not method in ["OPTIONS", "POST"]:
		_send_http_error_response(p_peer, 405, "Method Not Allowed")
		return

	var headers := _parse_http_headers(headers_lines)

	var content_length_str: String = headers.get('content-length', '')
	var content_length: int = 0
	if content_length_str.is_valid_int():
		content_length = content_length_str.to_int()

	var body: String = p_peer.buffer.substr(header_end_pos + 4)
	if body.to_utf8_buffer().size() < content_length:
		# We don't have the full body yet.
		return

	p_peer.consume_buffer(headers_buf.length() + 4 + body.length())
	_handle_http_request(p_peer, req_parts[0], headers, body)


func _handle_http_request(p_peer: Peer, p_method: String, p_headers: Dictionary, p_body: String) -> void:
	# Some default headers we always return.
	var headers := {
		'Access-Control-Allow-Origin': '*',
		'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
		'Access-Control-Allow-Headers': 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization,MCP-Protocol-Version',
		'Access-Control-Expose-Headers': 'Content-Length,Content-Range',
	}

	if p_method == "OPTIONS":
		_send_http_response(p_peer, "200 OK", headers)
		return

	_send_http_response(p_peer, "200 OK", headers, 'application/json', await _jsonrpc.process_string(p_body))


func _parse_http_headers(p_lines: PackedStringArray) -> Dictionary:
	var headers := {}
	for line in p_lines:
		var parts := line.split(": ", true, 2)
		if parts.size() != 2:
			continue
		headers[parts[0].to_lower()] = parts[1]

	return headers


func _send_http_response(p_peer: Peer, p_status: String, p_headers: Dictionary = {}, p_content_type: String = "text/plain", p_response_body: String = "") -> void:
	var body_utf8 = p_response_body.to_utf8_buffer()

	var headers = "HTTP/1.1 %s\r\n" % p_status
	for header_name in p_headers:
		headers += "%s: %s\r\n" % [header_name, p_headers[header_name]]
	if body_utf8.size() > 0:
		headers += "Content-Type: %s\r\n" % p_content_type
	headers += "Content-Length: %d\r\n" % body_utf8.size()
	headers += "Connection: close\r\n\r\n"

	var tcp: StreamPeerTCP = p_peer.tcp_peer
	tcp.put_data(headers.to_ascii_buffer())
	tcp.put_data(body_utf8)
	tcp.disconnect_from_host()



func _send_http_error_response(p_peer: Peer, p_status_code: int, p_status_text: String) -> void:
	_send_http_response(p_peer, "%d %s" % [p_status_code, p_status_text], {}, "text/plain", p_status_text)

