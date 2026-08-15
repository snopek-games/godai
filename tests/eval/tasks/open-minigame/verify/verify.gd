# Run: godot --headless --path <project> --script res://verify/verify.gd
# The minigame is its own project, so its settings are read straight from its
# project.godot rather than through this project's ProjectSettings.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var config := ConfigFile.new()
	var err := config.load("res://minigame/project.godot")
	if err != OK:
		_check("minigame_config_readable", false,
			"minigame/project.godot is missing or failed to parse (error %d)" % err)
		_emit()
		return
	_check("minigame_config_readable", true)

	var mini_name = config.get_value("application", "config/name", "")
	_check("minigame_renamed", str(mini_name) == "Bonus Round",
		"minigame's project name is '%s'" % mini_name)

	var own_name := str(ProjectSettings.get_setting("application/config/name", ""))
	_check("main_project_untouched", own_name == "Eval: open-minigame",
		"this project was renamed to '%s'" % own_name)

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
