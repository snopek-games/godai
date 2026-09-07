extends GutTest

const JsonViewScene = preload("res://addons/godai/ui/json_view/json_view.tscn")
const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")
const ScalarView = preload("res://addons/godai/ui/json_view/json_scalar_view.gd")
const CodeView = preload("res://addons/godai/ui/json_view/json_code_view.gd")
const ObjectView = preload("res://addons/godai/ui/json_view/json_object_view.gd")
const Section = preload("res://addons/godai/ui/json_view/json_section.gd")

var _view


func before_each() -> void:
	_view = JsonViewScene.instantiate()
	add_child_autofree(_view)


func _first_section(p_object: Control) -> Control:
	for child in p_object.get_children():
		if child is Section:
			return child
	return null


func test_starts_out_showing_the_empty_text() -> void:
	assert_true(_view.get_content() is ScalarView)
	assert_eq(_view.get_content().label.text, "(none)")
	assert_eq(_view.raw_text.text, "")


func test_set_value_builds_the_formatted_view_and_the_raw_json() -> void:
	_view.set_value({file_path = "res://a.gd"})
	assert_true(_view.get_content() is ObjectView)
	assert_eq(_view.raw_text.text, JSON.stringify({file_path = "res://a.gd"}, "    "))
	assert_true(_view.panel.visible)
	assert_false(_view.raw_text.visible)


func test_json_strings_are_parsed_and_other_strings_shown_as_text() -> void:
	_view.set_value('{"success": true}')
	assert_eq(_view.value, {success = true})
	assert_true(_view.get_content() is ObjectView)

	_view.set_value("The user denied the tool use.")
	assert_true(_view.get_content() is ScalarView)
	assert_eq(_view.get_content().label.text, "The user denied the tool use.")
	assert_eq(_view.raw_text.text, "The user denied the tool use.")

	_view.set_value("line 1\nline 2")
	assert_true(_view.get_content() is CodeView)


func test_toggle_button_switches_to_raw_json_and_back() -> void:
	_view.set_value({a = 1.0})

	_view.toggle_button.button_pressed = true
	assert_false(_view.panel.visible)
	assert_true(_view.raw_text.visible)

	_view.toggle_button.button_pressed = false
	assert_true(_view.panel.visible)
	assert_false(_view.raw_text.visible)


func test_raw_view_survives_a_new_value() -> void:
	_view.toggle_button.button_pressed = true
	_view.set_value({b = 2.0})
	assert_true(_view.raw_text.visible)
	assert_eq(_view.content_container.get_child_count(), 1, "old content is removed on rebuild")


func test_null_text_and_expand_all_are_passed_to_the_builder() -> void:
	_view.null_text = "(no output yet)"
	_view.expand_all = true
	_view.set_value({outer = {inner = {deep = {x = 1.0}}}})
	var outer = _first_section(_view.get_content())
	var inner = _first_section(outer.get_body_content())
	var deep = _first_section(inner.get_body_content())
	assert_true(deep.expanded)

	_view.set_value(null)
	assert_eq(_view.get_content().label.text, "(no output yet)")
	_view.set_value("")
	assert_eq(_view.get_content().label.text, "(none)")


func test_severity_colors_the_whole_value() -> void:
	_view.set_value("Unknown tool: nope", {}, JsonFormat.Severity.ERROR)
	assert_eq(_view.get_content().severity, JsonFormat.Severity.ERROR)

	_view.set_value("Unknown tool: nope")
	assert_eq(_view.get_content().severity, JsonFormat.Severity.NONE)
