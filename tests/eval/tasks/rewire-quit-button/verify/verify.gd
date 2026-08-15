# Run: godot --headless --path <project> --script res://verify/verify.gd

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var scene: PackedScene = load("res://menu.tscn")
	if scene == null:
		_check("menu_loads", false, "menu.tscn is missing or failed to parse")
		_emit()
		return
	_check("menu_loads", true)

	var menu: Node = scene.instantiate()

	var quit: Node = menu.find_child("QuitButton", true, false)
	if quit == null:
		_check("quit_calls_quit_handler", false, "QuitButton is missing")
		_check("quit_no_longer_starts_game", false, "QuitButton is missing")
	else:
		var methods := _pressed_methods(quit)
		_check("quit_calls_quit_handler", "_on_quit_button_pressed" in methods,
			"QuitButton.pressed is connected to %s" % [methods])
		_check("quit_no_longer_starts_game", not ("_on_start_button_pressed" in methods),
			"QuitButton.pressed still calls _on_start_button_pressed")

	var start: Node = menu.find_child("StartButton", true, false)
	if start == null:
		_check("start_still_wired", false, "StartButton is missing")
	else:
		_check("start_still_wired", "_on_start_button_pressed" in _pressed_methods(start),
			"StartButton.pressed is connected to %s" % [_pressed_methods(start)])

	menu.free()
	_emit()


func _pressed_methods(button: Node) -> Array:
	var methods := []
	for connection in button.pressed.get_connections():
		methods.append(connection["callable"].get_method())
	return methods


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
