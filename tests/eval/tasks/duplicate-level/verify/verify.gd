# Run: godot --headless --path <project> --script res://verify/verify.gd

extends SceneTree

const CHILDREN := {
	"Ground": Vector2(0, 500),
	"Spawn": Vector2(50, 100),
	"Goal": Vector2(800, 100),
}

var _checks: Array = []


func _init() -> void:
	var one: PackedScene = load("res://level_1.tscn")
	if one == null:
		_check("level_1_loads", false, "level_1.tscn is missing or failed to parse")
	else:
		var root: Node = one.instantiate()
		_check("level_1_loads", true)
		_check("level_1_root_untouched", root.name == "Level1", "root is named %s" % root.name)
		_check("level_1_children_untouched", _children_match(root), _children_detail(root))
		root.free()

	if not ResourceLoader.exists("res://level_2.tscn"):
		_check("level_2_exists", false, "res://level_2.tscn is missing")
		_emit()
		return
	var two: PackedScene = load("res://level_2.tscn")
	if two == null:
		_check("level_2_exists", false, "level_2.tscn failed to parse")
		_emit()
		return
	_check("level_2_exists", true)

	var root2: Node = two.instantiate()
	_check("level_2_root_renamed", root2.name == "Level2", "root is named %s" % root2.name)
	_check("level_2_children_match", _children_match(root2), _children_detail(root2))
	root2.free()

	_emit()


func _children_match(root: Node) -> bool:
	return _children_detail(root) == ""


func _children_detail(root: Node) -> String:
	for child_name in CHILDREN:
		var child: Node = root.find_child(child_name, true, false)
		if child == null or not (child is Node2D):
			return "%s is missing" % child_name
		if not child.position.is_equal_approx(CHILDREN[child_name]):
			return "%s is at %s" % [child_name, child.position]
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
