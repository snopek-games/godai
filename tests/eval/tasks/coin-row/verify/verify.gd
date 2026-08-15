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

	var positions: Array = []
	for node in main.get_children():
		if node.scene_file_path == "res://coin.tscn" and node is Node2D:
			positions.append(node.position)

	_check("five_coins", positions.size() == 5, "%d coin instances found" % positions.size())

	var missing := ""
	for i in 5:
		var wanted := Vector2(100 + i * 64, 200)
		var found := false
		for pos: Vector2 in positions:
			if pos.is_equal_approx(wanted):
				found = true
				break
		if not found:
			missing = "no coin at %s; coins are at %s" % [wanted, positions]
			break
	_check("coins_spaced_64px", missing == "", missing)

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
