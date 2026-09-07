@tool
extends RichTextLabel

const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")

const SHOW_ALL_META := "show_all"

var items: Array = []
var severity := JsonFormat.Severity.NONE
var limit := 0


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	_apply()


func setup(p_items: Array, p_severity := JsonFormat.Severity.NONE, p_limit := 0) -> void:
	items = p_items
	severity = p_severity
	limit = p_limit
	if is_node_ready():
		_apply()


func shown_count() -> int:
	return items.size() if limit <= 0 else mini(limit, items.size())


func _apply() -> void:
	if severity != JsonFormat.Severity.NONE:
		add_theme_color_override("default_color", JsonFormat.severity_color(severity, self))
	else:
		remove_theme_color_override("default_color")

	var lines := PackedStringArray()
	for i in shown_count():
		lines.append(JsonFormat.format_scalar(items[i]).replace("[", "[lb]"))
	text = "[ul]%s[/ul]" % "\n".join(lines)
	if shown_count() < items.size():
		text += "\n[url=%s]Show all %d items[/url]" % [SHOW_ALL_META, items.size()]


func _on_meta_clicked(p_meta: Variant) -> void:
	if str(p_meta) == SHOW_ALL_META:
		limit = 0
		_apply()
