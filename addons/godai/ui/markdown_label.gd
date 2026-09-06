@tool
extends HBoxContainer

const MarkdownToBBCode = preload("res://addons/godai/ui/markdown_to_bbcode.gd")

const TOGGLE_BUTTON_IDLE_ALPHA := 0.4

@onready var label: RichTextLabel = %Label
@onready var toggle_button: Button = %ToggleButton

var markdown := ""
var _bbcode := ""
var _toggle_button_hovered := false


func _ready() -> void:
	var invisible := StyleBoxEmpty.new()
	var boxed := toggle_button.get_theme_stylebox("pressed")
	for side in [SIDE_LEFT, SIDE_TOP, SIDE_RIGHT, SIDE_BOTTOM]:
		invisible.set_content_margin(side, boxed.get_margin(side))
	toggle_button.add_theme_stylebox_override("normal", invisible)
	toggle_button.add_theme_stylebox_override("focus", invisible)
	_update_toggle_button()
	if Engine.is_editor_hint() and not is_part_of_edited_scene():
		var editor_theme := EditorInterface.get_editor_theme()
		label.add_theme_font_override("mono_font", editor_theme.get_font(&"source", &"EditorFonts"))
		toggle_button.icon = editor_theme.get_icon(&"RichTextLabel", &"EditorIcons")
		toggle_button.text = ""


func set_markdown(p_markdown: String) -> void:
	markdown = p_markdown
	var converter := MarkdownToBBCode.new()
	converter.configure_from(label)
	_bbcode = converter.convert(p_markdown)
	_show_current_view()


func _show_current_view() -> void:
	var raw := toggle_button.button_pressed
	label.bbcode_enabled = not raw
	label.text = markdown if raw else _bbcode


func _update_toggle_button() -> void:
	var raw := toggle_button.button_pressed
	toggle_button.tooltip_text = "Show formatted text" if raw else "Show raw markdown"
	toggle_button.modulate.a = 1.0 if raw or _toggle_button_hovered else TOGGLE_BUTTON_IDLE_ALPHA


func _on_toggle_button_toggled(_pressed: bool) -> void:
	_show_current_view()
	_update_toggle_button()


func _on_toggle_button_mouse_entered() -> void:
	_toggle_button_hovered = true
	_update_toggle_button()


func _on_toggle_button_mouse_exited() -> void:
	_toggle_button_hovered = false
	_update_toggle_button()


func _on_label_meta_clicked(p_meta: Variant) -> void:
	OS.shell_open(str(p_meta))
