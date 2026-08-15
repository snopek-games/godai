# Run: godot --headless --path <project> --script res://verify/verify.gd
# The editor persists its settings on shutdown, and the harness closes the
# editor before verifying, so the change must be on disk by now. The harness
# also runs this with the workspace's XDG dirs, so OS.get_config_dir() is the
# run's own isolated config, not the developer's.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var settings_dir := OS.get_config_dir().path_join("godot")
	var text := _read_editor_settings(settings_dir)
	if text == "":
		_check("editor_settings_saved", false,
			"no editor_settings-*.tres found under %s" % settings_dir)
		_emit()
		return
	_check("editor_settings_saved", true)

	var regex := RegEx.create_from_string("run/output/font_size\\s*=\\s*(\\d+)")
	var found := regex.search(text)
	if found == null:
		_check("output_font_size_20", false,
			"run/output/font_size is not in the saved editor settings")
	else:
		_check("output_font_size_20", found.get_string(1) == "20",
			"run/output/font_size is %s" % found.get_string(1))

	_emit()


func _read_editor_settings(settings_dir: String) -> String:
	var dir := DirAccess.open(settings_dir)
	if dir == null:
		return ""
	for file in dir.get_files():
		if file.begins_with("editor_settings") and file.ends_with(".tres"):
			return FileAccess.get_file_as_string(settings_dir.path_join(file))
	return ""


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
