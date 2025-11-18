@tool
extends Label

const TEXT = "Thinking"
const MAX_DOTS = 3

var dots := 3

func _on_timer_timeout() -> void:
	dots += 1
	if dots > MAX_DOTS:
		dots = 0

	var s := ""
	for i in range(dots):
		s += "."

	text = TEXT + s
