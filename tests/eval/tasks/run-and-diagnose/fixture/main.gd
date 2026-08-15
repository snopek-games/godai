extends Node2D

@onready var score_label: Label = $UI/ScoreLabel


func _ready() -> void:
	score_label.text = "Score: 0"
