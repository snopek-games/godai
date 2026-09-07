extends GutTest

const JsonViewBuilder = preload("res://addons/godai/ui/json_view/json_view_builder.gd")
const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")
const ScalarView = preload("res://addons/godai/ui/json_view/json_scalar_view.gd")
const CodeView = preload("res://addons/godai/ui/json_view/json_code_view.gd")
const ListView = preload("res://addons/godai/ui/json_view/json_list_view.gd")
const Section = preload("res://addons/godai/ui/json_view/json_section.gd")
const ObjectView = preload("res://addons/godai/ui/json_view/json_object_view.gd")

var _builder: JsonViewBuilder


func before_each() -> void:
	_builder = JsonViewBuilder.new()


func _build(p_value, p_schema := {}) -> Control:
	var view := _builder.build(p_value, p_schema)
	add_child_autofree(view)
	return view


func _grid_rows(p_object: Control) -> Dictionary:
	var rows := {}
	for child in p_object.get_children():
		if child is GridContainer:
			var cells := child.get_children()
			for i in range(0, cells.size(), 2):
				rows[cells[i].text] = cells[i + 1]
	return rows


func _sections(p_object: Control) -> Array:
	return p_object.get_children().filter(func (c): return c is Section)


func test_scalars_become_scalar_views() -> void:
	assert_eq(_build("hello").label.text, "hello")
	assert_eq(_build(3.0).label.text, "3")
	assert_eq(_build(2.5).label.text, "2.5")
	assert_eq(_build(123456.7).label.text, "123456.7")
	assert_eq(_build(0.000005).label.text, "0.000005")
	var member_null = _grid_rows(_build({x = null}))["X"]
	assert_eq(member_null.label.text, "null")
	assert_true(member_null.dim)

	var yes = _build(true)
	assert_eq(yes.label.text, "true")
	assert_eq(yes.icon, ScalarView.Icon.CHECKED)
	assert_eq(_build(false).icon, ScalarView.Icon.UNCHECKED)


func test_empty_values_show_the_empty_text_and_null_the_null_text() -> void:
	_builder.empty_text = "(nothing)"
	_builder.null_text = "(pending)"
	for value in [{}, [], ""]:
		var view = _build(value)
		assert_eq(view.label.text, "(nothing)")
		assert_true(view.dim)
	var null_view = _build(null)
	assert_eq(null_view.label.text, "(pending)")
	assert_true(null_view.dim)


func test_root_severity_applies_to_the_top_level_value_only() -> void:
	_builder.root_severity = JsonFormat.Severity.ERROR
	assert_eq(_build("Unknown tool: nope").severity, JsonFormat.Severity.ERROR)
	assert_eq(_build(["a", "b"]).severity, JsonFormat.Severity.ERROR)
	assert_eq(_grid_rows(_build({other = "x"}))["Other"].severity, JsonFormat.Severity.NONE)


func test_multiline_and_long_strings_become_code_views() -> void:
	var multiline = _build("a\nb")
	assert_true(multiline is CodeView)
	assert_eq(multiline.text, "a\nb")
	assert_eq(multiline.media_type, "")
	assert_eq(multiline.wrap_mode, TextEdit.LINE_WRAPPING_BOUNDARY)

	var long = _build("x".repeat(JsonViewBuilder.LONG_STRING_LENGTH + 1))
	assert_true(long is CodeView)
	assert_eq(long.text, "x".repeat(JsonViewBuilder.LONG_STRING_LENGTH + 1))
	assert_false(long.gutters_draw_line_numbers, "a single wrapped paragraph has no line numbers")
	assert_true(multiline.gutters_draw_line_numbers)

	assert_true(_build("x".repeat(JsonViewBuilder.LONG_STRING_LENGTH)) is ScalarView)


