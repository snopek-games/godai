@tool
extends AcceptDialog

const ChatClient = preload("res://addons/godai/chat/client.gd")
const Profiles = preload("res://addons/godai/chat/profiles.gd")
const ModelCatalog = preload("res://addons/godai/chat/model_catalog.gd")
const ModelInfo = preload("res://addons/godai/chat/model_info.gd")

const EMPTY_TOOL_LIST_TEXT := "(none)"
const DEFAULT_EFFORT_TEXT := "Model default"
const REFRESH_MODELS_TOOLTIP := "Refresh the list of models from models.dev"
const OFFLINE_TOOLTIP := "Refreshing is unavailable while the editor is in offline mode (Editor Settings > Network > Connection)"
## Offered for Custom endpoints, where no catalog says which values the model takes.
const GENERIC_EFFORT_VALUES: PackedStringArray = ["none", "minimal", "low", "medium", "high", "xhigh", "max"]

@onready var profile_select: OptionButton = %ProfileSelect
@onready var provider_label: Label = %ProviderLabel
@onready var provider_select: OptionButton = %ProviderSelect
@onready var url_label: Label = %URLLabel
@onready var url_field: LineEdit = %URLField
@onready var key_field: LineEdit = %KeyField
@onready var model_select: OptionButton = %ModelSelect
@onready var model_field: LineEdit = %ModelField
@onready var refresh_models_button: Button = %RefreshModelsButton
@onready var effort_label: Label = %EffortLabel
@onready var effort_select: OptionButton = %EffortSelect
@onready var thinking_label: Label = %ThinkingLabel
@onready var thinking_check: CheckBox = %ThinkingCheck
@onready var budget_label: Label = %BudgetLabel
@onready var budget_field: SpinBox = %BudgetField
@onready var base_port_field: SpinBox = %BasePortField
@onready var port_count_field: SpinBox = %PortCountField
@onready var auto_approve_check: CheckBox = %AutoApproveCheck
@onready var allowed_tree: Tree = %AllowedTree
@onready var denied_tree: Tree = %DeniedTree

signal closed(values: Dictionary)

var catalog: ModelCatalog:
	set(p_catalog):
		if catalog:
			catalog.updated.disconnect(_on_catalog_updated)
			catalog.refresh_finished.disconnect(_on_catalog_refresh_finished)
		catalog = p_catalog
		if catalog:
			catalog.updated.connect(_on_catalog_updated)
			catalog.refresh_finished.connect(_on_catalog_refresh_finished)

var online := true:
	set(p_online):
		online = p_online
		if is_node_ready():
			_update_refresh_button()

var _model_id: String


func _ready() -> void:
	if is_part_of_edited_scene():
		return

	for id in Profiles.PROFILES:
		profile_select.add_item(Profiles.PROFILES[id].name)
		profile_select.set_item_metadata(profile_select.item_count - 1, String(id))
	profile_select.add_item("Custom")
	profile_select.set_item_metadata(profile_select.item_count - 1, Profiles.CUSTOM)

	for name in ChatClient.PROVIDERS:
		provider_select.add_item(name)
		provider_select.set_item_metadata(provider_select.item_count - 1, String(name))

	if Engine.is_editor_hint():
		refresh_models_button.icon = EditorInterface.get_editor_theme().get_icon("Reload", "EditorIcons")
	else:
		refresh_models_button.text = "Refresh"

	provider_select.item_selected.connect(_on_provider_select_item_selected)
	model_select.item_selected.connect(_on_model_select_item_selected)
	model_field.text_changed.connect(_on_model_field_text_changed)
	refresh_models_button.pressed.connect(_on_refresh_models_button_pressed)
	_update_refresh_button()

	allowed_tree.button_clicked.connect(_on_tool_list_button_clicked)
	denied_tree.button_clicked.connect(_on_tool_list_button_clicked)

	confirmed.connect(_on_closed)
	canceled.connect(_on_closed)


func _notification(p_what: int) -> void:
	match p_what:
		NOTIFICATION_THEME_CHANGED:
			_use_tabbed_dialog_background()


func _use_tabbed_dialog_background() -> void:
	if not Engine.is_editor_hint() or is_part_of_edited_scene():
		return

	# Use the same style as editor settings dialog so tabs look OK.
	var editor_theme := EditorInterface.get_editor_theme()
	if not editor_theme.has_stylebox("panel", "EditorSettingsDialog"):
		return
	var stylebox := editor_theme.get_stylebox("panel", "EditorSettingsDialog")
	if get_theme_stylebox("panel") == stylebox:
		return
	add_theme_stylebox_override("panel", stylebox)


