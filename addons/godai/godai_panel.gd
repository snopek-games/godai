@tool
extends Control

const ClaudeClient = preload("res://addons/godai/claude_client.gd")
const ToolManager = preload("res://addons/godai/tool_manager.gd")
const DefaultTools = preload("res://addons/godai/default_tools.gd")

const ANTHROPIC_API_KEY_SETTING = "godai/anthropic_api_key"
const DEBUG := true

@onready var output_label: RichTextLabel = %OutputLabel
@onready var prompt: TextEdit = %Prompt
@onready var submit_button: Button = %SubmitButton
@onready var clear_button: Button = %ClearButton

var claude_client: ClaudeClient
var current_chat: ClaudeClient.Chat
var tools: ToolManager = ToolManager.new()


func _ready() -> void:
	claude_client = ClaudeClient.new()
	add_child(claude_client)

	if Engine.is_editor_hint():
		var settings: EditorSettings = EditorInterface.get_editor_settings()
		settings.settings_changed.connect(_update_from_editor_settings.bind(settings))
		_update_from_editor_settings(settings)

	clear_button.disabled = true

	DefaultTools.register(tools)
	claude_client.tools = tools


func show_panel() -> void:
	if not prompt.has_focus():
		prompt.grab_focus()


func _update_from_editor_settings(p_settings: EditorSettings) -> void:
	claude_client.api_key = p_settings.get_setting(ANTHROPIC_API_KEY_SETTING)


func _on_current_chat_message_added(p_msg: ClaudeClient.Message) -> void:
	for content in p_msg.content:
		var data: Dictionary = content.data
		match content.get_type():
			"text":
				var text: String = data.get("text", "")
				if text:
					if p_msg.role == "user":
						output_label.append_text("[color=green]> %s[/color]\n" % text)
					elif p_msg.role == "assistant":
						output_label.append_text(text + "\n")

			"tool_use":
				if DEBUG:
					output_label.append_text(" == Using tool '%s' (%s): %s\n" % [data['id'], data['name'], data['input']])
				else:
					output_label.append_text(" == Using tool '%s'\n" % data['name'])

			"tool_result":
				if DEBUG:
					output_label.append_text(" == Tool result (%s): %s\n" % [data['tool_use_id'], data['content']])



func _start_new_chat() -> void:
	_stop_current_chat()

	# @todo We can give Claude some context here if we want
	current_chat = ClaudeClient.Chat.new()
	current_chat.message_added.connect(_on_current_chat_message_added)

	clear_button.disabled = false


func _stop_current_chat() -> void:
	if current_chat:
		current_chat.message_added.disconnect(_on_current_chat_message_added)
	current_chat = null

	clear_button.disabled = true


func _submit_message() -> void:
	var content := prompt.text
	prompt.clear()

	if current_chat == null:
		_start_new_chat()
	current_chat.add_message(ClaudeClient.Message.new("user", content))

	submit_button.disabled = true
	var resp: ClaudeClient.Response = await claude_client.submit_chat(current_chat).completed
	submit_button.disabled = false

	if resp.is_error():
		var error := resp.get_error()
		output_label.append_text("[color=red]> Error (%s): %s[/color]\n" % [error.type, error.message])
		return

	if current_chat:
		current_chat.print_debug()


func _on_prompt_gui_input(p_event: InputEvent) -> void:
	if p_event is InputEventKey:
		var key_event: InputEventKey = p_event
		if key_event.keycode == KEY_ENTER and key_event.pressed and not key_event.ctrl_pressed and not key_event.shift_pressed:
			_submit_message()
			accept_event()


func _on_submit_button_pressed() -> void:
	_submit_message()


func _on_clear_button_pressed() -> void:
	_stop_current_chat()
	output_label.clear()
	prompt.clear()
