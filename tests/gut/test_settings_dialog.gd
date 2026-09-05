extends GutTest

const SettingsDialogScene = preload("res://addons/godai/ui/settings_dialog.tscn")
const SettingsDialog = preload("res://addons/godai/ui/settings_dialog.gd")
const Profiles = preload("res://addons/godai/chat/profiles.gd")
const ModelCatalog = preload("res://addons/godai/chat/model_catalog.gd")
const Fixture = preload("res://tests/gut/fixtures/models_dev_fixture.gd")

const ANTHROPIC := Profiles.PROFILES.anthropic
const OPENAI := Profiles.PROFILES.openai

var _no_tools := {auto_approve = false, allowed = PackedStringArray(), denied = PackedStringArray()}
var _default_mcp := {base_port = 12120, port_count = 10}
var _default_reasoning := {effort = "", thinking = true, budget_tokens = 0}
var _catalog: ModelCatalog


func _api_values() -> Dictionary:
	return _dialog.get_values()["api"]


func _tool_values() -> Dictionary:
	return _dialog.get_values()["tools"]


func _mcp_values() -> Dictionary:
	return _dialog.get_values()["mcp"]

var _dialog


func before_each() -> void:
	_dialog = SettingsDialogScene.instantiate()
	add_child_autofree(_dialog)
	_catalog = ModelCatalog.new(Profiles.models_dev_ids())
	add_child_autofree(_catalog)
	_catalog.load_from_dict(Fixture.API)
	_dialog.catalog = _catalog


func _select_profile(p_id: String) -> void:
	for i in _dialog.profile_select.item_count:
		if _dialog.profile_select.get_item_metadata(i) == p_id:
			_dialog.profile_select.select(i)
			_dialog.profile_select.item_selected.emit(i)
			return
	fail_test("no profile item '%s'" % p_id)


func _select_metadata(p_select: OptionButton) -> Array:
	var items := []
	for i in p_select.item_count:
		items.push_back(p_select.get_item_metadata(i))
	return items


func _select_model(p_id: String) -> void:
	for i in _dialog.model_select.item_count:
		if _dialog.model_select.get_item_metadata(i) == p_id:
			_dialog.model_select.select(i)
			_dialog.model_select.item_selected.emit(i)
			return
	fail_test("no model item '%s'" % p_id)


func _select_provider(p_name: String) -> void:
	for i in _dialog.provider_select.item_count:
		if _dialog.provider_select.get_item_metadata(i) == p_name:
			_dialog.provider_select.select(i)
			_dialog.provider_select.item_selected.emit(i)
			return
	fail_test("no provider item '%s'" % p_name)


func _custom_fields_visible() -> bool:
	return _dialog.provider_select.visible and _dialog.url_field.visible \
		and _dialog.provider_label.visible and _dialog.url_label.visible


func test_profiles_find() -> void:
	assert_eq(Profiles.find(ANTHROPIC.provider, ANTHROPIC.url), "anthropic")
	assert_eq(Profiles.find(OPENAI.provider, OPENAI.url.trim_suffix("/")), "openai", "a trailing slash doesn't matter")
	assert_eq(Profiles.find("openai_chat_completions", "http://localhost:11434/v1/"), Profiles.CUSTOM)
	assert_false(Profiles.matches("nope", ANTHROPIC.provider, ANTHROPIC.url))


func test_lists_every_profile_plus_custom() -> void:
	var ids := []
	for i in _dialog.profile_select.item_count:
		ids.push_back(_dialog.profile_select.get_item_metadata(i))

	assert_eq(ids, Array(Profiles.PROFILES.keys()).map(func (k): return String(k)) + [Profiles.CUSTOM])
	assert_true(ids[0] is String, "metadata is stored as plain Strings, like the settings")
	assert_eq(_dialog.profile_select.get_item_text(_dialog.profile_select.item_count - 1), "Custom")


