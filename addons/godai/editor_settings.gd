extends RefCounted

const SETTING_PREFIX = "godai/"
const EDITOR_OVERRIDES_PREFIX = "editor_overrides/"

const ANTHROPIC_API_KEY_SETTING = "godai/api/anthropic_key"
const ANTHROPIC_API_MODEL_SETTING = "godai/api/anthropic_model"

const MCP_TRANSPORT_SETTING = "godai/mcp/transport"
const MCP_BASE_PORT_SETTING = "godai/mcp/base_port"
const MCP_PORT_COUNT_SETTING = "godai/mcp/port_count"
const MCP_SKIP_SECRET_CHECK_SETTING = "godai/mcp/skip_secret_check"

const AUTO_APPROVE_TOOLS_SETTING = "godai/tools/auto_approve"
const ALLOWED_TOOLS_SETTING = "godai/tools/allowed"
const DENIED_TOOLS_SETTING = "godai/tools/denied"

const ANTHROPIC_API_MODEL_DEFAULT = "claude-sonnet-5"
const MCP_TRANSPORT_DEFAULT = 0
const MCP_BASE_PORT_DEFAULT = 12120
const MCP_PORT_COUNT_DEFAULT = 10
const MCP_SKIP_SECRET_CHECK_DEFAULT = false
const AUTO_APPROVE_TOOLS_DEFAULT = false

# Environment variables that override the editor settings (used for testing).
const ANTHROPIC_API_KEY_ENV = "GODAI_ANTHROPIC_API_KEY"
const ANTHROPIC_API_MODEL_ENV = "GODAI_ANTHROPIC_MODEL"
const MCP_TRANSPORT_ENV = "GODAI_MCP_TRANSPORT"
const MCP_BASE_PORT_ENV = "GODAI_MCP_BASE_PORT"
const MCP_PORT_COUNT_ENV = "GODAI_MCP_PORT_COUNT"
const MCP_SKIP_SECRET_CHECK_ENV = "GODAI_MCP_SKIP_SECRET_CHECK"
const AUTO_APPROVE_TOOLS_ENV = "GODAI_AUTO_APPROVE_TOOLS"


static func is_godai_setting(p_name) -> bool:
	return str(p_name).begins_with(SETTING_PREFIX)


static func is_godai_project_setting(p_name) -> bool:
	var name := str(p_name)
	if not name.begins_with(EDITOR_OVERRIDES_PREFIX):
		return false
	return is_godai_setting(name.substr(EDITOR_OVERRIDES_PREFIX.length()))


static func _add_editor_setting(p_name: String, p_type: int, p_default, p_hint = null, p_hint_string = null) -> void:
	var settings: EditorSettings = EditorInterface.get_editor_settings()

	if not settings.has_setting(p_name):
		settings.set_setting(p_name, p_default)

	settings.set_initial_value(p_name, p_default, false)

	var info := {
		name = p_name,
		type = p_type,
	}
	if p_hint != null:
		info['hint'] = p_hint
	if p_hint_string != null:
		info['hint_string'] = p_hint_string

	settings.add_property_info(info)


static func add_editor_settings() -> void:
	_add_editor_setting(ANTHROPIC_API_KEY_SETTING, TYPE_STRING, "", PROPERTY_HINT_PASSWORD)
	_add_editor_setting(ANTHROPIC_API_MODEL_SETTING, TYPE_STRING, ANTHROPIC_API_MODEL_DEFAULT)

	_add_editor_setting(MCP_TRANSPORT_SETTING, TYPE_INT, MCP_TRANSPORT_DEFAULT, PROPERTY_HINT_ENUM, "WebSocket,HTTP")
	_add_editor_setting(MCP_BASE_PORT_SETTING, TYPE_INT, MCP_BASE_PORT_DEFAULT)
	_add_editor_setting(MCP_PORT_COUNT_SETTING, TYPE_INT, MCP_PORT_COUNT_DEFAULT)
	_add_editor_setting(MCP_SKIP_SECRET_CHECK_SETTING, TYPE_BOOL, MCP_SKIP_SECRET_CHECK_DEFAULT)

	_add_editor_setting(AUTO_APPROVE_TOOLS_SETTING, TYPE_BOOL, AUTO_APPROVE_TOOLS_DEFAULT)
	_add_editor_setting(ALLOWED_TOOLS_SETTING, TYPE_STRING, "", PROPERTY_HINT_MULTILINE_TEXT)
	_add_editor_setting(DENIED_TOOLS_SETTING, TYPE_STRING, "", PROPERTY_HINT_MULTILINE_TEXT)


