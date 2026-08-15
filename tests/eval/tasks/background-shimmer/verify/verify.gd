# Run: godot --headless --path <project> --script res://verify/verify.gd
# Shimmer at a distance is aliasing from a texture with no mipmaps, so the fix
# has to end with mipmap generation enabled in the asset's import settings.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var import := ConfigFile.new()
	var err := import.load("res://sprites/background.png.import")
	if err != OK:
		_check("import_file_readable", false,
			"background.png.import is missing or failed to parse (error %d)" % err)
		_emit()
		return
	_check("import_file_readable", true)

	var mipmaps = import.get_value("params", "mipmaps/generate", false)
	_check("mipmaps_enabled", bool(mipmaps), "mipmaps/generate is %s" % mipmaps)

	var texture: Resource = load("res://sprites/background.png")
	_check("texture_still_imports", texture is Texture2D,
		"" if texture is Texture2D else "background.png no longer loads as a texture")

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
