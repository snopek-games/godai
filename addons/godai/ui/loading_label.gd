@tool
extends Label

const MAX_DOTS = 3

var base_text := "Thinking":
	set(p_value):
		base_text = p_value
		_update_text()

var dots := MAX_DOTS


func _ready() -> void:
	visibility_changed.connect(_update_timer)
	_update_timer()


func _update_timer() -> void:
	if is_visible_in_tree() and not is_part_of_edited_scene():
		$Timer.start()
	else:
		$Timer.stop()


func _on_timer_timeout() -> void:
	dots += 1
	if dots > MAX_DOTS:
		dots = 0

	_update_text()


func _update_text() -> void:
	text = base_text + ".".repeat(dots)
