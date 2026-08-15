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

	var camera: Node = main.find_child("Camera2D", true, false)
	if camera == null:
		_check("camera_still_there", false, "Camera2D is missing from the scene")
		_check("script_detached", false, "Camera2D is missing from the scene")
	else:
		_check("camera_still_there", true)
		var script: Script = camera.get_script()
		_check("script_detached", script == null,
			"" if script == null else "Camera2D still has '%s'" % script.resource_path)

	_check("script_file_kept", FileAccess.file_exists("res://camera_shake_test.gd"),
		"res://camera_shake_test.gd was deleted")

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