static func _get_string_env_or_setting(p_env: String, p_setting: String) -> String:
	var value := OS.get_environment(p_env)
	if not value.is_empty():
		return value
	return EditorInterface.get_editor_settings().get_setting(p_setting)


static func get_anthropic_api_key() -> String:
	return _get_string_env_or_setting(ANTHROPIC_API_KEY_ENV, ANTHROPIC_API_KEY_SETTING)


static func get_anthropic_model() -> String:
	return _get_string_env_or_setting(ANTHROPIC_API_MODEL_ENV, ANTHROPIC_API_MODEL_SETTING)


static func _get_int_env_or_setting(p_env: String, p_setting: String) -> int:
	if OS.has_environment(p_env):
		var value := OS.get_environment(p_env)
		if value.is_valid_int():
			return value.to_int()
		push_warning("Ignoring environment variable %s: '%s' is not a valid integer" % [p_env, value])
	return EditorInterface.get_editor_settings().get_setting(p_setting)


# Accepts "websocket" or "http" (case-insensitive), or the integer enum value.
static func get_mcp_transport() -> int:
	if OS.has_environment(MCP_TRANSPORT_ENV):
		var value := OS.get_environment(MCP_TRANSPORT_ENV)
		match value.to_lower():
			"websocket", "ws":
				return 0
			"http":
				return 1
		if value.is_valid_int():
			return value.to_int()
		push_warning("Ignoring environment variable %s: '%s' is not a valid transport" % [MCP_TRANSPORT_ENV, value])
	return EditorInterface.get_editor_settings().get_setting(MCP_TRANSPORT_SETTING)


static func get_mcp_base_port() -> int:
	return _get_int_env_or_setting(MCP_BASE_PORT_ENV, MCP_BASE_PORT_SETTING)


static func get_mcp_port_count() -> int:
	return _get_int_env_or_setting(MCP_PORT_COUNT_ENV, MCP_PORT_COUNT_SETTING)


static func get_mcp_skip_secret_check() -> bool:
	if OS.has_environment(MCP_SKIP_SECRET_CHECK_ENV):
		var value := OS.get_environment(MCP_SKIP_SECRET_CHECK_ENV)
		if not value.is_empty() and value != "0":
			return true
	return EditorInterface.get_editor_settings().get_setting(MCP_SKIP_SECRET_CHECK_SETTING)


## When true, tools that would otherwise need the user's approval run without
## asking. Needed for headless editors, where nobody can answer the dialog.
static func get_auto_approve_tools() -> bool:
	if OS.has_environment(AUTO_APPROVE_TOOLS_ENV):
		var value := OS.get_environment(AUTO_APPROVE_TOOLS_ENV)
		if not value.is_empty() and value != "0":
			return true
	return EditorInterface.get_editor_settings().get_setting(AUTO_APPROVE_TOOLS_SETTING)


static func _get_tool_list(p_setting: String) -> PackedStringArray:
	var tools := PackedStringArray()
	var value: String = EditorInterface.get_editor_settings().get_setting(p_setting)
	for line in value.split("\n", false):
		var tool_name := line.strip_edges()
		if not tool_name.is_empty():
			tools.push_back(tool_name)
	return tools


static func _set_tool_list(p_setting: String, p_tools: PackedStringArray) -> void:
	EditorInterface.get_editor_settings().set_setting(p_setting, "\n".join(p_tools))


## Tools the user has approved for every session, one name per line.
static func get_allowed_tools() -> PackedStringArray:
	return _get_tool_list(ALLOWED_TOOLS_SETTING)


static func set_allowed_tools(p_tools: PackedStringArray) -> void:
	_set_tool_list(ALLOWED_TOOLS_SETTING, p_tools)


## Tools the user has rejected for every session, one name per line.
static func get_denied_tools() -> PackedStringArray:
	return _get_tool_list(DENIED_TOOLS_SETTING)


static func set_denied_tools(p_tools: PackedStringArray) -> void:
	_set_tool_list(DENIED_TOOLS_SETTING, p_tools)
