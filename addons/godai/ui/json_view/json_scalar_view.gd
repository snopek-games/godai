@tool
extends HBoxContainer

const JsonFormat = preload("res://addons/godai/ui/json_view/json_format.gd")

enum Icon { NONE, CHECKED, UNCHECKED, SUCCESS, FAILURE }

const ICON_NAMES := {
	Icon.CHECKED: &"GuiChecked",
	Icon.UNCHECKED: &"GuiUnchecked",
	Icon.SUCCESS: &"StatusSuccess",
	Icon.FAILURE: &"StatusError",
}

@onready var icon_rect: TextureRect = %Icon
@onready var label: RichTextLabel = %Label

var text := ""
var icon := Icon.NONE
var severity := JsonFormat.Severity.NONE
var dim := false


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	_apply()


func setup(p_text: String, p_icon := Icon.NONE, p_severity := JsonFormat.Severity.NONE, p_dim := false) -> void:
	text = p_text
	icon = p_icon
	severity = p_severity
	dim = p_dim
	if is_node_ready():
		_apply()


func _apply() -> void:
	label.text = text
	var texture := JsonFormat.icon(ICON_NAMES.get(icon, &""), self)
	icon_rect.texture = texture
	icon_rect.visible = texture != null
	if severity != JsonFormat.Severity.NONE:
		label.add_theme_color_override("default_color", JsonFormat.severity_color(severity, self))
	elif dim:
		label.add_theme_color_override("default_color", JsonFormat.dim_color(self))
	else:
		label.remove_theme_color_override("default_color")
