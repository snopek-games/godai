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

	var lost: Node = main.find_child("Coin3", true, false)
	if lost == null or not (lost is Node2D):
		_check("lost_coin_exists", false, "no Node2D named Coin3 in the saved scene")
	else:
		_check("lost_coin_exists", true)
		_check("lost_coin_moved_back", lost.position.is_equal_approx(Vector2(400, 300)),
			"Coin3 is at %s" % lost.position)

	var untouched := {
		"Coin1": Vector2(120, 80),
		"Coin2": Vector2(260, 140),
		"Coin4": Vector2(500, 90),
	}
	var others_ok := true
	var detail := ""
	for coin_name in untouched:
		var coin: Node = main.find_child(coin_name, true, false)
		if coin == null or not (coin is Node2D):
			others_ok = false
			detail = "%s is missing" % coin_name
		elif not coin.position.is_equal_approx(untouched[coin_name]):
			others_ok = false
			detail = "%s moved to %s" % [coin_name, coin.position]
	_check("other_coins_untouched", others_ok, detail)

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
