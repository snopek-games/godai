# Run: godot --headless --path <project> --script res://verify/verify.gd
# "Deep blue" and "pale orange" are judged loosely by channel dominance, so any
# reasonable pair of colors passes; the ends may come in either order.

extends SceneTree

var _checks: Array = []


func _init() -> void:
	if not ResourceLoader.exists("res://sky_gradient.tres"):
		_check("gradient_exists", false, "res://sky_gradient.tres is missing")
		_emit()
		return
	var gradient: Resource = load("res://sky_gradient.tres")
	if gradient == null:
		_check("gradient_exists", false, "sky_gradient.tres failed to parse")
		_emit()
		return
	_check("gradient_exists", true)

	if not (gradient is Gradient):
		_check("is_gradient", false, "resource is a %s" % gradient.get_class())
		_emit()
		return
	_check("is_gradient", true)

	var count: int = gradient.get_point_count()
	_check("has_two_ends", count >= 2, "gradient has %d points" % count)
	if count < 2:
		_emit()
		return

	var first: Color = gradient.get_color(0)
	var last: Color = gradient.get_color(count - 1)
	var ends_ok := (_bluish(first) and _orangish(last)) or (_bluish(last) and _orangish(first))
	_check("blue_to_orange", ends_ok, "ends are %s and %s" % [first, last])

	_emit()


# A deep blue is dark, so the blue channel only needs to dominate, not be large.
func _bluish(c: Color) -> bool:
	return c.b > c.r and c.b > c.g and c.b > 0.15


func _orangish(c: Color) -> bool:
	return c.r > c.b and c.r > 0.5 and c.g > c.b


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
