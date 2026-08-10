extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(ImportReimport.new(p_data["reimport"]))
	p_tools.register_tool(ImportGetSettings.new(p_data["get_import_settings"]))
	p_tools.register_tool(ImportSetSettings.new(p_data["set_import_settings"]))


class ImportReimport extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_paths: Array = p_input.get('file_paths', [])

		var resolved_paths := PackedStringArray()
		var errors := PackedStringArray()
		for fp in file_paths:
			var path := Utils.to_res_path(fp)
			if path.is_empty():
				errors.append("'%s' must be inside the project (res://)" % fp)
				continue
			if not FileAccess.file_exists(path):
				errors.append("'%s' doesn't exist" % path)
				continue
			if not FileAccess.file_exists(path + ".import"):
				errors.append("'%s' is not an imported asset" % path)
				continue
			resolved_paths.append(path)

		if not errors.is_empty():
			return ToolResult.rejected({error = "Nothing was reimported, due to the following errors:\n" + "\n".join(errors)})

		EditorInterface.get_resource_filesystem().reimport_files(resolved_paths)

		return ToolResult.resolved({success = true})


class ImportGetSettings extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({error = "'file_path' must be inside the project (res://)"})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({error = "'%s' doesn't exist" % file_path})

		var import_path := file_path + ".import"
		if not FileAccess.file_exists(import_path):
			return ToolResult.rejected({error = "'%s' has no import settings (it isn't an imported asset)" % file_path})

		var cfg := ConfigFile.new()
		var err := cfg.load(import_path)
		if err != OK:
			return ToolResult.rejected({error = "Failed to read import settings: %s" % error_string(err)})

		var importer := ""
		if cfg.has_section_key("remap", "importer"):
			importer = cfg.get_value("remap", "importer")

		var options := {}
		if cfg.has_section("params"):
			for key in cfg.get_section_keys("params"):
				options[key] = Utils.encode_property_value(cfg.get_value("params", key))

		return ToolResult.resolved({
			importer = importer,
			options = options,
		})


class ImportSetSettings extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var importer: String = p_input.get('importer', '')
		var options: Dictionary = p_input.get('options', {})

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({error = "'file_path' must be inside the project (res://)"})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({error = "'%s' doesn't exist" % file_path})

		var import_path := file_path + ".import"
		if not FileAccess.file_exists(import_path):
			return ToolResult.rejected({error = "'%s' has no import settings (it isn't an imported asset)" % file_path})

		var cfg := ConfigFile.new()
		var err := cfg.load(import_path)
		if err != OK:
			return ToolResult.rejected({error = "Failed to read import settings: %s" % error_string(err)})

		if not importer.is_empty():
			cfg.set_value("remap", "importer", importer)

		for key in options:
			# The expected type isn't known here, so fall back to the raw
			# string when the value isn't valid variant syntax.
			var decoded := Utils.decode_property_value(options[key], TYPE_NIL)
			cfg.set_value("params", key, decoded['value'])

		err = cfg.save(import_path)
		if err != OK:
			return ToolResult.rejected({error = "Failed to write import settings: %s" % error_string(err)})

		EditorInterface.get_resource_filesystem().reimport_files(PackedStringArray([file_path]))

		return ToolResult.resolved({success = true})
