extends GutTest

const GodaiPanelScene = preload("res://addons/godai/ui/godai_panel.tscn")
const ClaudeClient = preload("res://addons/godai/client/claude_client.gd")
const EvalRun = preload("res://addons/godai/eval_run.gd")

const SEGMENT_1_INIT := '{"type":"system","subtype":"init","session_id":"segment1"}'

var _stream_path: String


func before_each() -> void:
	_stream_path = OS.get_cache_dir() + "/godai-test-eval-stream-%d.jsonl" % Time.get_ticks_usec()


func after_each() -> void:
	OS.set_environment(EvalRun.STREAM_ENV, "")
	OS.set_environment(EvalRun.PROMPT_ENV, "")
	if FileAccess.file_exists(_stream_path):
		DirAccess.remove_absolute(_stream_path)


func _make_panel() -> Control:
	var panel: Control = GodaiPanelScene.instantiate()
	add_child_autofree(panel)
	return panel


func _write_segment_1() -> void:
	var f := FileAccess.open(_stream_path, FileAccess.WRITE)
	f.store_line(SEGMENT_1_INIT)
	f.close()


func test_failed_resume_appends_an_error_result_instead_of_truncating() -> void:
	var panel := _make_panel()
	_write_segment_1()
	OS.set_environment(EvalRun.STREAM_ENV, _stream_path)

	var eval_run = EvalRun.start(panel)

	assert_null(eval_run._file, "the stream is closed after the final result")
	var lines := FileAccess.get_file_as_string(_stream_path).strip_edges().split("\n")
	assert_eq(lines.size(), 3)
	assert_eq(lines[0], SEGMENT_1_INIT)
	assert_string_contains(lines[1], '"subtype":"init"')
	assert_string_contains(lines[2], '"subtype":"error_during_execution"')
	assert_string_contains(lines[2], "could not resume")
	assert_null(panel._current_session, "the prompt is not re-submitted")


func test_resumed_chat_appends_to_the_stream() -> void:
	var panel := _make_panel()
	_write_segment_1()
	OS.set_environment(EvalRun.STREAM_ENV, _stream_path)

	panel._current_session = panel._session_store.create_session()
	panel._set_current_request(ClaudeClient.Request.new(panel._current_session.chat))

	var eval_run = EvalRun.start(panel)

	assert_not_null(eval_run._file, "the stream stays open for the resumed chat")
	var lines := FileAccess.get_file_as_string(_stream_path).strip_edges().split("\n")
	assert_eq(lines.size(), 2)
	assert_eq(lines[0], SEGMENT_1_INIT)
	assert_string_contains(lines[1], '"subtype":"init"')
