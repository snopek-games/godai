extends GutTest

const ToolManager = preload("res://addons/godai/tools/tool_manager.gd")
const DefaultTools = preload("res://addons/godai/tools/default_tools.gd")

var tool_manager: ToolManager


func before_all() -> void:
	tool_manager = ToolManager.new()
	DefaultTools.register(tool_manager)


func test_execute_editor_script_process_user_code() -> void:
	var test_tool: ToolManager.Tool = tool_manager.get_tool("execute_editor_script")

	var TESTS = [
		{
			name = "Simple",
			input = [
				"",
				"# Get the edited scene root",
				"var scene_root = EditorInterface.get_edited_scene_root()",
				"if scene_root == null:",
				"    print(\"No scene is currently open\")",
				"else:",
				"    # Create a new Label node",
				"    var label = Label.new()",
				"    label.name = \"PILabel\"",
				"    label.text = \"PI: 3.1415926535\"",
				"    ",
				"    # Get the undo/redo manager",
				"    var undo_redo = EditorInterface.get_editor_undo_redo()",
				"    ",
				"    # Use undo/redo to add the node",
				"    undo_redo.create_action(\"Add PI Label (AI)\")",
				"    editor_undo_redo_create_node(undo_redo, scene_root, label)",
				"    undo_redo.commit_action()",
				"    ",
				"    print(\"Successfully added Label showing PI to 10 digits: \", label.text)",
				"",
			],
			output = [
				"\t",
				"\t# Get the edited scene root",
				"\tvar scene_root = EditorInterface.get_edited_scene_root()",
				"\tif scene_root == null:",
				"\t\tprint(\"No scene is currently open\")",
				"\telse:",
				"\t\t# Create a new Label node",
				"\t\tvar label = Label.new()",
				"\t\tlabel.name = \"PILabel\"",
				"\t\tlabel.text = \"PI: 3.1415926535\"",
				"\t\t",
				"\t\t# Get the undo/redo manager",
				"\t\tvar undo_redo = EditorInterface.get_editor_undo_redo()",
				"\t\t",
				"\t\t# Use undo/redo to add the node",
				"\t\tundo_redo.create_action(\"Add PI Label (AI)\")",
				"\t\teditor_undo_redo_create_node(undo_redo, scene_root, label)",
				"\t\tundo_redo.commit_action()",
				"\t\t",
				"\t\tprint(\"Successfully added Label showing PI to 10 digits: \", label.text)",
				"\t",
			],
		},
	]

	for test in TESTS:
		var processed = test_tool._process_user_code("\n".join(test["input"]))
		assert_eq(processed, "\n".join(test["output"]))


func test_execute_editor_script() -> void:
	var test_tool: ToolManager.Tool = tool_manager.get_tool("execute_editor_script")

	var TESTS = [
		{
			name = "Simple",
			input = {
				code = 'print("hi")'
			},
			output = {
				"success": true,
				"output": PackedStringArray(["hi"]),
			}
		},
	]

	for test in TESTS:
		var output: Dictionary

		var result: ToolManager.ToolResult = test_tool.execute(test['input'])
		if result.is_done():
			output = result.content
		else:
			output = await result.completed

		assert_eq_deep(output, test['output'])
