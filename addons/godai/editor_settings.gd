extends RefCounted

const ChatClient = preload("res://addons/godai/chat/client.gd")
const Profiles = preload("res://addons/godai/chat/profiles.gd")

const SETTING_PREFIX = "godai/"
const EDITOR_OVERRIDES_PREFIX = "editor_overrides/"

const API_PROVIDER_SETTING = "godai/api/provider"
const API_URL_SETTING = "godai/api/url"
const API_KEY_SETTING = "godai/api/key"
const API_MODEL_SETTING = "godai/api/model"
const API_EFFORT_SETTING = "godai/api/effort"
const API_THINKING_SETTING = "godai/api/thinking"
const API_BUDGET_TOKENS_SETTING = "godai/api/budget_tokens"
const API_SYSTEM_PROMPT_SETTING = "godai/api/system_prompt"

## Keys of the "api" Dictionary the settings dialog edits, and the settings they map to.
const API_SETTINGS := {
	provider = API_PROVIDER_SETTING,
	url = API_URL_SETTING,
	key = API_KEY_SETTING,
	model = API_MODEL_SETTING,
	effort = API_EFFORT_SETTING,
	thinking = API_THINKING_SETTING,
	budget_tokens = API_BUDGET_TOKENS_SETTING,
	system_prompt = API_SYSTEM_PROMPT_SETTING,
}

const LEGACY_API_SETTINGS := {
	"godai/api/anthropic_key": API_KEY_SETTING,
	"godai/api/anthropic_model": API_MODEL_SETTING,
}

const MCP_TRANSPORT_SETTING = "godai/mcp/transport"
const MCP_BASE_PORT_SETTING = "godai/mcp/base_port"
const MCP_PORT_COUNT_SETTING = "godai/mcp/port_count"
const MCP_SKIP_SECRET_CHECK_SETTING = "godai/mcp/skip_secret_check"

const AUTO_APPROVE_TOOLS_SETTING = "godai/tools/auto_approve"
const ALLOWED_TOOLS_SETTING = "godai/tools/allowed"
const DENIED_TOOLS_SETTING = "godai/tools/denied"

const NETWORK_MODE_SETTING = "network/connection/network_mode"
const NETWORK_MODE_ONLINE = 1

const LOCAL_API_URL_PREFIXES = ["http://localhost", "https://localhost", "http://127.0.0.1", "https://127.0.0.1"]

const API_PROVIDER_DEFAULT = Profiles.PROFILES[Profiles.DEFAULT].provider
const API_URL_DEFAULT = Profiles.PROFILES[Profiles.DEFAULT].url
const API_MODEL_DEFAULT = Profiles.PROFILES[Profiles.DEFAULT].model
const API_EFFORT_DEFAULT = ""
const API_THINKING_DEFAULT = true
const API_BUDGET_TOKENS_DEFAULT = 0
const API_SYSTEM_PROMPT_DEFAULT = "You are Godai, an AI assistant running inside the Godot editor. Use the available tools to inspect and change the open project on the user's behalf. Be concise."
const MCP_TRANSPORT_DEFAULT = 0
const MCP_BASE_PORT_DEFAULT = 12120
const MCP_PORT_COUNT_DEFAULT = 10
const MCP_SKIP_SECRET_CHECK_DEFAULT = false
const AUTO_APPROVE_TOOLS_DEFAULT = false

# Environment variables that override the editor settings (used for testing).
const API_PROVIDER_ENV = "GODAI_API_PROVIDER"
const API_URL_ENV = "GODAI_API_URL"
const API_KEY_ENV = "GODAI_API_KEY"
const API_MODEL_ENV = "GODAI_API_MODEL"
const API_EFFORT_ENV = "GODAI_API_EFFORT"
const API_THINKING_ENV = "GODAI_API_THINKING"
const API_BUDGET_TOKENS_ENV = "GODAI_API_BUDGET_TOKENS"
const API_SYSTEM_PROMPT_ENV = "GODAI_API_SYSTEM_PROMPT"
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


