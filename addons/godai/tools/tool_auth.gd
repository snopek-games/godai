extends RefCounted

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")

const DENIED_MESSAGE = "The user denied permission to use the '%s' tool."
const TIMED_OUT_MESSAGE = "Timed out waiting for the user to approve use of the '%s' tool."

## Refusing these in a headless editor would leave whoever launched it with no
## way to shut it down again.
const HEADLESS_ALWAYS_ALLOWED_TOOLS: PackedStringArray = ["close_editor", "restart_editor"]

enum Decision {
	ASK,
	ALLOW,
	DENY,
}

class Request extends RefCounted:
	var tool_name: String
	var input
	var allowed := false
	var timed_out := false

	var _done := false
	var _timeout_timer: Timer

	signal completed(p_allowed: bool)

	func _init(p_tool_name: String, p_input) -> void:
		tool_name = p_tool_name
		input = p_input

	func start_timeout(p_parent: Node, p_timeout: float) -> void:
		if _done or p_timeout <= 0.0 or _timeout_timer:
			return

		_timeout_timer = Timer.new()
		_timeout_timer.one_shot = true
		_timeout_timer.wait_time = p_timeout
		_timeout_timer.timeout.connect(time_out)
		p_parent.add_child(_timeout_timer)
		_timeout_timer.start()

	func _stop_timeout() -> void:
		if _timeout_timer:
			# Deferred, so this is safe even when called from the timeout itself.
			_timeout_timer.queue_free()
			_timeout_timer = null

	func resolve(p_allowed: bool) -> void:
		if _done:
			return
		_done = true
		allowed = p_allowed
		_stop_timeout()
		completed.emit(allowed)

	## Refuses the tool: whoever asked has stopped waiting for the answer.
	func time_out() -> void:
		if _done:
			return
		timed_out = true
		resolve(false)

	func is_done() -> bool:
		return _done

	static func resolved(p_tool_name: String, p_input, p_allowed: bool) -> Request:
		var request := Request.new(p_tool_name, p_input)
		request.resolve(p_allowed)
		return request

var _session_decisions: Dictionary[String, Decision]
var _allow_all_for_session := false


## Whether using this tool requires the user's approval at all.
static func needs_authorization(p_tool: ToolManager.Tool) -> bool:
	return p_tool != null and not p_tool.is_read_only()


static func denied_result(p_tool_name: String) -> ToolManager.ToolResult:
	return ToolManager.ToolResult.rejected({errors = [DENIED_MESSAGE % p_tool_name]})


static func timed_out_result(p_tool_name: String) -> ToolManager.ToolResult:
	return ToolManager.ToolResult.rejected({errors = [TIMED_OUT_MESSAGE % p_tool_name]})


func get_decision(p_tool_name: String) -> Decision:
	# A persisted decision wins over the session ones: the user went out of their
	# way to say "always", and "allow all for this session" shouldn't quietly
	# override a tool they've explicitly denied.
	if p_tool_name in GodaiEditorSettings.get_denied_tools():
		return Decision.DENY
	if p_tool_name in GodaiEditorSettings.get_allowed_tools():
		return Decision.ALLOW

	var session: Decision = _session_decisions.get(p_tool_name, Decision.ASK)
	if session != Decision.ASK:
		return session

	if _allow_all_for_session:
		return Decision.ALLOW

	return Decision.ASK


func set_tool_for_session(p_tool_name: String, p_allowed: bool) -> void:
	_session_decisions[p_tool_name] = Decision.ALLOW if p_allowed else Decision.DENY


static func _remove_tool(p_tools: PackedStringArray, p_tool_name: String) -> void:
	var index := p_tools.find(p_tool_name)
	if index != -1:
		p_tools.remove_at(index)


func set_tool_always(p_tool_name: String, p_allowed: bool) -> void:
	var allowed := GodaiEditorSettings.get_allowed_tools()
	var denied := GodaiEditorSettings.get_denied_tools()

	# Allow moving between allowed/denied lists.
	_remove_tool(allowed, p_tool_name)
	_remove_tool(denied, p_tool_name)
	if p_allowed:
		allowed.push_back(p_tool_name)
	else:
		denied.push_back(p_tool_name)

	GodaiEditorSettings.set_allowed_tools(allowed)
	GodaiEditorSettings.set_denied_tools(denied)


func set_allow_all_for_session(p_allowed: bool) -> void:
	_allow_all_for_session = p_allowed


## Forgets everything that was only meant to last for one session. Called when
## the MCP client disconnects or the chat is cleared.
func clear_session() -> void:
	_session_decisions.clear()
	_allow_all_for_session = false
