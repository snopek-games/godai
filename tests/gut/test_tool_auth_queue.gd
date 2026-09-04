extends GutTest

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")
const DefaultToolsLoader = preload("res://addons/godai/tools/default/loader.gd")
const ToolAuthQueue = preload("res://addons/godai/ui/tool_auth_queue.gd")
const ToolUseAuthDialog = preload("res://addons/godai/ui/tool_use_auth_dialog.gd")
const ToolUseAuthDialogScene = preload("res://addons/godai/ui/tool_use_auth_dialog.tscn")

const READ_ONLY_TOOL := "get_current_project"
const WRITE_TOOL := "set_project_settings"


class Busy extends RefCounted:
	signal changed
	var busy := false

	func set_busy(p_busy: bool) -> void:
		busy = p_busy
		changed.emit.call_deferred()


var _tools: ToolManager
var _tool_auth: ToolAuth
var _busy: Busy
var _dialog: ToolUseAuthDialog
var _queue: ToolAuthQueue


func before_each() -> void:
	_tools = ToolManager.new()
	DefaultToolsLoader.load_default_tools(_tools)
	_tool_auth = ToolAuth.new()
	_busy = Busy.new()
	_dialog = ToolUseAuthDialogScene.instantiate()
	_dialog.visible = false
	add_child_autofree(_dialog)
	_queue = ToolAuthQueue.new(_tools, _tool_auth, _dialog, func (): return _busy.busy, _busy.changed)


func after_each() -> void:
	_queue = null


func test_tool_use_passes_through_when_not_busy() -> void:
	var request = _queue.authorize_when_idle(READ_ONLY_TOOL, {})

	assert_true(request.is_done())
	assert_true(request.allowed)


func test_tool_use_waits_while_busy() -> void:
	_busy.set_busy(true)

	var gated = _queue.authorize_when_idle(READ_ONLY_TOOL, {})
	await wait_frames(2)
	assert_false(gated.is_done())

	_busy.set_busy(false)
	await wait_frames(2)

	assert_true(gated.is_done())
	assert_true(gated.allowed)


func test_tool_use_keeps_waiting_when_still_busy_after_a_change() -> void:
	_busy.set_busy(true)

	var gated = _queue.authorize_when_idle(READ_ONLY_TOOL, {})
	_busy.set_busy(true)
	await wait_frames(2)
	assert_false(gated.is_done())

	_busy.set_busy(false)
	await wait_frames(2)
	assert_true(gated.is_done())


func test_headless_denies_write_tools_without_asking() -> void:
	var request = _queue.authorize(WRITE_TOOL, {})

	assert_true(request.is_done())
	assert_false(request.allowed)
	assert_false(_dialog.visible)
	assert_engine_error("Denying use of the 'set_project_settings' tool")


func test_write_tool_requests_queue_and_advance_through_the_dialog() -> void:
	_queue._headless = false
	var first = _queue.authorize(WRITE_TOOL, {settings = {a = "1"}})
	var second = _queue.authorize(WRITE_TOOL, {settings = {b = "2"}})

	assert_false(first.is_done())
	assert_false(second.is_done())
	assert_true(_dialog.visible)
	assert_eq(_dialog.name_field.text, WRITE_TOOL)

	_dialog.tool_use_allowed.emit(ToolUseAuthDialog.AllowDenyType.ONCE)
	assert_true(first.is_done())
	assert_true(first.allowed)
	assert_false(second.is_done(), "the dialog moves on to the next request")
	assert_true(_dialog.visible)

	_dialog.tool_use_denied.emit(ToolUseAuthDialog.AllowDenyType.ONCE)
	assert_true(second.is_done())
	assert_false(second.allowed)
	assert_false(_dialog.visible)

	assert_engine_error("spawned at invalid position", "popping the dialog headless is fine")


func test_allow_for_session_drains_queued_requests_for_the_tool() -> void:
	_queue._headless = false
	var first = _queue.authorize(WRITE_TOOL, {})
	var second = _queue.authorize(WRITE_TOOL, {})

	_dialog.tool_use_allowed.emit(ToolUseAuthDialog.AllowDenyType.TOOL_FOR_SESSION)

	assert_true(first.allowed)
	assert_true(second.is_done())
	assert_true(second.allowed)
	assert_false(_dialog.visible)

	var third = _queue.authorize(WRITE_TOOL, {})
	assert_true(third.is_done(), "the session decision answers without asking")
	assert_true(third.allowed)

	assert_engine_error("spawned at invalid position", "popping the dialog headless is fine")


func test_cancel_pending_denies_everything_and_hides_the_dialog() -> void:
	_queue._headless = false
	var first = _queue.authorize(WRITE_TOOL, {})
	var second = _queue.authorize(WRITE_TOOL, {})

	_queue.cancel_pending()

	assert_true(first.is_done())
	assert_false(first.allowed)
	assert_true(second.is_done())
	assert_false(second.allowed)
	assert_false(_dialog.visible)

	assert_engine_error("spawned at invalid position", "popping the dialog headless is fine")


func test_cancel_external_denies_a_request_gated_behind_a_busy_chat() -> void:
	_queue._headless = false
	_busy.set_busy(true)
	var gated = _queue.authorize_when_idle(WRITE_TOOL, {})
	assert_false(gated.is_done())

	_queue.cancel_external()

	assert_true(gated.is_done())
	assert_false(gated.allowed)

	_busy.set_busy(false)
	await wait_frames(2)
	assert_false(_dialog.visible, "no auth dialog for the disconnected client")


func test_cancel_external_leaves_other_pending_requests_alone() -> void:
	_queue._headless = false
	var editor_request = _queue.authorize(WRITE_TOOL, {})
	var external_request = _queue.authorize_when_idle(WRITE_TOOL, {})

	_queue.cancel_external()

	assert_true(external_request.is_done())
	assert_false(external_request.allowed)
	assert_false(editor_request.is_done())
	assert_true(_dialog.visible)

	assert_engine_error("spawned at invalid position", "popping the dialog headless is fine")


func test_gated_tool_use_can_time_out_while_waiting() -> void:
	_busy.set_busy(true)

	var gated = _queue.authorize_when_idle(READ_ONLY_TOOL, {})
	gated.time_out()

	_busy.set_busy(false)
	await wait_frames(2)

	assert_true(gated.timed_out)
	assert_false(gated.allowed)
