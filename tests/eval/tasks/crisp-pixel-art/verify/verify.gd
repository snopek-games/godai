# Run: godot --headless --path <project> --script res://verify/verify.gd
# Nearest filtering can be set project-wide or overridden on the sprite; either
# solves the blurriness, so both pass.

extends SceneTree

const FILTER_DEFAULT_NEAREST := 0
const FILTER_DEFAULT_NEAREST_MIPMAPS := 3

var _checks: Array = []


func _init() -> void:
	var setting := int(ProjectSettings.get_setting(
		"rendering/textures/canvas_textures/default_texture_filter", 1))
	var project_nearest := setting in [FILTER_DEFAULT_NEAREST, FILTER_DEFAULT_NEAREST_MIPMAPS]

	var node_nearest := false
	var player: Node = null
	var scene: PackedScene = load("res://main.tscn")
	if scene != null:
		var main: Node = scene.instantiate()
		player = main.find_child("Player", true, false)
		if player is Sprite2D:
			node_nearest = player.texture_filter in [
				CanvasItem.TEXTURE_FILTER_NEAREST,
				CanvasItem.TEXTURE_FILTER_NEAREST_WITH_MIPMAPS,
			]
			_check("texture_still_assigned", player.texture != null,
				"the Player sprite lost its texture")
		else:
			_check("texture_still_assigned", false, "no Sprite2D named Player in main.tscn")
		main.free()
	else:
		_check("texture_still_assigned", false, "main.tscn is missing or failed to parse")

	_check("renders_crisp", project_nearest or node_nearest,
		"default_texture_filter is %d and the sprite doesn't override it" % setting)

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
