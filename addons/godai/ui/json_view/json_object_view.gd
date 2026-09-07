@tool
extends VBoxContainer

signal show_all_requested

@export var key_style: StyleBox

# We can't use @onready vars because we build this before it's added to the scene tree.
var grid: GridContainer:
	get: return %GridContainer
var show_all_button: Button:
	get: return %ShowAllButton


func add_row(p_key: String, p_tooltip: String, p_value: Control) -> void:
	var key := Label.new()
	key.text = p_key
	key.tooltip_text = p_tooltip
	key.mouse_filter = Control.MOUSE_FILTER_PASS
	key.size_flags_vertical = SIZE_SHRINK_BEGIN
	key.add_theme_stylebox_override("normal", key_style)
	grid.add_child(key)

	p_value.size_flags_horizontal = SIZE_EXPAND_FILL
	grid.add_child(p_value)


func add_section(p_section: Control) -> void:
	add_child(p_section)
	move_child(p_section, show_all_button.get_index())


func set_hidden_count(p_count: int) -> void:
	show_all_button.text = "Show all (%d more)" % p_count
	show_all_button.visible = true


func _on_show_all_button_pressed() -> void:
	show_all_requested.emit()
