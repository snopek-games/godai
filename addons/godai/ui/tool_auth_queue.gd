extends RefCounted

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolAuth = preload("res://addons/godai/tools/tool_auth.gd")
const ToolUseAuthDialog = preload("res://addons/godai/ui/tool_use_auth_dialog.gd")

var _tools: ToolManager
var _tool_auth: ToolAuth
var _dialog: ToolUseAuthDialog
var _is_busy: Callable
var _busy_changed: Signal

var _pending_requests: Array[ToolAuth.Request]
var _external_requests: Array[ToolAuth.Request]
var _shown_request: ToolAuth.Request
var _updating_queue := false
var _unattended := GodaiEditorSettings.is_unattended()


func _init(p_tools: ToolManager, p_tool_auth: ToolAuth, p_dialog: ToolUseAuthDialog, p_is_busy: Callable, p_busy_changed: Signal) -> void:
	_tools = p_tools
	_tool_auth = p_tool_auth
	_dialog = p_dialog
	_is_busy = p_is_busy
	_busy_changed = p_busy_changed

	_dialog.tool_use_allowed.connect(_on_tool_use_allowed)
	_dialog.tool_use_denied.connect(_on_tool_use_denied)


func authorize(p_name: String, p_input) -> ToolAuth.Request:
	var tool_obj := _tools.get_tool(p_name)
	if not ToolAuth.needs_authorization(tool_obj):
		return ToolAuth.Request.resolved(p_name, p_input, true)

	if _unattended and p_name in ToolAuth.UNATTENDED_ALWAYS_ALLOWED_TOOLS:
		return ToolAuth.Request.resolved(p_name, p_input, true)

	match _tool_auth.get_decision(p_name):
		ToolAuth.Decision.ALLOW:
			return ToolAuth.Request.resolved(p_name, p_input, true)
		ToolAuth.Decision.DENY:
			return ToolAuth.Request.resolved(p_name, p_input, false)

	if GodaiEditorSettings.get_auto_approve_tools():
		return ToolAuth.Request.resolved(p_name, p_input, true)

	if _unattended:
		push_warning("Denying use of the '%s' tool: running unattended, and %s is not set."
			% [p_name, GodaiEditorSettings.AUTO_APPROVE_TOOLS_ENV])
		return ToolAuth.Request.resolved(p_name, p_input, false)

	var request := ToolAuth.Request.new(p_name, p_input)
	request.completed.connect(_on_request_completed)
	_pending_requests.push_back(request)
	_update_queue()
	return request


func authorize_when_idle(p_name: String, p_input) -> ToolAuth.Request:
	if not _is_busy.call():
		var request := authorize(p_name, p_input)
		_track_external(request)
		return request

	# If there's a request in progress, then we need to wait until its done.
	var gated := ToolAuth.Request.new(p_name, p_input)
	_track_external(gated)
	_authorize_when_idle(gated)
	return gated


func _track_external(p_request: ToolAuth.Request) -> void:
	if p_request.is_done():
		return
	_external_requests.push_back(p_request)
	p_request.completed.connect(func (_allowed): _external_requests.erase(p_request), CONNECT_ONE_SHOT)


func _authorize_when_idle(p_gated: ToolAuth.Request) -> void:
	while _is_busy.call():
		await _busy_changed
		# If the MCP server resolved this due to timeout, then bail.
		if p_gated.is_done():
			return

	# Now, do the usual authorization.
	var inner := authorize(p_gated.tool_name, p_gated.input)
	if inner.is_done():
		p_gated.resolve(inner.allowed)
		return

	inner.completed.connect(p_gated.resolve)
	# If the outer request times out, then we need to resolve the inner request.
	p_gated.completed.connect(func (_allowed): inner.resolve(false), CONNECT_ONE_SHOT)


func cancel_external() -> void:
	var external := _external_requests.duplicate()
	_external_requests.clear()
	for request in external:
		request.resolve(false)


func cancel_pending() -> void:
	var pending := _pending_requests.duplicate()
	_pending_requests.clear()
	_shown_request = null
	_dialog.hide()

	for request in pending:
		request.resolve(false)


func _on_request_completed(_p_allowed: bool) -> void:
	_update_queue()


func _update_queue() -> void:
	# Resolving re-enters here, so this has to be the only loop walking the queue.
	if _updating_queue:
		return
	_updating_queue = true

	while _pending_requests.size() > 0:
		var request: ToolAuth.Request = _pending_requests[0]

		# The MCP server resolves these on its own when they time out.
		if request.is_done():
			_pending_requests.pop_front()
			continue

		# Check tool_auth again, in case we recorded an allow/deny for this tool
		# earlier in the queue.
		var decision := _tool_auth.get_decision(request.tool_name)
		if decision == ToolAuth.Decision.ASK:
			break

		_pending_requests.pop_front()
		request.resolve(decision == ToolAuth.Decision.ALLOW)

	if _pending_requests.size() > 0:
		var request: ToolAuth.Request = _pending_requests[0]
		if _shown_request != request:
			_shown_request = request
			var tool_obj := _tools.get_tool(request.tool_name)
			_dialog.setup_tool_use_auth_dialog(request.tool_name, request.input, tool_obj.input_schema if tool_obj else {})
			_dialog.popup_centered()
	else:
		_shown_request = null
		_dialog.hide()

	_updating_queue = false


func _resolve_current_request(p_type: ToolUseAuthDialog.AllowDenyType, p_allowed: bool) -> void:
	var request := _shown_request
	if request == null:
		return

	match p_type:
		ToolUseAuthDialog.AllowDenyType.TOOL_FOR_SESSION:
			_tool_auth.set_tool_for_session(request.tool_name, p_allowed)
		ToolUseAuthDialog.AllowDenyType.TOOL_ALWAYS:
			_tool_auth.set_tool_always(request.tool_name, p_allowed)
		ToolUseAuthDialog.AllowDenyType.ALL_FOR_SESSION:
			# Only makes sense for allow - denying all for this session isn't a thing.
			if p_allowed:
				_tool_auth.set_allow_all_for_session(true)

	request.resolve(p_allowed)


func _on_tool_use_allowed(p_type: ToolUseAuthDialog.AllowDenyType) -> void:
	_resolve_current_request(p_type, true)


func _on_tool_use_denied(p_type: ToolUseAuthDialog.AllowDenyType) -> void:
	_resolve_current_request(p_type, false)
