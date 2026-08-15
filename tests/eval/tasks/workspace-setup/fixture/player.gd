extends Sprite2D

var speed := 220.0
var jump_velocity := -400.0


func _process(delta: float) -> void:
	position.x += speed * delta
