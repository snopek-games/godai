extends RefCounted

## Helper for running `godai-eval` against the AI agent in the editor.
##
## It reads the prompt from a file, and outputs a stream of events that are
## meant to match Claude Code's `--output-format stream-json`.

const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const MCPServer = preload("res://addons/godai/mcp/mcp_server.gd")

const PROMPT_ENV = "GODAI_EVAL_PROMPT_FILE"
const STREAM_ENV = "GODAI_EVAL_STREAM_FILE"

const POLL_SECONDS := 0.25

var _godai_panel
var _file: FileAccess
var _session_id: String
var _started_msec: int
var _turns: int
var _usage: Dictionary
var _stop_reason: String
var _error: ClaudeClient.ResponseError


static func should_run() -> bool:
	return not OS.get_environment(PROMPT_ENV).is_empty() \
		and not OS.get_environment(STREAM_ENV).is_empty()


static func start(p_godai_panel):
	var runner = new()
	runner._run(p_godai_panel)
	return runner


func _run(p_godai_panel) -> void:
	_godai_panel = p_godai_panel

	# After restart_editor, the relaunched editor's chat is already resumed, so
	# events append to the stream instead of starting a second one.
	var resumed_request: ClaudeClient.Request = _godai_panel._current_request

	var path := OS.get_environment(STREAM_ENV)
	var stream_already_exists := FileAccess.file_exists(path)
	if stream_already_exists:
		_file = FileAccess.open(path, FileAccess.READ_WRITE)
		if _file:
			_file.seek_end()
	else:
		_file = FileAccess.open(path, FileAccess.WRITE)
	if not _file:
		push_error("Godai eval: cannot write %s: %s" % [path, error_string(FileAccess.get_open_error())])
		return

	var rng := RandomNumberGenerator.new()
	rng.randomize()
	_session_id = "%08x%08x" % [rng.randi(), rng.randi()]
	_started_msec = Time.get_ticks_msec()

	var client: ClaudeClient = _godai_panel.claude_client
	_write({
		type = "system",
		subtype = "init",
		session_id = _session_id,
		model = client.model,
		tools = _godai_panel.tools.tools.keys(),
		effort = client.effort,
		max_tokens = client.max_tokens,
	})

	if resumed_request:
		_on_chat_started(_godai_panel._current_session.chat, resumed_request)
		return

	if stream_already_exists:
		_on_completed(ClaudeClient.Response.new(null, ClaudeClient.ResponseError.new(
			"resume_failed", "The restarted editor could not resume the chat.")))
		return

	var prompt := await _await_prompt()

	_godai_panel.chat_started.connect(_on_chat_started)
	_godai_panel.submit_prompt(prompt)


func _await_prompt() -> String:
	var path := OS.get_environment(PROMPT_ENV)
	while true:
		if FileAccess.file_exists(path) \
			and _godai_panel.mcp_server.get_client_state() == MCPServer.ClientState.NOT_CONNECTED:
			return FileAccess.get_file_as_string(path)
		await _godai_panel.get_tree().create_timer(POLL_SECONDS).timeout
	return ""


func _on_chat_started(p_chat: ClaudeClient.Chat, p_request: ClaudeClient.Request) -> void:
	p_chat.message_added.connect(_on_message_added)
	p_request.response_received.connect(_on_response_received)
	p_request.completed.connect(_on_completed)


func _on_message_added(p_msg: ClaudeClient.Message) -> void:
	if p_msg.role == "assistant":
		_turns += 1
	_write({
		type = "assistant" if p_msg.role == "assistant" else "user",
		session_id = _session_id,
		message = p_msg.to_dict(),
	})


func _on_response_received(p_response: ClaudeClient.Response) -> void:
	if p_response.payload is Dictionary:
		_stop_reason = p_response.payload.get("stop_reason", _stop_reason)
		var usage = p_response.payload.get("usage", {})
		if usage is Dictionary:
			for key in usage:
				if usage[key] is float or usage[key] is int:
					_usage[key] = int(_usage.get(key, 0) + usage[key])


# The harness reads the "editor_teardown" subtype as non-final: it records the
# segment's turns/usage but keeps waiting for a relaunched editor's result.
func flush_teardown_result() -> void:
	if not _file:
		return

	_write({
		type = "result",
		subtype = "editor_teardown",
		session_id = _session_id,
		is_error = false,
		result = "",
		stop_reason = _stop_reason,
		num_turns = _turns,
		duration_ms = Time.get_ticks_msec() - _started_msec,
		total_cost_usd = 0.0,
		usage = _usage,
	})
	_file.close()
	_file = null


func _on_completed(p_response: ClaudeClient.Response) -> void:
	if p_response.is_error():
		_error = p_response.get_error()

	var client: ClaudeClient = _godai_panel.claude_client

	_write({
		dropped_options = client.dropped_options(client.model),
		type = "result",
		subtype = "error_during_execution" if _error else "success",
		session_id = _session_id,
		is_error = _error != null,
		result = _error.message if _error else "",
		stop_reason = _stop_reason,
		num_turns = _turns,
		duration_ms = Time.get_ticks_msec() - _started_msec,
		# The Messages API doesn't price a request, so the harness compares
		# tokens on this surface.
		total_cost_usd = 0.0,
		usage = _usage,
	})

	if _file:
		_file.close()
		_file = null


func _write(p_event: Dictionary) -> void:
	if not _file:
		return
	_file.store_line(JSON.stringify(p_event))
	_file.flush()
