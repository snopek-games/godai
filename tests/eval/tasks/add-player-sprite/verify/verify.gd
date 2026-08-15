# Run: godot --headless --path <project> --script res://verify/verify.gd
# Drives behaviour rather than reading source, so a different-but-correct
# implementation still passes.

extends SceneTree

const SPEED := 200.0

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
	if player == null:
		_check("player_exists", false, "no node named Player in the saved scene")
		_emit()
		return
	_check("player_exists", true)

	_check("player_is_sprite2d", player is Sprite2D, "Player is a %s" % player.get_class())

	if player is Node2D:
		_check("player_position", player.position.is_equal_approx(Vector2(100, 50)),
			"position is %s" % player.position)
	else:
		_check("player_position", false, "Player is not a Node2D")

	var script: Script = player.get_script()
	_check("player_has_script", script != null,
		"" if script != null else "no script attached to Player")
	if script != null:
		_check("script_path", script.resource_path == "res://player.gd",
			"script is at '%s'" % script.resource_path)
	else:
		_check("script_path", false, "no script attached to Player")

	if player is Node2D and script != null:
		root.add_child(main)
		var moved: float = _advance(player)
		_check("player_moves_right", moved > 0.0, "x moved by %f in one second" % moved)
		_check("player_speed", is_equal_approx(moved, SPEED),
			"x moved by %f in one second, expected %f" % [moved, SPEED])
		main.free()
	else:
		_check("player_moves_right", false, "nothing runnable to drive")
		_check("player_speed", false, "nothing runnable to drive")

	_emit()


# Drives one second of gameplay. Which callback the movement lives in is the
# agent's choice, so try the idle frame first and fall back to the physics one.
func _advance(player: Node2D) -> float:
	var before: float = player.position.x
	if player.has_method("_process"):
		player._process(1.0)
	if is_equal_approx(player.position.x, before) and player.has_method("_physics_process"):
		player._physics_process(1.0)
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
