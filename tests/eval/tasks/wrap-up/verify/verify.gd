# Run: godot --headless --path <project> --script res://verify/verify.gd
# Whether the editor was closed can't be read from the end state (the harness
# closes any editor still running before verifying); that part of the task
# shows up in the action metrics.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var project_name := str(ProjectSettings.get_setting("application/config/name", ""))
	_check("project_renamed", project_name == "Space Miner Gold",
		"project name is '%s'" % project_name)
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
