extends RefCounted

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const ToolResult = ToolManager.ToolResult
const DefaultTool = ToolManager.DefaultTool
const Utils = preload("res://addons/godai/utils.gd")


static func register(p_tools: ToolManager, p_data: Dictionary) -> void:
	p_tools.register_tool(ScriptCreate.new(p_data["create_script"]))
	p_tools.register_tool(ScriptOpen.new(p_data["open_script"]))
	p_tools.register_tool(ScriptSave.new(p_data["save_script"]))
	p_tools.register_tool(ScriptRead.new(p_data["read_script"]))
	p_tools.register_tool(ScriptWrite.new(p_data["write_script"]))


class ScriptCreate extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var base_class: String = p_input.get('base_class', 'Node')
		var content: String = p_input.get('content', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' already exists" % file_path]})

		if content.is_empty():
			content = "extends %s\n" % base_class

		var dir_path = file_path.get_base_dir()
		if not DirAccess.dir_exists_absolute(dir_path):
			var dir_err = DirAccess.make_dir_recursive_absolute(dir_path)
			if dir_err != OK:
				return ToolResult.rejected({errors = ["Failed to make parent directory '%s': %s" % [dir_path, error_string(dir_err)]]})

		var fa := FileAccess.open(file_path, FileAccess.WRITE)
		if not fa:
			return ToolResult.rejected({errors = ["Failed to open '%s' for writing: %s" % [file_path, error_string(FileAccess.get_open_error())]]})
		fa.store_string(content)
		fa.close()

		# Let the editor know about the new file.
		EditorInterface.get_resource_filesystem().update_file(file_path)

		# The AI just wrote this content, so it's safe for it to write again.
		Utils.record_script_read(file_path, content)

		return ToolResult.resolved({success = true})


class ScriptOpen extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		var script = load(file_path)
		if not script is Script:
			return ToolResult.rejected({errors = ["'%s' is not a script" % file_path]})

		EditorInterface.edit_script(script)

		return ToolResult.resolved({success = true})


class ScriptRead extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		# This returns the live editor buffer when the script is open, so the
		# AI sees (and is tracked against) any unsaved changes too.
		var result := Utils.read_script_content(file_path)
		if result.has("error"):
			return ToolResult.rejected({errors = [result['error']]})

		# Remember what the AI saw, so write_script can detect later changes.
		Utils.record_script_read(file_path, result['content'])

		return ToolResult.resolved({
			content = result['content'],
			open_in_editor = result['open_in_editor'],
		})


class ScriptWrite extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')
		var content: String = p_input['content']

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist - use create_script to make a new script" % file_path]})

		# Refuse to overwrite changes the AI hasn't seen: the current content
		# (the editor buffer if open, otherwise the file) must match what was
		# last read (or written) through these tools.
		var current := Utils.read_script_content(file_path)
		if current.has("error"):
			return ToolResult.rejected({errors = [current['error']]})
		var writable := Utils.check_script_writable(file_path, current['content'])
		if writable.has("error"):
			return ToolResult.rejected({errors = [writable['error']]})

		# If the script is open in the editor, update its buffer rather than
		# writing the file out from under it (which would clobber unsaved
		# edits and cause a reload conflict). The user (or save_script) can
		# then flush it to disk.
		var editor := Utils.get_open_script_editor(file_path)
		if editor:
			editor.text = content
			Utils.record_script_read(file_path, content)
			return ToolResult.resolved({
				success = true,
				open_in_editor = true,
				saved = false,
			})

		var fa := FileAccess.open(file_path, FileAccess.WRITE)
		if not fa:
			return ToolResult.rejected({errors = ["Failed to open '%s' for writing: %s" % [file_path, error_string(FileAccess.get_open_error())]]})
		fa.store_string(content)
		fa.close()

		EditorInterface.get_resource_filesystem().update_file(file_path)
		Utils.record_script_read(file_path, content)

		return ToolResult.resolved({
			success = true,
			open_in_editor = false,
			saved = true,
		})


class ScriptSave extends DefaultTool:
	func execute(p_input) -> ToolResult:
		var file_path: String = p_input.get('file_path', '')

		file_path = Utils.to_res_path(file_path)
		if file_path.is_empty():
			return ToolResult.rejected({errors = ["'file_path' must be inside the project (res://)"]})
		if not FileAccess.file_exists(file_path):
			return ToolResult.rejected({errors = ["'%s' doesn't exist" % file_path]})

		# Only an open script can have unsaved changes to flush.
		var editor := Utils.get_open_script_editor(file_path)
		if not editor:
			return ToolResult.resolved({
				success = true,
				saved = false,
			})

		var content: String = editor.text

		var fa := FileAccess.open(file_path, FileAccess.WRITE)
		if not fa:
			return ToolResult.rejected({errors = ["Failed to open '%s' for writing: %s" % [file_path, error_string(FileAccess.get_open_error())]]})
		fa.store_string(content)
		fa.close()

		# Keep the in-memory script resource in sync, then let the editor pick
		# up the change on disk.
		var script = load(file_path)
		if script is Script:
			script.source_code = content
			script.reload()
		EditorInterface.get_resource_filesystem().update_file(file_path)

		return ToolResult.resolved({
			success = true,
			saved = true,
		})
