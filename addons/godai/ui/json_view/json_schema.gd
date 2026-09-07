@tool
extends RefCounted


static func for_value(p_schema: Dictionary, p_value) -> Dictionary:
	for key in ["oneOf", "anyOf"]:
		var branches = p_schema.get(key)
		if branches is Array:
			for branch in branches:
				if branch is Dictionary and matches_type(p_value, branch.get("type", "")):
					return branch
	return p_schema


static func child(p_schema: Dictionary, p_key) -> Dictionary:
	var sub
	if p_key is int:
		sub = p_schema.get("items")
	else:
		var properties = p_schema.get("properties")
		if properties is Dictionary and properties.has(p_key):
			sub = properties[p_key]
		else:
			sub = p_schema.get("additionalProperties")
	return sub if sub is Dictionary else {}


static func description(p_schema: Dictionary) -> String:
	return str(p_schema.get("description", ""))


static func media_type(p_schema: Dictionary) -> String:
	return str(p_schema.get("contentMediaType", ""))


static func matches_type(p_value, p_type) -> bool:
	if p_type is Array:
		return p_type.any(func (t): return matches_type(p_value, t))
	match p_type:
		"object":
			return p_value is Dictionary
		"array":
			return p_value is Array
		"string":
			return p_value is String or p_value is StringName
		"boolean":
			return p_value is bool
		"integer", "number":
			return p_value is int or p_value is float
		"null":
			return p_value == null
	return true
