@tool
extends CodeEdit

const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")

const GDSCRIPT_MEDIA_TYPE := "text/x-gdscript"

var media_type := ""
var _text := ""


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	var font := JsonFormat.source_font(self)
	if font:
		add_theme_font_override("font", font)
	_apply()


func setup(p_text: String, p_media_type := "") -> void:
	_text = p_text
	media_type = p_media_type
	if is_node_ready():
		_apply()


func _apply() -> void:
	if media_type == GDSCRIPT_MEDIA_TYPE and ClassDB.can_instantiate("GDScriptSyntaxHighlighter"):
		syntax_highlighter = ClassDB.instantiate("GDScriptSyntaxHighlighter")
	else:
		syntax_highlighter = null
	var is_code := media_type != ""
	wrap_mode = TextEdit.LINE_WRAPPING_NONE if is_code else TextEdit.LINE_WRAPPING_BOUNDARY
	gutters_draw_line_numbers = is_code or "\n" in _text
	text = _text
