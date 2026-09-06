extends GutTest

const MarkdownLabelScene = preload("res://addons/godai/ui/markdown_label.tscn")

var _markdown_label


func before_each() -> void:
	_markdown_label = MarkdownLabelScene.instantiate()
	add_child_autofree(_markdown_label)


func test_shows_markdown_as_bbcode_and_keeps_the_original() -> void:
	_markdown_label.set_markdown("**bold** [b]raw[/b]")
	assert_eq(_markdown_label.markdown, "**bold** [b]raw[/b]")
	assert_true(_markdown_label.label.bbcode_enabled)
	assert_eq(_markdown_label.label.text, "[b]bold[/b] [lb]b]raw[lb]/b]")
	assert_eq(_markdown_label.label.get_parsed_text(), "bold [b]raw[/b]")


func test_toggle_button_switches_to_raw_markdown_and_back() -> void:
	_markdown_label.set_markdown("**bold** [b]raw[/b]")

	_markdown_label.toggle_button.button_pressed = true
	assert_false(_markdown_label.label.bbcode_enabled)
	assert_eq(_markdown_label.label.text, "**bold** [b]raw[/b]")
	assert_eq(_markdown_label.label.get_parsed_text(), "**bold** [b]raw[/b]")

	_markdown_label.toggle_button.button_pressed = false
	assert_true(_markdown_label.label.bbcode_enabled)
	assert_eq(_markdown_label.label.text, "[b]bold[/b] [lb]b]raw[lb]/b]")


func test_raw_view_survives_new_markdown() -> void:
	_markdown_label.toggle_button.button_pressed = true
	_markdown_label.set_markdown("# Title")
	assert_false(_markdown_label.label.bbcode_enabled)
	assert_eq(_markdown_label.label.text, "# Title")


func test_toggle_button_is_faded_unless_hovered_or_raw() -> void:
	var button: Button = _markdown_label.toggle_button
	assert_almost_eq(button.modulate.a, _markdown_label.TOGGLE_BUTTON_IDLE_ALPHA, 0.001)
	button.mouse_entered.emit()
	assert_almost_eq(button.modulate.a, 1.0, 0.001)
	button.mouse_exited.emit()
	assert_almost_eq(button.modulate.a, _markdown_label.TOGGLE_BUTTON_IDLE_ALPHA, 0.001)

	button.button_pressed = true
	assert_almost_eq(button.modulate.a, 1.0, 0.001, "stays visible while raw is active")
	button.mouse_entered.emit()
	button.mouse_exited.emit()
	assert_almost_eq(button.modulate.a, 1.0, 0.001)
	button.button_pressed = false
	assert_almost_eq(button.modulate.a, _markdown_label.TOGGLE_BUTTON_IDLE_ALPHA, 0.001)


func test_toggle_button_says_which_view_it_switches_to() -> void:
	var button: Button = _markdown_label.toggle_button
	assert_eq(button.tooltip_text, "Show raw markdown")
	button.button_pressed = true
	assert_eq(button.tooltip_text, "Show formatted text")
	button.button_pressed = false
	assert_eq(button.tooltip_text, "Show raw markdown")


func test_toggle_button_is_invisible_until_pressed_but_keeps_its_size() -> void:
	var button: Button = _markdown_label.toggle_button
	assert_is(button.get_theme_stylebox("normal"), StyleBoxEmpty)
	assert_false(button.flat)
	var pressed_box: StyleBox = button.get_theme_stylebox("pressed")
	for side in [SIDE_LEFT, SIDE_TOP, SIDE_RIGHT, SIDE_BOTTOM]:
		assert_eq(button.get_theme_stylebox("normal").get_margin(side), pressed_box.get_margin(side))
