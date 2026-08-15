# Run: godot --headless --path <project> --script res://verify/verify.gd

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var scene: PackedScene = load("res://arena.tscn")
	if scene == null:
		_check("arena_loads", false, "arena.tscn is missing or failed to parse")
		_emit()
		return
	_check("arena_loads", true)

	var arena: Node = scene.instantiate()
	root.add_child(arena)

	for enemy_name in ["Enemy1", "Enemy2", "Enemy3"]:
		var enemy: Node = arena.find_child(enemy_name, true, false)
		if enemy == null:
			_check("%s_in_enemies_group" % enemy_name.to_lower(), false, "%s is missing" % enemy_name)
		else:
			_check("%s_in_enemies_group" % enemy_name.to_lower(), enemy.is_in_group("enemies"),
				"%s is not in the 'enemies' group" % enemy_name)

	var player: Node = arena.find_child("Player", true, false)
	if player == null:
		_check("player_not_in_enemies_group", false, "Player is missing")
	else:
		_check("player_not_in_enemies_group", not player.is_in_group("enemies"),
			"Player is still in the 'enemies' group")

	arena.free()
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
