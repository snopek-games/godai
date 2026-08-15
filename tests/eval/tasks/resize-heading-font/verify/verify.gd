# Run: godot --headless --path <project> --script res://verify/verify.gd

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var settings: Resource = load("res://heading.tres")
	if settings == null:
		_check("heading_loads", false, "heading.tres is missing or failed to parse")
		_emit()
		return
	_check("heading_loads", true)

	_check("is_label_settings", settings is LabelSettings, "resource is a %s" % settings.get_class())

	if settings is LabelSettings:
		_check("font_size_48", settings.font_size == 48, "font_size is %d" % settings.font_size)
	else:
		_check("font_size_48", false, "not a LabelSettings resource")

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
