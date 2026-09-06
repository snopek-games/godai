extends GutTest

const GodaiEditorSettings = preload("res://addons/godai/editor_settings.gd")


func test_system_prompt_outside_the_editor_is_the_default() -> void:
	assert_eq(GodaiEditorSettings.get_api_system_prompt(), GodaiEditorSettings.API_SYSTEM_PROMPT_DEFAULT)


func test_system_prompt_env_override() -> void:
	OS.set_environment(GodaiEditorSettings.API_SYSTEM_PROMPT_ENV, "Be helpful.")
	assert_eq(GodaiEditorSettings.get_api_system_prompt(), "Be helpful.")

	OS.set_environment(GodaiEditorSettings.API_SYSTEM_PROMPT_ENV, "")
	assert_eq(GodaiEditorSettings.get_api_system_prompt(), "", "an empty override sends no prompt")

	OS.unset_environment(GodaiEditorSettings.API_SYSTEM_PROMPT_ENV)
	assert_eq(GodaiEditorSettings.get_api_system_prompt(), GodaiEditorSettings.API_SYSTEM_PROMPT_DEFAULT)


func test_local_api_urls() -> void:
	for url in [
		"http://localhost:11434/v1/",
		"http://localhost/v1",
		"https://localhost:8080",
		"http://127.0.0.1:1234/v1/",
		"https://127.0.0.1/",
		"HTTP://LocalHost:11434/v1/",
	]:
		assert_true(GodaiEditorSettings.is_local_api_url(url), url)

	for url in [
		"https://api.anthropic.com/v1/",
		"http://localhost.example.com/v1/",
		"http://127.0.0.10/v1/",
		"http://localhostile:80/",
		"localhost:11434",
		"",
	]:
		assert_false(GodaiEditorSettings.is_local_api_url(url), url)
