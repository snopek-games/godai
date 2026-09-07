@tool
extends RefCounted

enum Severity { NONE, ERROR, WARNING, NOTE }

const FALLBACK_ERROR_COLOR := Color(1.0, 0.47, 0.42)
const FALLBACK_WARNING_COLOR := Color(1.0, 0.87, 0.4)
const FALLBACK_DIM_COLOR := Color(1.0, 1.0, 1.0, 0.6)


static func editor_theme(p_node: Node) -> Theme:
	if Engine.is_editor_hint() and not p_node.is_part_of_edited_scene():
		return EditorInterface.get_editor_theme()
	return null


static func severity_color(p_severity: Severity, p_node: Node) -> Color:
	var theme := editor_theme(p_node)
	match p_severity:
		Severity.ERROR:
			return theme.get_color(&"error_color", &"Editor") if theme else FALLBACK_ERROR_COLOR
		Severity.WARNING:
			return theme.get_color(&"warning_color", &"Editor") if theme else FALLBACK_WARNING_COLOR
	return dim_color(p_node)


static func dim_color(p_node: Node) -> Color:
	var theme := editor_theme(p_node)
	return theme.get_color(&"disabled_font_color", &"Editor") if theme else FALLBACK_DIM_COLOR


static func icon(p_name: StringName, p_node: Node) -> Texture2D:
	var theme := editor_theme(p_node)
	if theme and p_name != &"" and theme.has_icon(p_name, &"EditorIcons"):
		return theme.get_icon(p_name, &"EditorIcons")
	return null


static func source_font(p_node: Node) -> Font:
	var theme := editor_theme(p_node)
	return theme.get_font(&"source", &"EditorFonts") if theme else null


static func is_scalar(p_value) -> bool:
	return not (p_value is Dictionary or p_value is Array)


static func format_scalar(p_value) -> String:
	if p_value == null:
		return "null"
	if p_value is float and p_value == floor(p_value) and absf(p_value) < 1e15:
		return str(int(p_value))
	return str(p_value)
