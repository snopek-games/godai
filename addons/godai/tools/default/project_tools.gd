extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const VerifiedSettingsTool = preload("res://addons/godai/tools/default/verified_settings_tool.gd")
const Utils = preload("res://addons/godai/utils.gd")
const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")
const EditorGlobals = preload("res://addons/godai/editor_globals.gd")

const GODAI_SETTING_MESSAGE = "'%s' overrides a Godai setting; those are only accessible to the user, via Editor Settings."


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(ProjectGetCurrent.new(p_data["get_current_project"]))
	p_tools.register_tool(ProjectGetSettings.new(p_data["get_project_settings"]))
	p_tools.register_tool(ProjectSetSettings.new(p_data["set_project_settings"]))
	p_tools.register_tool(ProjectRun.new(p_data["run_project"]))
	p_tools.register_tool(ProjectStop.new(p_data["stop_project"]))


class ProjectGetCurrent extends DefaultTool:
	func execute(p_input) -> ToolResult:
		return ToolResult.resolved({
			project_path = ProjectSettings.globalize_path("res://").simplify_path(),
			project_name = ProjectSettings.get_setting("application/config/name"),
			headless = (DisplayServer.get_name() == "headless"),
			offscreen = GodaiEditorSettings.is_offscreen(),
			godot_version = _get_godot_version(),
		})

	func _get_godot_version() -> String:
		var info := Engine.get_version_info()

		var parts := PackedStringArray([str(info["major"]), str(info["minor"])])
		if int(info["patch"]) != 0:
			parts.append(str(info["patch"]))
		parts.append(info["status"])
		if ClassDB.class_exists("CSharpScript"):
			parts.append("mono")
		parts.append(info["build"])

		return ".".join(parts)


class ProjectGetSettings extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var names: Array = p_input.get('names', [])
		var include_defaults: bool = p_input.get('include_defaults', false)
		var enums_as_ints: bool = bool(p_input.get('enums_as_ints', false))

		for name in names:
			if GodaiEditorSettings.is_godai_project_setting(name):
				return ToolResult.rejected({errors = [GODAI_SETTING_MESSAGE % name]})

		var result := Utils.get_settings_map(ProjectSettings, names, include_defaults, enums_as_ints)
		if result.has('error'):
			return ToolResult.rejected({errors = [result['error']]})

		# Only has anything to do when 'names' was empty, since named Godai
		# overrides are rejected above.
		var settings: Dictionary = result['settings']
		for name in settings.keys():
			if GodaiEditorSettings.is_godai_project_setting(name):
				settings.erase(name)

		var resolved := { settings = settings }
		var translated := PackedStringArray()
		for name in result['translated']:
			if settings.has(name):
				translated.append(name)
		if not translated.is_empty():
			resolved['notes'] = [Utils.enum_translation_note(translated)]
		return ToolResult.resolved(resolved)


class ProjectSetSettings extends VerifiedSettingsTool:
	func get_properties_tool_name() -> String:
		return "get_project_settings"

	func get_settings_object() -> Object:
		return ProjectSettings

	func check_setting_allowed(p_name: String) -> String:
		if GodaiEditorSettings.is_godai_project_setting(p_name):
			return GODAI_SETTING_MESSAGE % p_name
		return ""

	func allows_feature_overrides() -> bool:
		return true

	func save_settings() -> String:
		var err := ProjectSettings.save()
		if err != OK:
			return "Failed to save project settings: %s" % error_string(err)
		return ""


class ProjectRun extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var scene: String = p_input.get('scene', '')

		var play: Callable
		if scene.is_empty() or scene == "main":
			var main_scene: String = ProjectSettings.get_setting("application/run/main_scene", "")
			if main_scene.is_empty():
				return ToolResult.rejected({errors = ["No main scene is configured; pass 'scene' set to 'current' or a scene path"]})
			play = EditorInterface.play_main_scene
		elif scene == "current":
			if not EditorInterface.get_edited_scene_root():
				return ToolResult.rejected({errors = ["No scene open"]})
			play = EditorInterface.play_current_scene
		else:
			var scene_path := Utils.to_res_path(scene)
			if scene_path.is_empty():
				return ToolResult.rejected({errors = ["'scene' must be inside the project (res://)"]})
			if not FileAccess.file_exists(scene_path):
				return ToolResult.rejected({errors = ["'%s' doesn't exist" % scene_path]})
			play = EditorInterface.play_custom_scene.bind(scene_path)

		if bool(p_input.get('clear_log_messages', false)):
			EditorGlobals.logger.clear()

		play.call()

		return ToolResult.resolved({success = true})


class ProjectStop extends DefaultTool:
	func execute(p_input) -> ToolResult:
		EditorInterface.stop_playing_scene()
		return ToolResult.resolved({success = true})
