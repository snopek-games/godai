extends Camera2D

var shake_strength := 0.0


func _process(delta: float) -> void:
	offset = Vector2(randf() - 0.5, randf() - 0.5) * shake_strength
	shake_strength = maxf(shake_strength - 20.0 * delta, 0.0)
