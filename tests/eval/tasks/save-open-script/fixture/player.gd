extends Sprite2D

var speed := 100.0


func _process(delta: float) -> void:
	position.x += speed * delta
