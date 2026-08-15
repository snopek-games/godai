# Run: godot --headless --path <project> --script res://verify/verify.gd

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var width = ProjectSettings.get_setting("display/window/size/viewport_width", 0)
	var height = ProjectSettings.get_setting("display/window/size/viewport_height", 0)
	_check("window_width", int(width) == 1280, "viewport_width is %s" % width)
	_check("window_height", int(height) == 720, "viewport_height is %s" % height)
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
