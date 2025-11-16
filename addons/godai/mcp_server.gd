extends Node

const ToolManager = preload("res://addons/godai/tool_manager.gd")
const JSONRPCDispatcher = preload("res://addons/godai/jsonrpc_dispatcher.gd")

const PROTOCOL_VERSION = "2025-06-18"
const GODAI_VERSION = "0.1.0"

enum Transport {
	WEBSOCKET,
	HTTP,
}

enum ServerState {
	STARTED,
	STOPPING,
	STOPPED,
}
class Peer extends RefCounted:
	var tcp_peer: StreamPeerTCP
	var websocket_peer: WebSocketPeer
	var buffer: String
	var buffer_bytes: int

	func consume_buffer(p_amount: int) -> void:
		buffer = buffer.substr(p_amount)


var _tcp_server: TCPServer
var _peers: Dictionary[int, Peer]
var _server_state: ServerState = ServerState.STOPPED
var _transport: Transport = Transport.WEBSOCKET
var _last_peer_id := 1

var _jsonrpc := JSONRPCDispatcher.new()

var tools: ToolManager

func _init(p_tools: ToolManager) -> void:
	tools = p_tools

	set_process(false)

	_jsonrpc.set_method("initialize", _rpc_initialize)
	_jsonrpc.set_method("notifications/initialized", _rpc_client_initialized)
	_jsonrpc.set_method("tools/list", _rpc_list_tools)
	_jsonrpc.set_method("tools/call", _rpc_call_tool)


func get_server_state() -> ServerState:
	return _server_state


func get_transport() -> Transport:
	return _transport


func get_peer_count() -> int:
	return _peers.size()


func start_server(p_port: int, p_transport: Transport) -> Error:
	if _server_state != ServerState.STOPPED:
		return ERR_ALREADY_IN_USE

	_transport = p_transport

	_tcp_server = TCPServer.new()
	var err = _tcp_server.listen(p_port)
	set_process(err == OK)
	return err


func stop_server() -> void:
	if not _server_state == ServerState.STARTED:
		return

	_tcp_server.stop()

	for peer in _peers.values():
		if _transport == Transport.WEBSOCKET:
			peer.websocket_peer.close()
		else:
			peer.tcp_peer.disconnect_from_host()

	if _transport == Transport.WEBSOCKET:
		_server_state = ServerState.STOPPING
	else:
		_peers.clear()
		_stop_server_complete()


func _stop_server_complete() -> void:
	_server_state = ServerState.STOPPED
	_tcp_server = null
	set_process(false)


func _rpc_initialize(p_params: Dictionary):
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
			# @todo Add a title to ToolManager.Tool!
			title = tool_obj.name,
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

	var result: ToolManager.ToolResult = tools.execute_tool(name, args)
	if result.is_done():
		return _process_tool_result(result.content)

	# Handle async results.
	var async_result = JSONRPCDispatcher.AsyncResult.new()
	result.completed.connect(func (content):
		async_result.resolve(_process_tool_result(content))
	)
	return async_result


func _process_tool_result(p_content):
	var ret := {}

	var s: String
	if p_content is String:
		s = p_content
	else:
		s = JSON.stringify(p_content)
		ret['structuredContent'] = p_content

	ret['content'] = [
		{
			type = "text",
			text = s,
		}
	]

	return ret


# @todo Should this use its own thread, so it's not affected by "low processor mode"?
func _process(_delta) -> void:
	while _tcp_server.is_connection_available():
		_last_peer_id += 1
		#print("+ Peer %d connected." % _last_peer_id)

		var peer := Peer.new()
		if _transport == Transport.WEBSOCKET:
			peer.websocket_peer = WebSocketPeer.new()
			peer.websocket_peer.accept_stream(_tcp_server.take_connection())
		else:
			peer.tcp_peer = _tcp_server.take_connection()

		_peers[_last_peer_id] = peer

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
					print("RECEIVED: ", packet_text)
					var response = await _jsonrpc.process_string(packet_text)
					if response != "":
						ws.send_text(response)
		elif peer_state == WebSocketPeer.STATE_CLOSED:
			_peers.erase(peer_id)
			var code = ws.get_close_code()
			var reason = ws.get_close_reason()
			#print("- Peer %d closed with code %d, reason %s. Clean: %s" % [peer_id, code, reason, code != -1])

			# If we are stopping and the last peer closed the last peer closed, then we can consider the whole
			# server closed.
			if _server_state == ServerState.STOPPING and _peers.size() == 0:
				_stop_server_complete()


func _process_http_peers() -> void:
	for peer_id in _peers.keys():
		var peer := _peers[peer_id]
		var tcp: StreamPeerTCP = peer.tcp_peer

		tcp.poll()

		if tcp.get_status() != StreamPeerTCP.STATUS_CONNECTED:
			_peers.erase(peer_id)
			#print("- Peer %d disconnected" % peer_id)
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

