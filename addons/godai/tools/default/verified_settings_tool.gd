## Base for the tools that set editor/project settings: the same prepare/verify
## flow as the property tools, applied to a settings singleton.
@abstract
extends "res://addons/godai/tools/default/verified_property_tool.gd"

const ToolResult = preload("res://addons/godai/tools/tool_manager.gd").ToolResult


## The settings singleton the tool operates on.
@abstract
func get_settings_object() -> Object


## An error message when the setting is off-limits, or "".
@abstract
func check_setting_allowed(p_name: String) -> String


## An error message when persisting the settings failed, or "".
func save_settings() -> String:
	return ""


## Whether a dotted name whose base is a known setting counts as known (a
## feature-tag override, e.g. "application/config/name.web").
func allows_feature_overrides() -> bool:
	return false


func _discovery_hint() -> String:
	return ' (%s with "include_defaults": true lists the valid settings)' % get_properties_tool_name()


func execute(p_input) -> ToolResult:
	var settings: Dictionary = p_input.get('settings', {})
	var create_missing: bool = bool(p_input.get('create_missing', false))

	for name in settings:
		var not_allowed := check_setting_allowed(name)
		if not not_allowed.is_empty():
			return ToolResult.rejected({errors = [not_allowed]})

	var settings_object := get_settings_object()

	var errors := PackedStringArray()
	var warnings := PackedStringArray()
	var ops := []
	var prop_cache := {}

	logger.start()

	# Each setting is prepared and set independently: one bad setting doesn't
	# stop the others from being applied.
	for name in settings:
		# A feature-tag override is validated (type and enum hint included)
		# against its base setting.
		var resolve_name: String = name
		if not settings_object.has_setting(name):
			var base := _override_base(name)
			if not base.is_empty() and settings_object.has_setting(base):
				resolve_name = base
			elif not create_missing:
				errors.append('%s: no such setting%s (pass "create_missing": true to create it)%s' % [name, Utils.property_suggestion(settings_object, name, prop_cache), _discovery_hint()])
				continue

		var prepared := prepare_property_op(settings_object, resolve_name, settings[name], prop_cache)
		if prepared.has("error"):
			errors.append("%s: %s" % [name, prepared['error']])
			continue

		var op: Dictionary = prepared['op']
		if resolve_name != name:
			op['path'] = name
			op['old_value'] = settings_object.get_indexed(name)
		op['object'] = settings_object
		op['label'] = name
		ops.append(op)

	if ops.is_empty() and not errors.is_empty():
		return ToolResult.rejected(build_rejection(errors))

	for op in ops:
		settings_object.set_setting(op['path'], op['value'])

	verify_property_ops(ops, errors, warnings)

	for op in ops:
		if not (op['usage'] & PROPERTY_USAGE_RESTART_IF_CHANGED):
			continue
		# Only when the value actually changed: an unchanged setting (or one
		# the setter refused) doesn't need a restart.
		if Utils.values_equal_approx(settings_object.get_indexed(op['path']), op['old_value']):
			continue
		warnings.append("%s: the editor must be restarted for the new value to take effect (the restart_editor tool can do that)" % op['label'])

	var save_error := save_settings()
	if not save_error.is_empty():
		errors.append(save_error)

	return ToolResult.resolved(build_result({}, errors, warnings))


func _override_base(p_name: String) -> String:
	if not allows_feature_overrides():
		return ""
	var dot := p_name.find(".")
	if dot == -1:
		return ""
	return p_name.substr(0, dot)
