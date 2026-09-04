extends GutTest

const LoadingLabelScene = preload("res://addons/godai/ui/loading_label.tscn")

# Comfortably longer than the label's animation interval.
const TICK := 0.6

var _label


func before_each() -> void:
	_label = LoadingLabelScene.instantiate()
	add_child_autofree(_label)


func test_base_text_updates_the_text() -> void:
	_label.base_text = "Cancelling"
	assert_string_starts_with(_label.text, "Cancelling")


func test_animates_only_while_visible() -> void:
	var before: String = _label.text
	await wait_seconds(TICK)
	assert_ne(_label.text, before, "the dots advance while visible")

	_label.visible = false
	before = _label.text
	await wait_seconds(TICK)
	assert_eq(_label.text, before, "hidden labels stop animating")
