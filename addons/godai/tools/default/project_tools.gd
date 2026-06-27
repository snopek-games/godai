extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")


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
		})


class ProjectGetSettings extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var names: Array = p_input.get('names', [])
		var include_defaults: bool = p_input.get('include_defaults', false)

		var result := Utils.get_settings_map(ProjectSettings, names, include_defaults)
		if result.has('error'):
			return ToolResult.rejected({error = result['error']})

		return ToolResult.resolved({ settings = result['settings'] })


class ProjectSetSettings extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var settings: Dictionary = p_input.get('settings', {})
		if settings.is_empty():
			return ToolResult.rejected({error = "'settings' is required"})

		var result := Utils.decode_settings(ProjectSettings, settings)
		if result.has('error'):
			return ToolResult.rejected({error = result['error']})

		var values: Dictionary = result['values']
		for name in values:
			ProjectSettings.set_setting(name, values[name])

		var err := ProjectSettings.save()
		if err != OK:
			return ToolResult.rejected({error = "Failed to save project settings: %s" % error_string(err)})

		return ToolResult.resolved({success = true})


class ProjectRun extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var scene: String = p_input.get('scene', '')

		if scene.is_empty() or scene == "main":
			var main_scene: String = ProjectSettings.get_setting("application/run/main_scene", "")
			if main_scene.is_empty():
				return ToolResult.rejected({error = "No main scene is configured; pass 'scene' set to 'current' or a scene path"})
			EditorInterface.play_main_scene()
		elif scene == "current":
			if not EditorInterface.get_edited_scene_root():
				return ToolResult.rejected({error = "No scene open"})
			EditorInterface.play_current_scene()
		else:
			var scene_path := Utils.to_res_path(scene)
			if not FileAccess.file_exists(scene_path):
				return ToolResult.rejected({error = "'%s' doesn't exist" % scene_path})
			EditorInterface.play_custom_scene(scene_path)

		return ToolResult.resolved({success = true})


class ProjectStop extends DefaultTool:
	func execute(p_input) -> ToolResult:
		EditorInterface.stop_playing_scene()
		return ToolResult.resolved({success = true})
