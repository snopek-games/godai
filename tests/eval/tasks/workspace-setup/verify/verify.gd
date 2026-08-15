# Run: godot --headless --path <project> --script res://verify/verify.gd
# What's open in the editor doesn't survive the editor closing, so this only
# smoke-checks that nothing was broken; whether the right things were opened
# is checked by editor_verify.gd while the editor is still running.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var script: Resource = load("res://player.gd")
	_check("player_script_intact", script is GDScript,
		"" if script is GDScript else "player.gd is missing or failed to parse")

	var stats: Resource = load("res://player_stats.tres")
	_check("player_stats_intact", stats != null, "player_stats.tres is missing or failed to parse")

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
