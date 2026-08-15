# Run: godot --headless --path <project> --script res://verify/verify.gd
# Whether the fix renamed the node or repointed the script doesn't matter:
# entering the tree must leave the label assigned and showing the score.
# @onready and _ready only run when the main loop starts, so the checks happen
# on the first frame rather than in _init.

extends SceneTree

var _checks: Array = []
var _main: Node


func _init() -> void:
	var scene: PackedScene = load("res://main.tscn")
	if scene == null:
		_check("main_scene_loads", false, "main.tscn is missing or failed to parse")
		_emit()
		return
	_check("main_scene_loads", true)

	_main = scene.instantiate()
	root.add_child(_main)
	process_frame.connect(_after_ready)


func _after_ready() -> void:
	var label = _main.get("score_label")
	if label == null or not (label is Label):
		_check("label_found_at_startup", false,
			"the script's score_label is still null after _ready")
		_check("score_text_set", false, "the script's score_label is still null after _ready")
	else:
		_check("label_found_at_startup", true)
		_check("score_text_set", label.text == "Score: 0", "label text is '%s'" % label.text)

	_emit()


func _check(name: String, ok: bool, detail: String = "") -> void:
	_checks.append({"name": name, "ok": ok, "detail": detail})


func _emit() -> void:
	var passed: int = 0
	for c in _checks:
		if c["ok"]:
			passed += 1
	var result := {
		"passed": passed == _checks.size(),
		"checks_passed": passed,
		"checks_total": _checks.size(),
		"checks": _checks,
	}
	# Sentinel prefix so the scorer can find this amid Godot's own output.
	print("GODAI_VERIFY_JSON:" + JSON.stringify(result))
	quit(0 if passed == _checks.size() else 1)
