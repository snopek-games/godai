# Run: godot --headless --path <project> --script res://verify/verify.gd

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

	_check("debug_overlay_gone", main.find_child("DebugOverlay", true, false) == null,
		"DebugOverlay is still in the scene")
	_check("overlay_children_gone", main.find_child("FPSLabel", true, false) == null,
		"FPSLabel is still in the scene")

	var player: Node = main.find_child("Player", true, false)
	if player == null or not (player is Node2D):
		_check("player_untouched", false, "Player is missing")
	else:
		_check("player_untouched", player.position.is_equal_approx(Vector2(100, 100)),
			"Player moved to %s" % player.position)

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
