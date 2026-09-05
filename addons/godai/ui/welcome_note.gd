@tool
extends VBoxContainer

signal settings_requested
signal go_online_requested

@onready var fix_section: Control = %FixSection
@onready var fix_label: Label = %FixLabel
@onready var settings_button: Button = %SettingsButton
@onready var go_online_button: Button = %GoOnlineButton
@onready var get_started_label: Label = %GetStartedLabel


func _ready() -> void:
	settings_button.pressed.connect(settings_requested.emit)
	go_online_button.pressed.connect(go_online_requested.emit)


func _on_rich_text_label_meta_clicked(p_meta: Variant) -> void:
	OS.shell_open(str(p_meta))
