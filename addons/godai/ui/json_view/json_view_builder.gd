@tool
extends RefCounted

const JsonSchema = preload("res://addons/godai/ui/json_view/json_schema.gd")
const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")
const ObjectViewScene = preload("res://addons/godai/ui/json_view/json_object_view.tscn")
const SectionScene = preload("res://addons/godai/ui/json_view/json_section.tscn")
const ScalarViewScene = preload("res://addons/godai/ui/json_view/json_scalar_view.tscn")
const CodeViewScene = preload("res://addons/godai/ui/json_view/json_code_view.tscn")
const ListViewScene = preload("res://addons/godai/ui/json_view/json_list_view.tscn")
const ScalarView = preload("res://addons/godai/ui/json_view/json_scalar_view.gd")

const LONG_STRING_LENGTH := 80
const INLINE_ARRAY_MAX_ITEMS := 3
const MAX_ROWS := 50
const COLLAPSE_DEPTH := 2
const TITLE_KEYS := ["name", "path", "title", "file_path"]
const SEVERITY_KEYS := {
	errors = JsonFormat.Severity.ERROR,
	warnings = JsonFormat.Severity.WARNING,
	notes = JsonFormat.Severity.NOTE,
}
const STATUS_KEY := "success"

var expand_all := false
var empty_text := "(none)"
var null_text := "(none)"
var root_severity := JsonFormat.Severity.NONE

var _identifier_regex := RegEx.create_from_string("^[a-z][a-z0-9_]*$")


func build(p_value, p_schema: Dictionary = {}, p_depth := 0, p_key := "") -> Control:
	var schema := JsonSchema.for_value(p_schema, p_value)
	var severity: int = SEVERITY_KEYS.get(p_key, root_severity if p_depth == 0 else JsonFormat.Severity.NONE)

	if p_value == null and p_depth == 0:
		return _scalar(null_text, ScalarView.Icon.NONE, JsonFormat.Severity.NONE, true)

	if p_value is Dictionary:
		if p_value.is_empty():
			return _scalar(empty_text, ScalarView.Icon.NONE, JsonFormat.Severity.NONE, true)
		return _build_members(p_value.keys(), p_value.values(), schema, p_depth, MAX_ROWS)

	if p_value is Array:
		if p_value.is_empty():
			return _scalar(empty_text, ScalarView.Icon.NONE, JsonFormat.Severity.NONE, true)
		if p_value.all(JsonFormat.is_scalar):
			if not _is_big(p_value):
				return _scalar(", ".join(p_value.map(JsonFormat.format_scalar)), ScalarView.Icon.NONE, severity)
			var list = ListViewScene.instantiate()
			list.setup(p_value, severity, MAX_ROWS)
			return list
		return _build_members(range(p_value.size()), p_value, schema, p_depth, MAX_ROWS)

	if p_value is String or p_value is StringName:
		var text := str(p_value)
		if text == "":
			return _scalar(empty_text, ScalarView.Icon.NONE, JsonFormat.Severity.NONE, true)
		if _is_big(text, schema):
			var code = CodeViewScene.instantiate()
			code.setup(text, JsonSchema.media_type(schema))
			return code
		return _scalar(text, ScalarView.Icon.NONE, severity)

	if p_value is bool:
		var icon: ScalarView.Icon
		if p_key == STATUS_KEY:
			icon = ScalarView.Icon.SUCCESS if p_value else ScalarView.Icon.FAILURE
		else:
			icon = ScalarView.Icon.CHECKED if p_value else ScalarView.Icon.UNCHECKED
		return _scalar(JsonFormat.format_scalar(p_value), icon)

	return _scalar(JsonFormat.format_scalar(p_value), ScalarView.Icon.NONE, JsonFormat.Severity.NONE, p_value == null)


func _scalar(p_text: String, p_icon: ScalarView.Icon = ScalarView.Icon.NONE, p_severity: JsonFormat.Severity = JsonFormat.Severity.NONE, p_dim := false) -> Control:
	var view = ScalarViewScene.instantiate()
	view.setup(p_text, p_icon, p_severity, p_dim)
	return view


func _build_members(p_keys: Array, p_values: Array, p_schema: Dictionary, p_depth: int, p_limit: int) -> Control:
	var view = ObjectViewScene.instantiate()
	var shown := p_keys.size() if p_limit <= 0 else mini(p_limit, p_keys.size())

	for i in shown:
		var key = p_keys[i]
		var value = p_values[i]
		var child_schema := JsonSchema.child(p_schema, key)
		var value_schema := JsonSchema.for_value(child_schema, value)
		var key_name := str(key)
		var key_text := "[%d]" % key if key is int else _display_key(key_name)
		var tooltip := JsonSchema.description(value_schema)

		if _is_big(value, value_schema):
			var section = SectionScene.instantiate()
			var title := key_text
			if key is int and value is Dictionary:
				var item_title := _item_title(value)
				if item_title != "":
					title += "  " + item_title
			var expanded := expand_all or p_depth < COLLAPSE_DEPTH
			section.setup(title, _count_text(value), tooltip, expanded,
				func (): return build(value, child_schema, p_depth + 1, key_name))
			view.add_section(section)
		else:
			view.add_row(key_text, tooltip, build(value, child_schema, p_depth + 1, key_name))

	if shown < p_keys.size():
		view.set_hidden_count(p_keys.size() - shown)
		view.show_all_requested.connect(func ():
			var full := _build_members(p_keys, p_values, p_schema, p_depth, 0)
			full.size_flags_horizontal = view.size_flags_horizontal
			var parent := view.get_parent()
			parent.add_child(full)
			parent.move_child(full, view.get_index())
			parent.remove_child(view)
			view.queue_free()
		)

	return view


func _is_big(p_value, p_schema: Dictionary = {}) -> bool:
	if p_value is Dictionary:
		return not p_value.is_empty()
	if p_value is Array:
		if p_value.is_empty():
			return false
		return p_value.size() > INLINE_ARRAY_MAX_ITEMS or p_value.any(func (item): return not JsonFormat.is_scalar(item) or (item is String and _is_big(item)))
	if p_value is String:
		return "\n" in p_value or p_value.length() > LONG_STRING_LENGTH or JsonSchema.media_type(p_schema) != ""
	return false


func _display_key(p_key: String) -> String:
	return p_key.capitalize() if _identifier_regex.search(p_key) else p_key


func _item_title(p_dict: Dictionary) -> String:
	for key in TITLE_KEYS:
		if p_dict.get(key) is String:
			return p_dict[key]
	return ""


func _count_text(p_value) -> String:
	if p_value is Dictionary:
		return _plural(p_value.size(), "key")
	if p_value is Array:
		return _plural(p_value.size(), "item")
	if p_value is String and "\n" in p_value:
		return _plural(p_value.count("\n") + 1, "line")
	return ""


func _plural(p_count: int, p_noun: String) -> String:
	return "%d %s" % [p_count, p_noun if p_count == 1 else p_noun + "s"]
