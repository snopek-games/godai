extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")

const ProjectTools = preload("res://addons/godai/tools/default/project_tools.gd")
const SceneTools = preload("res://addons/godai/tools/default/scene_tools.gd")
const NodeTools = preload("res://addons/godai/tools/default/node_tools.gd")
const ResourceTools = preload("res://addons/godai/tools/default/resource_tools.gd")
const ScriptTools = preload("res://addons/godai/tools/default/script_tools.gd")
const ImportTools = preload("res://addons/godai/tools/default/import_tools.gd")
const EditorTools = preload("res://addons/godai/tools/default/editor_tools.gd")

const DEFAULT_TOOLS_JSON = "res://addons/godai/tools/default/default_tools.json"


static func load_default_tools(p_tools: ToolManager) -> void:
	var data := _load_json_data()

	# Each category file registers its own tools, pulling their descriptions
	# and schemas from the shared default_tools.json data.
	ProjectTools.register(p_tools, data)
	SceneTools.register(p_tools, data)
	NodeTools.register(p_tools, data)
	ResourceTools.register(p_tools, data)
	ScriptTools.register(p_tools, data)
	ImportTools.register(p_tools, data)
	EditorTools.register(p_tools, data)


static func _load_json_data() -> Dictionary:
	var fa := FileAccess.open(DEFAULT_TOOLS_JSON, FileAccess.READ)
	if fa:
		var data: Dictionary = JSON.parse_string(fa.get_as_text())
		var tools: Dictionary = data["tools"]
		# Store the name in the value so we don't have to repeat it.
		for name in tools.keys():
			tools[name]["name"] = name
		return tools

	return Dictionary()