static func _migrate_legacy_api_settings() -> void:
	var settings: EditorSettings = EditorInterface.get_editor_settings()
	for old_name in LEGACY_API_SETTINGS:
		if not settings.has_setting(old_name):
			continue
		var new_name: String = LEGACY_API_SETTINGS[old_name]
		var value = settings.get_setting(old_name)
		var new_is_empty: bool = not settings.has_setting(new_name) or str(settings.get_setting(new_name)).is_empty()
		if value is String and not value.is_empty() and new_is_empty:
			settings.set_setting(new_name, value)
		settings.erase(old_name)


static func add_editor_settings() -> void:
	_migrate_legacy_api_settings()

	_add_editor_setting(API_PROVIDER_SETTING, TYPE_STRING, API_PROVIDER_DEFAULT, PROPERTY_HINT_ENUM, ",".join(ChatClient.PROVIDERS.keys()))
	_add_editor_setting(API_URL_SETTING, TYPE_STRING, API_URL_DEFAULT)
	_add_editor_setting(API_KEY_SETTING, TYPE_STRING, "", PROPERTY_HINT_PASSWORD)
	_add_editor_setting(API_MODEL_SETTING, TYPE_STRING, API_MODEL_DEFAULT)
	_add_editor_setting(API_EFFORT_SETTING, TYPE_STRING, API_EFFORT_DEFAULT)
	_add_editor_setting(API_THINKING_SETTING, TYPE_BOOL, API_THINKING_DEFAULT)
	_add_editor_setting(API_BUDGET_TOKENS_SETTING, TYPE_INT, API_BUDGET_TOKENS_DEFAULT)
	_add_editor_setting(API_SYSTEM_PROMPT_SETTING, TYPE_STRING, API_SYSTEM_PROMPT_DEFAULT, PROPERTY_HINT_MULTILINE_TEXT)

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


static func get_api_provider() -> String:
	return _get_string_env_or_setting(API_PROVIDER_ENV, API_PROVIDER_SETTING)


static func get_api_url() -> String:
	return _get_string_env_or_setting(API_URL_ENV, API_URL_SETTING)


static func get_api_key() -> String:
	return _get_string_env_or_setting(API_KEY_ENV, API_KEY_SETTING)


static func get_api_model() -> String:
	return _get_string_env_or_setting(API_MODEL_ENV, API_MODEL_SETTING)


static func get_api_effort() -> String:
	return _get_string_env_or_setting(API_EFFORT_ENV, API_EFFORT_SETTING)


static func get_api_thinking() -> bool:
	if OS.has_environment(API_THINKING_ENV):
		return not OS.get_environment(API_THINKING_ENV).to_lower() in ["0", "false", "no", "off"]
	return EditorInterface.get_editor_settings().get_setting(API_THINKING_SETTING)


static func get_api_budget_tokens() -> int:
	return _get_int_env_or_setting(API_BUDGET_TOKENS_ENV, API_BUDGET_TOKENS_SETTING)


static func get_api_system_prompt() -> String:
	if OS.has_environment(API_SYSTEM_PROMPT_ENV):
		return OS.get_environment(API_SYSTEM_PROMPT_ENV)
	var settings := _editor_settings()
	if not settings:
		return API_SYSTEM_PROMPT_DEFAULT
	return settings.get_setting(API_SYSTEM_PROMPT_SETTING)


static func get_dialog_settings() -> Dictionary:
	var settings := _editor_settings()
	if not settings:
		return {}
	var api := {}
	for key in API_SETTINGS:
		api[key] = settings.get_setting(API_SETTINGS[key])
	return {
		api = api,
		mcp = {
			base_port = int(settings.get_setting(MCP_BASE_PORT_SETTING)),
			port_count = int(settings.get_setting(MCP_PORT_COUNT_SETTING)),
		},
		tools = {
			auto_approve = bool(settings.get_setting(AUTO_APPROVE_TOOLS_SETTING)),
			allowed = get_allowed_tools(),
			denied = get_denied_tools(),
		},
	}