func test_setup_with_a_profile_hides_the_custom_fields() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, key = "sk-1", model = "claude-x"}})

	assert_eq(_dialog.get_profile(), "anthropic")
	assert_false(_custom_fields_visible())
	assert_eq(_dialog.key_field.text, "sk-1")
	assert_eq(_api_values().model, "claude-x")
	assert_eq(_dialog.get_values(), {api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, key = "sk-1", model = "claude-x"}.merged(_default_reasoning), mcp = _default_mcp, tools = _no_tools})


func test_setup_with_custom_shows_the_fields() -> void:
	_dialog.setup({api = {provider = "openai_chat_completions", url = "http://localhost:11434/v1/", key = "", model = "llama3"}})

	assert_eq(_dialog.get_profile(), Profiles.CUSTOM)
	assert_true(_custom_fields_visible())
	assert_eq(_dialog.provider_select.get_selected_metadata(), "openai_chat_completions")
	assert_eq(_dialog.url_field.text, "http://localhost:11434/v1/")
	assert_eq(_dialog.get_values(), {api = {provider = "openai_chat_completions", url = "http://localhost:11434/v1/", key = "", model = "llama3"}.merged(_default_reasoning), mcp = _default_mcp, tools = _no_tools})


func test_setup_shows_custom_for_an_unrecognized_url() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = "http://proxy.local/v1/", key = "", model = "claude-x"}})

	assert_eq(_dialog.get_profile(), Profiles.CUSTOM)
	assert_true(_custom_fields_visible())
	assert_eq(_dialog.url_field.text, "http://proxy.local/v1/")


func test_setup_recognizes_a_profile_by_provider_and_url() -> void:
	_dialog.setup({api = {provider = OPENAI.provider, url = OPENAI.url, key = "", model = "gpt-x"}})

	assert_eq(_dialog.get_profile(), "openai")


func test_setup_without_values() -> void:
	_dialog.setup({})

	assert_eq(_dialog.get_profile(), Profiles.CUSTOM)
	assert_eq(_api_values().key, "")


func test_switching_profile_updates_provider_url_and_default_model() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, key = "sk-1", model = ANTHROPIC.model}})

	_select_profile("openai")

	assert_eq(_api_values().model, OPENAI.model, "the untouched default follows the profile")
	assert_eq(_dialog.get_values(), {api = {provider = OPENAI.provider, url = OPENAI.url, key = "sk-1", model = OPENAI.model}.merged(_default_reasoning), mcp = _default_mcp, tools = _no_tools})
	assert_false(_custom_fields_visible())


func test_switching_profile_keeps_a_model_the_new_catalog_lists() -> void:
	_dialog.setup({api = {provider = "openai_chat_completions", url = "http://proxy/", key = "", model = "claude-old"}})

	_select_profile("anthropic")

	assert_eq(_api_values().model, "claude-old")
	assert_eq(_dialog.model_select.get_selected_metadata(), "claude-old")


func test_switching_profile_replaces_a_model_the_new_catalog_lacks() -> void:
	_dialog.setup({api = {provider = "openai_chat_completions", url = "http://proxy/", key = "", model = "llama3"}})

	_select_profile("anthropic")

	assert_eq(_api_values().model, ANTHROPIC.model)
	assert_eq(_dialog.model_select.get_selected_metadata(), ANTHROPIC.model)


func test_switching_between_catalog_profiles_drops_the_other_providers_model() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, key = "", model = "claude-old"}})

	_select_profile("openai")

	assert_eq(_api_values().model, OPENAI.model)


func test_switching_profile_fills_an_empty_model() -> void:
	_dialog.setup({api = {provider = "anthropic", url = "http://proxy/", key = "", model = ""}})

	_select_profile("gemini")

	assert_eq(_api_values().model, Profiles.PROFILES.gemini.model)


func test_switching_to_custom_keeps_the_profile_values_as_a_starting_point() -> void:
	_dialog.setup({api = {provider = "openai_chat_completions", url = Profiles.PROFILES.gemini.url, key = "", model = "gemini-x"}})

	_select_profile(Profiles.CUSTOM)

	assert_true(_custom_fields_visible())
	assert_eq(_dialog.provider_select.get_selected_metadata(), "openai_chat_completions")
	assert_eq(_dialog.url_field.text, Profiles.PROFILES.gemini.url)
	assert_eq(_dialog.get_profile(), Profiles.CUSTOM)


