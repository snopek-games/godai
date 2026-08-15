# Run: godot --headless --path <project> --script res://verify/verify.gd
# Drives the movement callback with different deltas: frame-rate independent
# movement scales with delta, the buggy version moves the same amount per call.

extends SceneTree

const SPEED_PER_SECOND := 240.0

var _checks: Array = []


func _init() -> void:
	var scene: PackedScene = load("res://main.tscn")
	if scene == null:
		_check("main_scene_loads", false, "main.tscn is missing or failed to parse")
		_emit()
		return
	_check("main_scene_loads", true)

	var main: Node = scene.instantiate()
	var player: Node = main.find_child("Player", true, false)
	if player == null or not (player is Node2D) or player.get_script() == null:
		_check("player_has_script", false, "no scripted Node2D named Player in the scene")
		_emit()
		return
	_check("player_has_script", true)

	root.add_child(main)

	var full: float = _advance(player, 1.0)
	var half: float = _advance(player, 0.5)

	_check("moves_right", full > 0.0, "x moved by %f over one second" % full)
	_check("scales_with_delta", half > 0.0 and absf(full - half * 2.0) < 1.0,
		"one second moved %f but two half seconds moved %f" % [full, half * 2.0])
	_check("speed_kept_at_240", absf(full - SPEED_PER_SECOND) < 5.0,
		"x moved by %f in one second, expected about %f" % [full, SPEED_PER_SECOND])

	main.free()
	_emit()


# Which callback the movement lives in is the agent's choice, so try the idle
# frame first and fall back to the physics one.
func _advance(player: Node2D, delta: float) -> float:
	var before: float = player.position.x
	if player.has_method("_process"):
		player._process(delta)
	if is_equal_approx(player.position.x, before) and player.has_method("_physics_process"):
		player._physics_process(delta)
	return player.position.x - before


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
