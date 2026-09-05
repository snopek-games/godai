extends RefCounted

const Chat = preload("res://addons/godai/chat/chat.gd")
const Utils = preload("res://addons/godai/utils.gd")

enum ClientKind {
	EDITOR,
	CLI,
	MCP,
}

const PROJECT_PATH := ".godot/godai-chat-sessions"
const CLI_CHAT_ID_SUFFIX := "-cli"
const MCP_CHAT_ID_SUFFIX := "-mcp"

const FLUSH_DELAY_SECONDS := 1.0
const FORMAT_VERSION := 1


class ChatSession extends RefCounted:
	var id: String
	var chat: Chat
	var client_kind: ClientKind
	var persisted_message_count := 0

	func _init(p_id: String, p_chat: Chat, p_client_kind: ClientKind) -> void:
		id = p_id
		chat = p_chat
		client_kind = p_client_kind

	func is_external() -> bool:
		return client_kind != ClientKind.EDITOR


var _base_path: String
var _sessions: Dictionary
var _dirty: Dictionary
var _flush_queued := false


func _init(p_base_path := "") -> void:
	_base_path = p_base_path if not p_base_path.is_empty() else Utils.get_project_path() + "/" + PROJECT_PATH


func _notification(p_what: int) -> void:
	if p_what == NOTIFICATION_PREDELETE:
		# flush() can't be called here: a dying RefCounted converts to a null Variant,
		# so instance method calls on self fail during NOTIFICATION_PREDELETE.
		_write_sessions(_base_path, _dirty)


func create_session(p_client_kind := ClientKind.EDITOR) -> ChatSession:
	var id := generate_chat_id()
	match p_client_kind:
		ClientKind.CLI:
			id += CLI_CHAT_ID_SUFFIX
		ClientKind.MCP:
			id += MCP_CHAT_ID_SUFFIX
	var session := ChatSession.new(id, Chat.new(), p_client_kind)
	_register(session)
	return session


func load_session(p_id: String) -> ChatSession:
	if _sessions.has(p_id):
		return _sessions[p_id]

	var file := FileAccess.open(session_file_path(p_id), FileAccess.READ)
	if not file:
		return null

	var chat := _parse_session(file)
	if not chat:
		return null

	var kind := kind_from_chat_id(p_id)
	if kind == ClientKind.EDITOR:
		# A chat with a dangling tool use would error if the user attempted to resume it.
		chat.repair_dangling_tool_use()

	var session := ChatSession.new(p_id, chat, kind)
	# Counted after the repair which may add messages.
	session.persisted_message_count = chat.messages.size()
	_register(session)
	return session


static func _parse_session(p_file: FileAccess) -> Chat:
	var chat := Chat.new()
	var header_seen := false
	while not p_file.eof_reached():
		var line := p_file.get_line()
		if line.strip_edges().is_empty():
			continue
		var data = JSON.parse_string(line)
		if not data is Dictionary:
			push_error("Chat session line is not a %s: %s" % ["message" if header_seen else "header", line])
			return null
		if not header_seen:
			header_seen = true
			if int(data.get("version", 0)) != FORMAT_VERSION:
				push_error("Unsupported chat session format: %s" % line)
				return null
			continue
		var msg := Chat.Message.from_dict(data)
		if not msg:
			return null
		chat.messages.push_back(msg)
	return chat


func _register(p_session: ChatSession) -> void:
	_sessions[p_session.id] = p_session

	var store_wr := weakref(self)
	var session_wr := weakref(p_session)
	p_session.chat.message_added.connect(func (_msg: Chat.Message):
		var store = store_wr.get_ref()
		var session = session_wr.get_ref()
		if store and session:
			store.save(session))


func unload_session(p_id: String) -> void:
	if _dirty.has(p_id):
		flush()
	_sessions.erase(p_id)


func list_session_ids() -> PackedStringArray:
	var ids := PackedStringArray()

	var dir := DirAccess.open(_base_path)
	if dir:
		dir.list_dir_begin()
		var fn := dir.get_next()
		while fn != "":
			if not dir.current_is_dir() and fn.ends_with(".jsonl"):
				ids.push_back(fn.trim_suffix(".jsonl"))
			fn = dir.get_next()

	var sorted := Array(ids)
	sorted.sort_custom(func (a: String, b: String): return a.casecmp_to(b) > 0)
	return PackedStringArray(sorted)


func session_file_path(p_id: String) -> String:
	return _session_file_path(_base_path, p_id)


static func _session_file_path(p_base_path: String, p_id: String) -> String:
	return "%s/%s.jsonl" % [p_base_path, p_id]


func save(p_session: ChatSession) -> void:
	_dirty[p_session.id] = p_session
	if _flush_queued:
		return
	_flush_queued = true
	var tree := Engine.get_main_loop() as SceneTree
	if tree:
		tree.create_timer(FLUSH_DELAY_SECONDS).timeout.connect(flush)
	else:
		flush.call_deferred()


func flush() -> void:
	_flush_queued = false
	var dirty := _dirty
	_dirty = {}
	_write_sessions(_base_path, dirty)


static func _write_sessions(p_base_path: String, p_dirty: Dictionary) -> void:
	if p_dirty.is_empty():
		return

	if not DirAccess.dir_exists_absolute(p_base_path):
		DirAccess.make_dir_recursive_absolute(p_base_path)

	for session in p_dirty.values():
		var path := _session_file_path(p_base_path, session.id)
		var f: FileAccess
		if FileAccess.file_exists(path):
			f = FileAccess.open(path, FileAccess.READ_WRITE)
			if f:
				f.seek_end()
		else:
			f = FileAccess.open(path, FileAccess.WRITE)
			if f:
				f.store_line(JSON.stringify({version = FORMAT_VERSION}))
		if not f:
			continue
		for i in range(session.persisted_message_count, session.chat.messages.size()):
			f.store_line(JSON.stringify(session.chat.messages[i].to_dict()))
		session.persisted_message_count = session.chat.messages.size()


static func generate_chat_id() -> String:
	var unix := Time.get_unix_time_from_system()
	var bias: int = Time.get_time_zone_from_system()["bias"]
	return "%s-%03d" % [Time.get_datetime_string_from_unix_time(int(unix) + bias * 60).replace(":", "-"), int(fmod(unix, 1.0) * 1000)]


static func kind_from_chat_id(p_id: String) -> ClientKind:
	if p_id.ends_with(CLI_CHAT_ID_SUFFIX):
		return ClientKind.CLI
	if p_id.ends_with(MCP_CHAT_ID_SUFFIX):
		return ClientKind.MCP
	return ClientKind.EDITOR


static func chat_id_to_label(p_id: String) -> String:
	match kind_from_chat_id(p_id):
		ClientKind.CLI:
			return chat_id_to_label(p_id.trim_suffix(CLI_CHAT_ID_SUFFIX)) + " (CLI)"
		ClientKind.MCP:
			return chat_id_to_label(p_id.trim_suffix(MCP_CHAT_ID_SUFFIX)) + " (MCP)"

	var halves: PackedStringArray = p_id.split("T")
	if halves.size() != 2:
		return p_id

	var time: PackedStringArray = halves[1].split("-")
	if time.size() != 4:
		return p_id

	var hour = int(time[0])
	var minute = int(time[1])
	var am_pm = "am" if hour < 12 else "pm"

	if hour == 0:
		hour = 12
	elif hour > 12:
		hour -= 12

	return "%s @ %d:%02d%s" % [halves[0], hour, minute, am_pm]
