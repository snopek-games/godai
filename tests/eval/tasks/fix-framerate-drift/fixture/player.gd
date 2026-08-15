extends Sprite2D

var speed := 4.0


func _process(_delta: float) -> void:
	position.x += speed