static func set_dialog_settings(p_values: Dictionary) -> void:
	var settings := _editor_settings()
	if not settings:
		return
	var api: Dictionary = p_values.get("api", {})
	for key in API_SETTINGS:
		if api.has(key):
			_set_setting_if_changed(settings, API_SETTINGS[key], api[key])
	var mcp: Dictionary = p_values.get("mcp", {})
	if mcp.has("base_port"):
		_set_setting_if_changed(settings, MCP_BASE_PORT_SETTING, int(mcp["base_port"]))
	if mcp.has("port_count"):
		_set_setting_if_changed(settings, MCP_PORT_COUNT_SETTING, int(mcp["port_count"]))
	var tools: Dictionary = p_values.get("tools", {})
	if tools.has("auto_approve"):
		_set_setting_if_changed(settings, AUTO_APPROVE_TOOLS_SETTING, tools["auto_approve"])
	if tools.has("allowed"):
		_set_setting_if_changed(settings, ALLOWED_TOOLS_SETTING, "\n".join(tools["allowed"]))
	if tools.has("denied"):
		_set_setting_if_changed(settings, DENIED_TOOLS_SETTING, "\n".join(tools["denied"]))


static func _set_setting_if_changed(p_settings: EditorSettings, p_name: String, p_value) -> void:
	if p_settings.get_setting(p_name) != p_value:
		p_settings.set_setting(p_name, p_value)


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


static func is_api_configured() -> bool:
	return not get_api_url().is_empty() and not get_api_key().is_empty()


static func is_local_api_url(p_url: String) -> bool:
	var url := p_url.to_lower()
	for prefix in LOCAL_API_URL_PREFIXES:
		if url.begins_with(prefix):
			var rest := url.substr(prefix.length())
			if rest.is_empty() or rest[0] == ":" or rest[0] == "/":
				return true
	return false


static func api_needs_online_mode() -> bool:
	return not is_local_api_url(get_api_url())


static func can_api_connect() -> bool:
	return is_network_online() or not api_needs_online_mode()


static func set_network_online() -> void:
	var settings := _editor_settings()
	if settings:
		settings.set_setting(NETWORK_MODE_SETTING, NETWORK_MODE_ONLINE)


static func is_network_online() -> bool:
	var settings := _editor_settings()
	if not settings or not settings.has_setting(NETWORK_MODE_SETTING):
		return true
	return int(settings.get_setting(NETWORK_MODE_SETTING)) == NETWORK_MODE_ONLINE


# Outside the editor (e.g. GUT in script mode), EditorInterface has no settings to return.
static func _editor_settings() -> EditorSettings:
	return EditorInterface.get_editor_settings() if Engine.is_editor_hint() else null


## When true, tools that would otherwise need the user's approval run without
## asking. Needed for headless editors, where nobody can answer the dialog.
static func get_auto_approve_tools() -> bool:
	if OS.has_environment(AUTO_APPROVE_TOOLS_ENV):
		var value := OS.get_environment(AUTO_APPROVE_TOOLS_ENV)
		if not value.is_empty() and value != "0":
			return true
	var settings := _editor_settings()
	if not settings:
		return AUTO_APPROVE_TOOLS_DEFAULT
	return settings.get_setting(AUTO_APPROVE_TOOLS_SETTING)


static func _get_tool_list(p_setting: String) -> PackedStringArray:
	var tools := PackedStringArray()
	var settings := _editor_settings()
	if not settings:
		return tools
	var value: String = settings.get_setting(p_setting)
	for line in value.split("\n", false):
		var tool_name := line.strip_edges()
		if not tool_name.is_empty():
			tools.push_back(tool_name)
	return tools


static func _set_tool_list(p_setting: String, p_tools: PackedStringArray) -> void:
	var settings := _editor_settings()
	if settings:
		settings.set_setting(p_setting, "\n".join(p_tools))


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