func test_code_view_uses_the_schema_media_type() -> void:
	var code = _build("extends Node\nfunc _ready():\n\tpass", {type = "string", contentMediaType = "text/x-gdscript"})
	assert_eq(code.media_type, "text/x-gdscript")
	assert_eq(code.wrap_mode, TextEdit.LINE_WRAPPING_NONE)
	assert_true(code.gutters_draw_line_numbers)
	if ClassDB.can_instantiate("GDScriptSyntaxHighlighter"):
		assert_not_null(code.syntax_highlighter)
	else:
		assert_null(code.syntax_highlighter)


func test_short_strings_with_a_media_type_still_use_the_code_view() -> void:
	var schema := {type = "string", contentMediaType = "text/x-gdscript"}
	var code = _build("print(1)", schema)
	assert_true(code is CodeView)
	assert_eq(code.media_type, "text/x-gdscript")

	var view = _build({code = "print(1)"}, {type = "object", properties = {code = schema}})
	var sections := _sections(view)
	assert_eq(sections.size(), 1)
	assert_eq(sections[0].title, "Code")
	assert_true(sections[0].get_body_content() is CodeView)


func test_objects_lay_out_small_members_in_a_grid_with_descriptions() -> void:
	var schema := {type = "object", properties = {file_path = {description = "Where"}, success = {}}}
	var view = _build({file_path = "res://a.tscn", success = true, count = 2.0}, schema)
	assert_true(view is ObjectView)

	var rows := _grid_rows(view)
	assert_eq(rows.keys(), ["File Path", "Success", "Count"])
	assert_eq(rows["File Path"].label.text, "res://a.tscn")
	assert_eq(rows["Success"].icon, ScalarView.Icon.SUCCESS)
	assert_eq(rows["Count"].label.text, "2")

	var key_labels := view.get_child(0).get_children().filter(func (c): return c is Label)
	assert_eq(key_labels[0].tooltip_text, "Where")
	assert_eq(key_labels[1].tooltip_text, "")


func test_failed_success_gets_the_failure_icon() -> void:
	assert_eq(_grid_rows(_build({success = false}))["Success"].icon, ScalarView.Icon.FAILURE)


func test_only_snake_case_keys_are_capitalized() -> void:
	var rows := _grid_rows(_build({
		"open_in_editor": 1.0,
		"Player/Sprite": 2.0,
		"MyMesh:mesh": 3.0,
		"res://a.gd": 4.0,
		"ToggleButton": 5.0,
		"2d_thing": 6.0,
		"already spaced": 7.0,
	}))
	assert_eq(rows.keys(), ["Open In Editor", "Player/Sprite", "MyMesh:mesh", "res://a.gd", "ToggleButton", "2d_thing", "already spaced"])


func test_severity_comes_from_the_key_name() -> void:
	var view = _build({errors = ["a", "b", "c", "d"], warnings = ["w"], notes = ["n1", "n2"], other = ["o"]})

	var sections := _sections(view)
	assert_eq(sections.size(), 1)
	assert_eq(sections[0].title, "Errors")
	assert_eq(sections[0].count_text, "4 items")
	var errors = sections[0].get_body_content()
	assert_true(errors is ListView)
	assert_eq(errors.severity, JsonFormat.Severity.ERROR)
	assert_eq(errors.text, "[ul]a\nb\nc\nd[/ul]")

	var rows := _grid_rows(view)
	assert_eq(rows["Warnings"].severity, JsonFormat.Severity.WARNING)
	assert_eq(rows["Notes"].label.text, "n1, n2")
	assert_eq(rows["Notes"].severity, JsonFormat.Severity.NOTE)
	assert_eq(rows["Other"].severity, JsonFormat.Severity.NONE)


