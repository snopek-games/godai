@tool
extends PanelContainer

signal button_pressed

@export var text: String:
	set(p_text):
		text = p_text
		if is_node_ready():
			label.text = p_text

@export var button_text: String:
	set(p_text):
		button_text = p_text
		if is_node_ready():
			button.text = p_text

@onready var label: Label = %Label
@onready var button: Button = %Button


func _ready() -> void:
	label.text = text
	button.text = button_text
	button.pressed.connect(button_pressed.emit)