## Takes the same nested Dictionary that get_values() returns, one entry per tab.
func setup(p_values: Dictionary) -> void:
	_setup_api(p_values.get("api", {}))
	_setup_mcp(p_values.get("mcp", {}))
	_setup_tools(p_values.get("tools", {}))


func _setup_api(p_values: Dictionary) -> void:
	var provider: String = p_values.get("provider", "")
	var url: String = p_values.get("url", "")
	var profile := Profiles.find(provider, url)

	_select_by_metadata(provider_select, provider)
	url_field.text = url
	key_field.text = p_values.get("key", "")
	_model_id = str(p_values.get("model", ""))
	thinking_check.button_pressed = p_values.get("thinking", true)
	budget_field.value = p_values.get("budget_tokens", 0)

	_select_by_metadata(profile_select, profile)
	_update_profile_fields()
	_update_model_controls(str(p_values.get("effort", "")))


func _setup_mcp(p_values: Dictionary) -> void:
	base_port_field.value = p_values.get("base_port", base_port_field.value)
	port_count_field.value = p_values.get("port_count", port_count_field.value)


func _setup_tools(p_values: Dictionary) -> void:
	auto_approve_check.button_pressed = p_values.get("auto_approve", false)
	_fill_tool_list(allowed_tree, p_values.get("allowed", PackedStringArray()))
	_fill_tool_list(denied_tree, p_values.get("denied", PackedStringArray()))


func get_values() -> Dictionary:
	return {
		api = _get_api_values(),
		mcp = {
			base_port = int(base_port_field.value),
			port_count = int(port_count_field.value),
		},
		tools = {
			auto_approve = auto_approve_check.button_pressed,
			allowed = _tool_list(allowed_tree),
			denied = _tool_list(denied_tree),
		},
	}


func _get_api_values() -> Dictionary:
	var profile := get_profile()
	var values := {
		provider = _selected_metadata(provider_select),
		url = url_field.text,
		key = key_field.text,
		model = _model_id,
		effort = _selected_metadata(effort_select),
		thinking = thinking_check.button_pressed,
		budget_tokens = int(budget_field.value),
	}
	if profile != Profiles.CUSTOM:
		values.provider = String(Profiles.PROFILES[profile].provider)
		values.url = String(Profiles.PROFILES[profile].url)
	return values


func get_profile() -> String:
	return _selected_metadata(profile_select)


func _on_profile_select_item_selected(_p_index: int) -> void:
	var profile := get_profile()
	if profile != Profiles.CUSTOM:
		if _current_model_info() == null:
			_model_id = Profiles.PROFILES[profile].model
		_select_by_metadata(provider_select, Profiles.PROFILES[profile].provider)
		url_field.text = Profiles.PROFILES[profile].url
	_update_profile_fields()
	_update_model_controls(_selected_metadata(effort_select))


func _update_profile_fields() -> void:
	var custom := get_profile() == Profiles.CUSTOM
	provider_label.visible = custom
	provider_select.visible = custom
	url_label.visible = custom
	url_field.visible = custom


func _update_model_controls(p_effort: String) -> void:
	var custom := get_profile() == Profiles.CUSTOM
	model_field.visible = custom
	model_select.visible = not custom
	refresh_models_button.visible = not custom

	if custom:
		model_field.text = _model_id
	else:
		model_select.clear()
		var listed := false
		for info in _profile_models():
			model_select.add_item(info.name)
			model_select.set_item_metadata(model_select.item_count - 1, info.id)
			listed = listed or info.id == _model_id
		if not listed and not _model_id.is_empty():
			model_select.add_item(_model_id)
			model_select.set_item_metadata(model_select.item_count - 1, _model_id)
		_select_by_metadata(model_select, _model_id)

	_update_reasoning_controls(p_effort)


func _profile_models() -> Array[ModelInfo]:
	if not catalog:
		return []
	return catalog.get_models(Profiles.models_dev_id(get_profile()))


func _current_model_info() -> ModelInfo:
	if not catalog or get_profile() == Profiles.CUSTOM:
		return null
	return catalog.get_model(Profiles.models_dev_id(get_profile()), _model_id)


func _current_provider_name() -> String:
	if get_profile() == Profiles.CUSTOM:
		return _selected_metadata(provider_select)
	return String(Profiles.PROFILES[get_profile()].provider)