func test_default_model_still_follows_after_a_detour_through_custom() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, key = "", model = ANTHROPIC.model}})

	_select_profile(Profiles.CUSTOM)
	_select_profile("openai")

	assert_eq(_api_values().model, OPENAI.model)


func test_profile_lists_catalog_models_newest_first_and_selects_the_stored_one() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-old"}})

	assert_true(_dialog.model_select.visible)
	assert_false(_dialog.model_field.visible)
	assert_true(_dialog.refresh_models_button.visible)
	assert_eq(_select_metadata(_dialog.model_select), ["claude-new", "claude-old", "claude-plain"])
	assert_eq(_dialog.model_select.get_item_text(0), "Claude New")
	assert_eq(_dialog.model_select.get_selected_metadata(), "claude-old")
	assert_eq(_api_values().model, "claude-old")


func test_a_stored_model_missing_from_the_catalog_is_appended() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-x"}})

	assert_eq(_select_metadata(_dialog.model_select), ["claude-new", "claude-old", "claude-plain", "claude-x"])
	assert_eq(_dialog.model_select.get_selected_metadata(), "claude-x")


func test_custom_profile_uses_the_model_text_field() -> void:
	_dialog.setup({api = {provider = "openai_chat_completions", url = "http://localhost:11434/v1/", model = "llama3"}})

	assert_true(_dialog.model_field.visible)
	assert_false(_dialog.model_select.visible)
	assert_false(_dialog.refresh_models_button.visible)
	assert_eq(_dialog.model_field.text, "llama3")

	_dialog.model_field.text = "llama4"
	_dialog.model_field.text_changed.emit("llama4")

	assert_eq(_api_values().model, "llama4")


func test_effort_options_follow_the_model() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-new", effort = "high"}})

	assert_true(_dialog.effort_select.visible)
	assert_eq(_select_metadata(_dialog.effort_select), ["", "low", "high"])
	assert_eq(_dialog.effort_select.get_item_text(0), SettingsDialog.DEFAULT_EFFORT_TEXT)
	assert_eq(_api_values().effort, "high")

	_select_model("claude-plain")
	assert_false(_dialog.effort_select.visible)
	assert_eq(_api_values().effort, "", "a model without effort resets it to the default")

	_select_model("claude-new")
	assert_true(_dialog.effort_select.visible)
	assert_eq(_api_values().effort, "")


func test_effort_the_model_does_not_accept_resets_to_default() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-new", effort = "max"}})

	assert_eq(_api_values().effort, "")


func test_thinking_toggle_shown_only_for_toggle_models() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-new", thinking = false}})

	assert_true(_dialog.thinking_check.visible)
	assert_false(_dialog.thinking_check.button_pressed)
	assert_false(_api_values().thinking)

	_select_model("claude-old")
	assert_false(_dialog.thinking_check.visible)
	assert_false(_api_values().thinking, "the stored value is kept while hidden")


func test_budget_shown_only_when_model_and_provider_support_it() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-old", budget_tokens = 2048}})
	assert_true(_dialog.budget_field.visible)
	assert_eq(_dialog.budget_field.value, 2048.0)
	assert_eq(_api_values().budget_tokens, 2048)

	_select_model("claude-new")
	assert_false(_dialog.budget_field.visible)

	_dialog.setup({api = {provider = OPENAI.provider, url = OPENAI.url, model = "gpt-budget"}})
	assert_false(_dialog.budget_field.visible, "the OpenAI provider cannot send a budget")
	assert_true(_dialog.thinking_check.visible)


func test_custom_profile_offers_generic_reasoning_options() -> void:
	_dialog.setup({api = {provider = "anthropic", url = "http://proxy/", model = "m"}})
	assert_eq(_select_metadata(_dialog.effort_select), [""] + Array(SettingsDialog.GENERIC_EFFORT_VALUES))
	assert_true(_dialog.thinking_check.visible)
	assert_true(_dialog.budget_field.visible)

	_dialog.setup({api = {provider = "openai_chat_completions", url = "http://proxy/", model = "m", effort = "xhigh"}})
	assert_true(_dialog.effort_select.visible)
	assert_eq(_api_values().effort, "xhigh")
	assert_true(_dialog.thinking_check.visible)
	assert_false(_dialog.budget_field.visible)


