extends GutTest

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")

var _tools: ToolManager
var _busy := false
var _executions: Array[String]


func before_each() -> void:
	_busy = false
	_executions = []
	_tools = ToolManager.new()
	_tools.is_busy = func () -> bool: return _busy
	_tools.register_tool(ToolManager.CallbackTool.new("echo", "Echo", "", _echo, {type = "object", properties = {}}))
	_tools.register_tool(ToolManager.CallbackTool.new("fail", "Fail", "", _fail, {type = "object", properties = {}}))


func _echo(p_input: Dictionary) -> ToolManager.ToolResult:
	_executions.append("echo")
	return ToolManager.ToolResult.resolved(p_input)


func _fail(_input: Dictionary) -> ToolManager.ToolResult:
	_executions.append("fail")
	return ToolManager.ToolResult.rejected({errors = ["nope"]})


func test_callback_tool_keeps_its_schemas() -> void:
	var output_schema := {type = "object", properties = {code = {type = "string"}}}
	var tool := ToolManager.CallbackTool.new("t", "T", "", _echo, {type = "object", properties = {}}, output_schema)
	assert_eq(tool.output_schema, output_schema)
	assert_eq(_tools.get_tool("echo").output_schema, ToolManager.OUTPUT_SCHEMA_STRING)


func test_executes_immediately_when_not_busy() -> void:
	var result := _tools.execute_tool("echo", {value = 1})
	assert_true(result.is_done())
	assert_eq(result.content, {value = 1})


func test_waits_until_no_longer_busy() -> void:
	_busy = true

	var result := _tools.execute_tool("echo", {value = 1})
	await wait_process_frames(2)
	assert_false(result.is_done())
	assert_eq(_executions, [] as Array[String])

	_busy = false
	await wait_process_frames(2)
	assert_true(result.is_done())
	assert_eq(result.content, {value = 1})


func test_calls_made_while_busy_run_in_order_afterwards() -> void:
	_busy = true

	var first := _tools.execute_tool("echo", {value = 1})
	var second := _tools.execute_tool("fail", {})
	var third := _tools.execute_tool("echo", {value = 3})
	await wait_process_frames(2)
	assert_eq(_executions, [] as Array[String])

	_busy = false
	await wait_process_frames(3)
	assert_eq(_executions, ["echo", "fail", "echo"] as Array[String])
	assert_eq(first.content, {value = 1})
	assert_true(second.is_error())
	assert_eq(second.content, {errors = ["nope"]})
	assert_eq(third.content, {value = 3})


func _make_image() -> Image:
	return Image.create(2, 2, false, Image.FORMAT_RGB8)


func test_attachments_survive_the_queue() -> void:
	var image := _make_image()
	_tools.register_tool(ToolManager.CallbackTool.new("shot", "Shot", "", func (_input): return ToolManager.ToolResult.resolved_with_attachments("ok", [image]), {type = "object", properties = {}}))
	_busy = true

	var result := _tools.execute_tool("shot", {})
	_busy = false
	await wait_process_frames(2)

	assert_true(result.is_done())
	assert_eq(result.content, "ok")
	assert_eq(result.attachments, [image])


func test_attachments_survive_execute_next_frame() -> void:
	var image := _make_image()
	var tool := ToolManager.CallbackTool.new("t", "T", "", _echo, {type = "object", properties = {}})

	var result := tool.execute_next_frame(func (): return ToolManager.ToolResult.resolved_with_attachments("ok", [image]))
	await wait_process_frames(2)

	assert_true(result.is_done())
	assert_eq(result.attachments, [image])


func test_resolve_with_attachments_after_done_is_ignored() -> void:
	var result := ToolManager.ToolResult.rejected("nope")
	result.resolve_with_attachments("ok", [_make_image()])

	assert_true(result.is_error())
	assert_eq(result.content, "nope")
	assert_eq(result.attachments, [])
