# Run: godot --headless --path <project> --script res://verify/verify.gd

extends SceneTree

var _checks: Array = []


func _init() -> void:
	var scene: PackedScene = load("res://title.tscn") if ResourceLoader.exists("res://title.tscn") else null
	if scene == null:
		_check("title_scene_exists", false, "res://title.tscn is missing or failed to parse")
		_emit()
		return
	_check("title_scene_exists", true)

	var title: Node = scene.instantiate()
	_check("root_is_control", title is Control, "root is a %s" % title.get_class())

	var label := _find_label(title)
	_check("label_says_space_miner", label != null,
		"no Label with text 'Space Miner' in the scene")

	var main_scene: String = str(ProjectSettings.get_setting("application/run/main_scene", ""))
	var points_at_title := main_scene == "res://title.tscn"
	if not points_at_title and main_scene != "":
		var resolved: Resource = load(main_scene)
		points_at_title = resolved != null and resolved.resource_path == "res://title.tscn"
	_check("title_is_main_scene", points_at_title, "main scene is '%s'" % main_scene)

	title.free()
	_emit()


func _find_label(node: Node) -> Label:
	if node is Label and node.text == "Space Miner":
		return node
	for child in node.get_children():
		var found := _find_label(child)
		if found != null:
			return found
	return null


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