func test_switching_the_custom_provider_updates_reasoning_controls() -> void:
	_dialog.setup({api = {provider = "anthropic", url = "http://proxy/", model = "m"}})
	assert_true(_dialog.budget_field.visible)

	_select_provider("openai_chat_completions")
	assert_false(_dialog.budget_field.visible)
	assert_true(_dialog.thinking_check.visible)
	assert_eq(_api_values().provider, "openai_chat_completions")

	_select_provider("anthropic")
	assert_true(_dialog.budget_field.visible)


func test_switching_profile_relists_models() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = ANTHROPIC.model}})

	_select_profile("openai")

	assert_eq(_select_metadata(_dialog.model_select), ["gpt-new", "gpt-budget", OPENAI.model],
		"the profile default is appended when the catalog lacks it")
	assert_eq(_api_values().model, OPENAI.model)


func test_catalog_update_relists_models() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-new"}})

	_catalog.load_from_dict({anthropic = {models = {
		"claude-new": Fixture.API.anthropic.models["claude-new"],
		"claude-newer": {id = "claude-newer", name = "Newer", release_date = "2027-01-01", tool_call = true},
	}}})

	assert_eq(_select_metadata(_dialog.model_select), ["claude-newer", "claude-new"])
	assert_eq(_api_values().model, "claude-new")


func test_refresh_button_without_a_cache_dir_stays_enabled() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-new"}})

	_dialog.refresh_models_button.pressed.emit()

	assert_false(_dialog.refresh_models_button.disabled)


func test_refresh_button_is_disabled_while_offline() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, model = "claude-new"}})
	_catalog.set_cache_dir("user://godai-test-offline")
	assert_eq(_dialog.refresh_models_button.tooltip_text, SettingsDialog.REFRESH_MODELS_TOOLTIP)

	_dialog.online = false
	assert_true(_dialog.refresh_models_button.disabled)
	assert_eq(_dialog.refresh_models_button.tooltip_text, SettingsDialog.OFFLINE_TOOLTIP)

	_dialog.refresh_models_button.pressed.emit()
	assert_false(_catalog.is_refreshing(), "offline never starts a request")

	_dialog.online = true
	assert_false(_dialog.refresh_models_button.disabled)
	assert_eq(_dialog.refresh_models_button.tooltip_text, SettingsDialog.REFRESH_MODELS_TOOLTIP)


func _tool_items(p_tree: Tree) -> Array:
	return p_tree.get_root().get_children().map(func (item): return item.get_text(0))


func _click_remove(p_tree: Tree, p_index: int) -> void:
	var item: TreeItem = p_tree.get_root().get_child(p_index)
	assert_eq(item.get_button_count(0), 1, "each tool has a remove button")
	p_tree.button_clicked.emit(item, 0, item.get_button_id(0, 0), MOUSE_BUTTON_LEFT)


func test_mcp_tab_sits_between_api_and_tools() -> void:
	var tabs: TabContainer = _dialog.get_node("%Tabs")

	assert_eq(range(tabs.get_tab_count()).map(func (i): return tabs.get_tab_title(i)), ["API", "MCP", "Tools"])


func test_setup_fills_the_mcp_tab() -> void:
	_dialog.setup({mcp = {base_port = 23000, port_count = 3}})

	assert_eq(_dialog.base_port_field.value, 23000.0)
	assert_eq(_dialog.port_count_field.value, 3.0)
	assert_eq(_mcp_values(), {base_port = 23000, port_count = 3})


func test_mcp_values_are_integers_and_clamped() -> void:
	_dialog.setup({})
	assert_eq(_mcp_values(), {base_port = 12120, port_count = 10}, "the scene defaults match the setting defaults")

	_dialog.base_port_field.value = 70000
	_dialog.port_count_field.value = 0

	assert_eq(_mcp_values(), {base_port = 65535, port_count = 1})
	assert_true(_mcp_values().base_port is int)


