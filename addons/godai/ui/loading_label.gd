@tool
extends Control

const MAX_DOTS = 3

@onready var label: Label = %Label
@onready var timer: Timer = %Timer
@onready var sprite: AnimatedSprite2D = %AnimatedSprite2D

var base_text := "Thinking":
	set(p_value):
		base_text = p_value
		_update_text()

var dots := MAX_DOTS


func _ready() -> void:
	_update_animation()


func _notification(p_what: int) -> void:
	if p_what == NOTIFICATION_VISIBILITY_CHANGED and is_node_ready():
		_update_animation()


func _update_animation() -> void:
	if is_visible_in_tree() and not is_part_of_edited_scene():
		timer.start()
		sprite.play(&"default")
	else:
		timer.stop()
		sprite.pause()


func _on_timer_timeout() -> void:
	dots += 1
	if dots > MAX_DOTS:
		dots = 0

	_update_text()


func _update_text() -> void:
	label.text = base_text + ".".repeat(dots)
