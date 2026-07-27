extends GutTest

const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")

# Long enough that a slow frame can't trip it, short enough to wait out.
const TIMEOUT := 0.1

var parent: Node


func before_each() -> void:
	parent = add_child_autofree(Node.new())


func test_timeout_refuses_an_unanswered_request() -> void:
	var request := ToolAuth.Request.new("save_scene", {})
	request.start_timeout(parent, TIMEOUT)

	await wait_for_signal(request.completed, 1.0)

	assert_true(request.is_done(), "should be done")
	assert_false(request.allowed, "should not be allowed")
	assert_true(request.timed_out, "should be marked as timed out")


func test_answering_frees_the_timer() -> void:
	var request := ToolAuth.Request.new("save_scene", {})
	request.start_timeout(parent, TIMEOUT)
	assert_eq(parent.get_child_count(), 1, "the timer should be running")

	request.resolve(true)

	# queue_free() lands at the end of the frame.
	await wait_process_frames(2)
	assert_eq(parent.get_child_count(), 0, "the timer should be gone")


func test_answering_beats_a_later_timeout() -> void:
	var request := ToolAuth.Request.new("save_scene", {})
	request.start_timeout(parent, TIMEOUT)
	request.resolve(true)

	# Well past when the timer would have fired.
	await wait_seconds(TIMEOUT * 3)

	assert_true(request.allowed, "the answer should stand")
	assert_false(request.timed_out, "should not be marked as timed out")


func test_timing_out_frees_the_timer() -> void:
	var request := ToolAuth.Request.new("save_scene", {})
	request.start_timeout(parent, TIMEOUT)

	await wait_for_signal(request.completed, 1.0)
	await wait_process_frames(2)

	# Freeing from inside the timer's own signal has to work too.
	assert_eq(parent.get_child_count(), 0, "the timer should be gone")


func test_start_timeout_ignores_a_request_already_answered() -> void:
	var request := ToolAuth.Request.resolved("save_scene", {}, true)
	request.start_timeout(parent, TIMEOUT)

	assert_eq(parent.get_child_count(), 0, "no timer should have been created")


func test_start_timeout_ignores_a_non_positive_timeout() -> void:
	var request := ToolAuth.Request.new("save_scene", {})
	request.start_timeout(parent, 0.0)

	assert_eq(parent.get_child_count(), 0, "no timer should have been created")
	assert_false(request.is_done(), "the request should still be waiting")