func _provider_reasoning_options() -> PackedStringArray:
	var script: GDScript = ChatClient.PROVIDERS.get(_current_provider_name())
	if not script:
		return PackedStringArray()
	return script.get_reasoning_options()


## An option is offered when the provider can send it and the model (if known) takes it.
func _update_reasoning_controls(p_effort: String) -> void:
	var info := _current_model_info()
	var supported := _provider_reasoning_options()

	var show_effort := "effort" in supported and (info == null or info.supports_effort())
	effort_label.visible = show_effort
	effort_select.visible = show_effort
	var effort_values := info.effort_values if info else GENERIC_EFFORT_VALUES
	effort_select.clear()
	effort_select.add_item(DEFAULT_EFFORT_TEXT)
	effort_select.set_item_metadata(0, "")
	for value in effort_values:
		effort_select.add_item(value)
		effort_select.set_item_metadata(effort_select.item_count - 1, value)
	_select_by_metadata(effort_select, p_effort if p_effort in effort_values else "")

	var show_thinking := "toggle" in supported and (info == null or info.thinking_toggle)
	thinking_label.visible = show_thinking
	thinking_check.visible = show_thinking

	var show_budget := "budget_tokens" in supported and (info == null or info.supports_budget_tokens())
	budget_label.visible = show_budget
	budget_field.visible = show_budget


func _on_provider_select_item_selected(_p_index: int) -> void:
	_update_reasoning_controls(_selected_metadata(effort_select))


func _on_model_select_item_selected(_p_index: int) -> void:
	_model_id = _selected_metadata(model_select)
	_update_reasoning_controls(_selected_metadata(effort_select))


func _on_model_field_text_changed(p_text: String) -> void:
	_model_id = p_text


func _on_refresh_models_button_pressed() -> void:
	if catalog and online:
		catalog.refresh()
		_update_refresh_button()


func _update_refresh_button() -> void:
	refresh_models_button.disabled = not online or (catalog != null and catalog.is_refreshing())
	refresh_models_button.tooltip_text = REFRESH_MODELS_TOOLTIP if online else OFFLINE_TOOLTIP


func _on_catalog_updated() -> void:
	if is_node_ready():
		_update_model_controls(_selected_metadata(effort_select))


func _on_catalog_refresh_finished(_p_ok: bool) -> void:
	_update_refresh_button()


static func _selected_metadata(p_select: OptionButton) -> String:
	if p_select.selected < 0:
		return ""
	return str(p_select.get_selected_metadata())


static func _select_by_metadata(p_select: OptionButton, p_value: String) -> void:
	for i in p_select.item_count:
		if p_select.get_item_metadata(i) == p_value:
			p_select.select(i)
			return


func _fill_tool_list(p_tree: Tree, p_tools: PackedStringArray) -> void:
	p_tree.clear()
	var root := p_tree.create_item()
	for tool_name in p_tools:
		var item := p_tree.create_item(root)
		item.set_text(0, tool_name)
		item.add_button(0, _remove_icon(), 0, false, "Remove")
	_update_empty_tool_list(p_tree)


func _update_empty_tool_list(p_tree: Tree) -> void:
	var root := p_tree.get_root()
	if root.get_child_count() > 0:
		return
	var item := p_tree.create_item(root)
	item.set_text(0, EMPTY_TOOL_LIST_TEXT)
	item.set_selectable(0, false)
	item.set_custom_color(0, p_tree.get_theme_color("font_disabled_color"))


func _is_empty_tool_list_item(p_item: TreeItem) -> bool:
	return p_item.get_button_count(0) == 0


func _tool_list(p_tree: Tree) -> PackedStringArray:
	var tools := PackedStringArray()
	var root := p_tree.get_root()
	if root:
		for item in root.get_children():
			if not _is_empty_tool_list_item(item):
				tools.push_back(item.get_text(0))
	return tools


func _remove_icon() -> Texture2D:
	if Engine.is_editor_hint():
		return EditorInterface.get_editor_theme().get_icon("Remove", "EditorIcons")
	return get_theme_icon("close", "Tree")


func _on_tool_list_button_clicked(p_item: TreeItem, _p_column: int, _p_id: int, p_mouse_button: int) -> void:
	if p_mouse_button == MOUSE_BUTTON_LEFT:
		var tree := p_item.get_tree()
		p_item.get_parent().remove_child(p_item)
		p_item.free()
		_update_empty_tool_list(tree)


func _on_closed() -> void:
	closed.emit(get_values())