func test_setup_fills_the_tools_tab() -> void:
	_dialog.setup({tools = {auto_approve = true, allowed = PackedStringArray(["save_scene", "run_project"]), denied = PackedStringArray(["close_editor"])}})

	assert_true(_dialog.auto_approve_check.button_pressed)
	assert_eq(_tool_items(_dialog.allowed_tree), ["save_scene", "run_project"])
	assert_eq(_tool_items(_dialog.denied_tree), ["close_editor"])

	var values: Dictionary = _tool_values()
	assert_true(values.auto_approve)
	assert_eq(values.allowed, PackedStringArray(["save_scene", "run_project"]))
	assert_eq(values.denied, PackedStringArray(["close_editor"]))


func test_empty_tool_lists_show_a_none_row() -> void:
	_dialog.setup({})

	assert_eq(_tool_items(_dialog.allowed_tree), [SettingsDialog.EMPTY_TOOL_LIST_TEXT])
	assert_eq(_tool_items(_dialog.denied_tree), [SettingsDialog.EMPTY_TOOL_LIST_TEXT])
	var none: TreeItem = _dialog.allowed_tree.get_root().get_child(0)
	assert_false(none.is_selectable(0))
	assert_eq(none.get_button_count(0), 0, "nothing to remove")
	assert_eq(_tool_values().allowed, PackedStringArray(), "the placeholder is not a tool")


func test_removing_the_last_tool_shows_the_none_row() -> void:
	_dialog.setup({tools = {denied = PackedStringArray(["close_editor"])}})

	_click_remove(_dialog.denied_tree, 0)

	assert_eq(_tool_items(_dialog.denied_tree), [SettingsDialog.EMPTY_TOOL_LIST_TEXT])
	assert_eq(_tool_values().denied, PackedStringArray())


func test_setup_replaces_earlier_tool_lists() -> void:
	_dialog.setup({tools = {allowed = PackedStringArray(["save_scene"])}})
	_dialog.setup({tools = {allowed = PackedStringArray(["run_project"])}})

	assert_eq(_tool_items(_dialog.allowed_tree), ["run_project"])


func test_remove_button_drops_the_tool() -> void:
	_dialog.setup({tools = {allowed = PackedStringArray(["save_scene", "run_project", "stop_project"]), denied = PackedStringArray(["close_editor"])}})

	_click_remove(_dialog.allowed_tree, 1)

	assert_eq(_tool_items(_dialog.allowed_tree), ["save_scene", "stop_project"])
	assert_eq(_tool_values().allowed, PackedStringArray(["save_scene", "stop_project"]))
	assert_eq(_tool_values().denied, PackedStringArray(["close_editor"]), "the other list is untouched")

	_click_remove(_dialog.denied_tree, 0)
	assert_eq(_tool_values().denied, PackedStringArray())


func test_remove_ignores_other_mouse_buttons() -> void:
	_dialog.setup({tools = {allowed = PackedStringArray(["save_scene"])}})
	var item: TreeItem = _dialog.allowed_tree.get_root().get_child(0)

	_dialog.allowed_tree.button_clicked.emit(item, 0, item.get_button_id(0, 0), MOUSE_BUTTON_RIGHT)

	assert_eq(_tool_items(_dialog.allowed_tree), ["save_scene"])


func test_auto_approve_toggle_is_reported() -> void:
	_dialog.setup({})
	assert_false(_tool_values().auto_approve)

	_dialog.auto_approve_check.button_pressed = true

	assert_true(_tool_values().auto_approve)


func test_closing_emits_the_values() -> void:
	_dialog.setup({api = {provider = ANTHROPIC.provider, url = ANTHROPIC.url, key = "sk-1", model = "m"}})
	watch_signals(_dialog)

	_dialog.confirmed.emit()
	assert_signal_emitted_with_parameters(_dialog, "closed", [_dialog.get_values()])

	_dialog.canceled.emit()
	assert_signal_emit_count(_dialog, "closed", 2)