func test_small_members_fill_the_grid_and_big_members_follow_as_sections() -> void:
	var view = _build({a = 1.0, content = "x\ny", b = 2.0, nested = {c = 3.0}})

	assert_eq(view.get_child(0), view.grid)
	assert_eq(_grid_rows(view).keys(), ["A", "B"])

	var sections := _sections(view)
	assert_eq(sections.size(), 2)
	assert_eq(sections[0].title, "Content")
	assert_eq(sections[0].count_text, "2 lines")
	assert_true(sections[0].get_body_content() is CodeView)
	assert_eq(sections[1].title, "Nested")
	assert_eq(sections[1].count_text, "1 key")
	assert_eq(_grid_rows(sections[1].get_body_content())["C"].label.text, "3")
	assert_eq(view.get_children().find(sections[1]) + 1, view.show_all_button.get_index(), "sections sit between the grid and the show-all button")


func test_arrays_of_objects_become_titled_sections() -> void:
	var view = _build([{name = "Player", type = "Node2D"}, {type = "Sprite2D"}, "plain"])

	var sections := _sections(view)
	assert_eq(sections.size(), 2)
	assert_eq(sections[0].title, "[0]  Player")
	assert_eq(sections[0].count_text, "2 keys")
	assert_eq(sections[1].title, "[1]")
	assert_eq(_grid_rows(view)["[2]"].label.text, "plain")


func test_sections_collapse_below_the_collapse_depth_and_build_lazily() -> void:
	var view = _build({l0 = {l1 = {l2 = {l3 = 1.0}}}})

	var l0 = _sections(view)[0]
	assert_true(l0.expanded)
	var l1 = _sections(l0.get_body_content())[0]
	assert_true(l1.expanded)
	var l2 = _sections(l1.get_body_content())[0]
	assert_false(l2.expanded)
	assert_null(l2.get_body_content(), "collapsed sections build their body on first expand")

	l2.header.button_pressed = true
	assert_true(l2.expanded)
	assert_eq(_grid_rows(l2.get_body_content())["L 3"].label.text, "1")


func test_expand_all_expands_every_section() -> void:
	_builder.expand_all = true
	var view = _build({l0 = {l1 = {l2 = {l3 = 1.0}}}})
	var l2 = _sections(_sections(_sections(view)[0].get_body_content())[0].get_body_content())[0]
	assert_true(l2.expanded)
	assert_not_null(l2.get_body_content())


func test_long_objects_are_capped_until_show_all() -> void:
	var big := {}
	for i in JsonViewBuilder.MAX_ROWS + 5:
		big["k%d" % i] = float(i)
	var parent := VBoxContainer.new()
	add_child_autofree(parent)
	var view := _builder.build(big)
	parent.add_child(view)

	assert_eq(_grid_rows(view).size(), JsonViewBuilder.MAX_ROWS)
	assert_true(view.show_all_button.visible)
	assert_eq(view.show_all_button.text, "Show all (5 more)")

	view.show_all_button.pressed.emit()
	var replacement := parent.get_child(0)
	assert_ne(replacement, view)
	assert_eq(_grid_rows(replacement).size(), JsonViewBuilder.MAX_ROWS + 5)
	assert_false(replacement.show_all_button.visible)
	await get_tree().process_frame


func test_long_lists_are_capped_until_show_all() -> void:
	var list = _build(range(JsonViewBuilder.MAX_ROWS + 1).map(func (i): return "line %d" % i))
	assert_true(list is ListView)
	assert_eq(list.shown_count(), JsonViewBuilder.MAX_ROWS)
	assert_string_contains(list.text, "[url=show_all]Show all %d items[/url]" % (JsonViewBuilder.MAX_ROWS + 1))

	list.meta_clicked.emit(ListView.SHOW_ALL_META)
	assert_eq(list.shown_count(), JsonViewBuilder.MAX_ROWS + 1)
	assert_string_ends_with(list.text, "line %d[/ul]" % JsonViewBuilder.MAX_ROWS)


func test_list_items_are_escaped_bbcode_list_entries() -> void:
	var list = _build(["[b]x[/b]", true, null, 2.0])
	assert_eq(list.text, "[ul][lb]b]x[lb]/b]\ntrue\nnull\n2[/ul]")
	assert_eq(list.get_parsed_text().dedent().strip_edges(), "[b]x[/b]\ntrue\nnull\n2")
