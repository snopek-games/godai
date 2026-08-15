# Run: godot --headless --path <project> --script res://verify/verify.gd
# The eval_stale fixture plugin overwrote tileset.png (red -> blue) after the
# editor imported the red original. Loading goes through the import cache, so
# the texture is only blue if something reimported after the overwrite.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var texture: Resource = load("res://sprites/tileset.png")
	if texture == null or not (texture is Texture2D):
		_check("tileset_imports", false, "tileset.png no longer loads as a texture")
		_emit()
		return
	_check("tileset_imports", true)
	_check("tileset_intact", texture.get_width() == 64 and texture.get_height() == 64,
		"texture is %dx%d, expected 64x64" % [texture.get_width(), texture.get_height()])

	var pixel: Color = texture.get_image().get_pixel(0, 0)
	_check("tileset_reimported", pixel.b > 0.9 and pixel.r < 0.1,
		"imported pixel is %s, still the pre-overwrite image" % pixel)
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
