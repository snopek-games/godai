# Run: godot --headless --path <project> --script res://verify/verify.gd
# "Evenly spaced on a circle" is checked geometrically: every orb at radius
# 200 from the centre, and the sorted angular gaps all equal. Where around the
# circle the first orb sits is the agent's choice.

extends SceneTree

const CENTER := Vector2(400, 300)
const RADIUS := 200.0

var _checks: Array = []


func _init() -> void:
	var scene: PackedScene = load("res://main.tscn")
	if scene == null:
		_check("main_scene_loads", false, "main.tscn is missing or failed to parse")
		_emit()
		return
	_check("main_scene_loads", true)

	var main: Node = scene.instantiate()
	var orbs: Node = main.find_child("Orbs", true, false)
	if orbs == null:
		_check("twelve_orbs", false, "the Orbs node is gone")
		_emit()
		return

	var angles: Array = []
	var radius_detail := ""
	for orb in orbs.get_children():
		if not (orb is Node2D):
			continue
		var offset: Vector2 = orb.position - CENTER
		if absf(offset.length() - RADIUS) > 2.0:
			radius_detail = "%s is %.1f from the centre" % [orb.name, offset.length()]
		angles.append(offset.angle())

	_check("twelve_orbs", angles.size() == 12, "%d orbs found" % angles.size())
	_check("all_on_radius_200", radius_detail == "", radius_detail)

	if angles.size() >= 2:
		angles.sort()
		var expected_gap := TAU / angles.size()
		var gap_detail := ""
		for i in angles.size():
			var gap: float = angles[(i + 1) % angles.size()] - angles[i]
			if i == angles.size() - 1:
				gap += TAU
			if absf(gap - expected_gap) > 0.05:
				gap_detail = "angular gap of %.3f rad, expected %.3f" % [gap, expected_gap]
		_check("evenly_spaced", gap_detail == "", gap_detail)
	else:
		_check("evenly_spaced", false, "not enough orbs to space")

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
