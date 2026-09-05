extends GutTest

const ChatSessionStore = preload("res://addons/godai/chat/chat_session_store.gd")
const Chat = preload("res://addons/godai/chat/chat.gd")

var _temp_dir: String
var _store: ChatSessionStore


func before_each() -> void:
	_temp_dir = OS.get_cache_dir() + "/godai-test-chat-sessions-%d" % Time.get_ticks_usec()
	_store = ChatSessionStore.new(_temp_dir)


func after_each() -> void:
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


func _write_session_file(p_id: String, p_lines: Array = [], p_header = {version = ChatSessionStore.FORMAT_VERSION}) -> void:
	DirAccess.make_dir_recursive_absolute(_temp_dir)
	var f := FileAccess.open(_store.session_file_path(p_id), FileAccess.WRITE)
	if p_header != null:
		f.store_line(JSON.stringify(p_header))
	for line in p_lines:
		f.store_line(JSON.stringify(line))
	f.close()


func _content_types(p_msg: Chat.Message) -> Array:
	return p_msg.content.map(func (c): return c.to_dict()["type"])


func test_generate_chat_id_format() -> void:
	var id := ChatSessionStore.generate_chat_id()
	assert_true(id.contains("T"))
	assert_eq(ChatSessionStore.chat_id_to_label(id).split(" @ ").size(), 2)


func test_chat_id_to_label() -> void:
	assert_eq(ChatSessionStore.chat_id_to_label("2026-08-30T14-30-00-123"), "2026-08-30 @ 2:30pm")
	assert_eq(ChatSessionStore.chat_id_to_label("2026-08-30T14-30-00-123-cli"), "2026-08-30 @ 2:30pm (CLI)")
	assert_eq(ChatSessionStore.chat_id_to_label("2026-08-30T14-30-00-123-mcp"), "2026-08-30 @ 2:30pm (MCP)")
	assert_eq(ChatSessionStore.chat_id_to_label("2026-08-30T00-15-00-123"), "2026-08-30 @ 12:15am")
	assert_eq(ChatSessionStore.chat_id_to_label("2026-08-30T12-05-00-123"), "2026-08-30 @ 12:05pm")
	assert_eq(ChatSessionStore.chat_id_to_label("not-a-timestamp"), "not-a-timestamp")


func test_create_session_kinds() -> void:
	assert_false(_store.create_session().is_external())

	var cli = _store.create_session(ChatSessionStore.ClientKind.CLI)
	assert_true(cli.is_external())
	assert_true(cli.id.ends_with(ChatSessionStore.CLI_CHAT_ID_SUFFIX))

	var mcp = _store.create_session(ChatSessionStore.ClientKind.MCP)
	assert_true(mcp.is_external())
	assert_true(mcp.id.ends_with(ChatSessionStore.MCP_CHAT_ID_SUFFIX))


func test_load_session_derives_kind_from_id() -> void:
	_write_session_file("2026-08-30T14-30-00-123-cli")
	_write_session_file("2026-08-30T14-30-00-124-mcp")
	_write_session_file("2026-08-30T14-30-00-125")

	assert_eq(_store.load_session("2026-08-30T14-30-00-123-cli").client_kind, ChatSessionStore.ClientKind.CLI)
	assert_eq(_store.load_session("2026-08-30T14-30-00-124-mcp").client_kind, ChatSessionStore.ClientKind.MCP)
	assert_eq(_store.load_session("2026-08-30T14-30-00-125").client_kind, ChatSessionStore.ClientKind.EDITOR)


func test_no_file_until_first_message() -> void:
	var session = _store.create_session()
	_store.flush()

	assert_false(FileAccess.file_exists(_store.session_file_path(session.id)))


func test_message_added_saves_after_a_delayed_flush() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	session.chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "hi there"))

	assert_false(FileAccess.file_exists(_store.session_file_path(session.id)))

	await wait_seconds(ChatSessionStore.FLUSH_DELAY_SECONDS + 0.2)

	assert_true(FileAccess.file_exists(_store.session_file_path(session.id)))


func test_save_load_roundtrip() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	session.chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "hi there"))
	_store.flush()

	var other_store := ChatSessionStore.new(_temp_dir)
	var loaded = other_store.load_session(session.id)

	assert_not_null(loaded)
	assert_eq(loaded.chat.messages.size(), 2)
	assert_eq(loaded.chat.messages[0].content[0].text, "hello")


func test_load_session_returns_the_live_object() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))

	assert_eq(_store.load_session(session.id), session)


func test_load_missing_session_returns_null() -> void:
	assert_null(_store.load_session("nope"))


func test_load_malformed_session_returns_null() -> void:
	_write_session_file("not-a-message", ["just a string"])
	_write_session_file("not-a-dict", [[1, 2]])
	_write_session_file("no-role", [{content = []}])
	_write_session_file("no-content", [{role = "user"}])
	_write_session_file("no-header", [{role = "user", content = []}], null)
	_write_session_file("future-version", [{role = "user", content = []}], {version = ChatSessionStore.FORMAT_VERSION + 1})

	assert_null(_store.load_session("not-a-message"))
	assert_null(_store.load_session("not-a-dict"))
	assert_null(_store.load_session("no-role"))
	assert_null(_store.load_session("no-content"))
	assert_null(_store.load_session("no-header"))
	assert_null(_store.load_session("future-version"))
	assert_push_error(6, "each malformed file reports what it is missing")


