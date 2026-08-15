# Run: godot --headless --path <project> --script res://verify/verify.gd
# The eval_select fixture plugin selected Crate1 and Crate2 when the editor
# opened, so those two must be gone and everything else untouched.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var scene: PackedScene = load("res://main.tscn")
	if scene == null:
		_check("main_scene_loads", false, "main.tscn is missing or failed to parse")
		_emit()
		return
	_check("main_scene_loads", true)

	var main: Node = scene.instantiate()

	_check("crate1_deleted", main.find_child("Crate1", true, false) == null,
		"Crate1 is still in the scene")
	_check("crate2_deleted", main.find_child("Crate2", true, false) == null,
		"Crate2 is still in the scene")

	var kept := ""
	for keep_name in ["Keep1", "Keep2"]:
		if main.find_child(keep_name, true, false) == null:
			kept = "%s was deleted but wasn't selected" % keep_name
	_check("unselected_nodes_kept", kept == "", kept)

	main.free()
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
