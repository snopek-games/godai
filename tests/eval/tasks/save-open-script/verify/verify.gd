# Run: godot --headless --path <project> --script res://verify/verify.gd
# Godot flushes open script buffers on editor exit regardless of skip-save, so
# after close the disk says 250 whether or not the agent saved; that half is
# checked by editor_verify.gd. This only smoke-checks the script survived.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var script: Resource = load("res://player.gd")
	if not (script is GDScript):
		_check("script_loads", false, "player.gd is missing or failed to parse")
		_emit()
		return
	_check("script_loads", true)

	var player: Node = Sprite2D.new()
	player.set_script(script)
	var speed = player.get("speed")
	_check("edit_survived", speed == 250.0, "speed is %s, expected 250" % speed)
	player.free()

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