func test_new_session_file_starts_with_a_version_header() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	_store.flush()

	var lines := FileAccess.get_file_as_string(_store.session_file_path(session.id)).strip_edges().split("\n")
	assert_eq(lines.size(), 2)
	assert_eq(JSON.parse_string(lines[0]), {version = float(ChatSessionStore.FORMAT_VERSION)})
	assert_string_contains(lines[1], "hello")


func test_load_session_repairs_a_dangling_tool_use() -> void:
	_write_session_file("2026-08-30T14-30-00-123", [
		{role = "user", content = [{type = "text", text = "hi"}]},
		{role = "assistant", content = [
			{type = "text", text = "Running the tool."},
			{type = "tool_use", id = "toolu_1", name = "save_scene", input = {}},
		]},
	])

	var loaded = _store.load_session("2026-08-30T14-30-00-123")

	assert_eq(loaded.chat.messages.size(), 3)
	assert_eq(_content_types(loaded.chat.messages[1]), ["text", "tool_use"])
	assert_eq(_content_types(loaded.chat.messages[2]), ["tool_result"])


func test_load_session_keeps_a_dangling_tool_use_in_external_sessions() -> void:
	_write_session_file("2026-08-30T14-30-00-123-mcp", [
		{role = "assistant", content = [
			{type = "tool_use", id = "toolu_1", name = "save_scene", input = {}},
		]},
	])

	var loaded = _store.load_session("2026-08-30T14-30-00-123-mcp")

	assert_eq(loaded.chat.messages.size(), 1)
	assert_eq(_content_types(loaded.chat.messages[0]), ["tool_use"])


func test_loaded_sessions_keep_saving_on_new_messages() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	_store.flush()

	var other_store := ChatSessionStore.new(_temp_dir)
	var loaded = other_store.load_session(session.id)
	loaded.chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "hi there"))
	other_store.flush()

	assert_eq(ChatSessionStore.new(_temp_dir).load_session(session.id).chat.messages.size(), 2)


func test_unload_session_flushes_and_reloads_fresh_from_disk() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))

	_store.unload_session(session.id)

	var loaded = _store.load_session(session.id)
	assert_not_null(loaded)
	assert_ne(loaded, session)  # not the cached object anymore
	assert_eq(loaded.chat.messages.size(), 1)


func test_unloaded_session_still_saves_new_messages() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	_store.unload_session(session.id)

	session.chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "hi there"))
	_store.flush()

	assert_eq(ChatSessionStore.new(_temp_dir).load_session(session.id).chat.messages.size(), 2)


func test_predelete_flushes_dirty_sessions() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	await wait_seconds(ChatSessionStore.FLUSH_DELAY_SECONDS + 0.2)

	var path := _store.session_file_path(session.id)
	DirAccess.remove_absolute(path)

	# save() would queue a deferred flush whose Callable keeps the store alive;
	# dirty with no pending flush is the state a dropped queue leaves at shutdown.
	_store._dirty[session.id] = session
	session.persisted_message_count = 0
	_store = null

	assert_true(FileAccess.file_exists(path))


func test_flush_appends_only_new_messages() -> void:
	var session = _store.create_session()
	session.chat.add_message(Chat.Message.new(Chat.Role.USER, "hello"))
	_store.flush()

	var path := _store.session_file_path(session.id)
	var f := FileAccess.open(path, FileAccess.WRITE)
	f.store_line('{"version": 1}')
	f.store_line('{"role": "user", "content": "mangled externally"}')
	f.close()

	session.chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "hi there"))
	_store.flush()

	var lines := FileAccess.get_file_as_string(path).strip_edges().split("\n")
	assert_eq(lines.size(), 3)
	assert_string_contains(lines[1], "mangled externally",
		"already-persisted messages are never rewritten")
	assert_string_contains(lines[2], "hi there")


func test_repaired_tool_results_stay_out_of_the_file() -> void:
	_write_session_file("2026-08-30T14-30-00-123", [
		{role = "user", content = [{type = "text", text = "restart please"}]},
		{role = "assistant", content = [
			{type = "tool_use", id = "toolu_1", name = "restart_editor", input = {}},
		]},
	])

	var session = _store.load_session("2026-08-30T14-30-00-123")
	assert_eq(session.chat.messages.size(), 3, "the dangling tool use is answered on load")

	session.chat.add_message(Chat.Message.new(Chat.Role.ASSISTANT, "welcome back"))
	_store.flush()

	var lines := FileAccess.get_file_as_string(_store.session_file_path(session.id)).strip_edges().split("\n")
	assert_eq(lines.size(), 4, "the synthetic tool result is recreated on load, not persisted")

	var reloaded = ChatSessionStore.new(_temp_dir).load_session(session.id)
	assert_eq(reloaded.chat.messages.size(), 4)
	assert_true(reloaded.chat.messages[2].content[0] is Chat.ToolResultContent)


func test_list_session_ids_newest_first() -> void:
	_write_session_file("2026-08-30T14-30-00-123")
	_write_session_file("2026-08-31T09-00-00-000")

	assert_eq(_store.list_session_ids(), PackedStringArray(["2026-08-31T09-00-00-000", "2026-08-30T14-30-00-123"]))
