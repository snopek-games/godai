extends RefCounted

var id: String
var name: String
var release_date: String
var deprecated := false
var tool_call := false
var reasoning := false
var effort_values: PackedStringArray
var thinking_toggle := false
var budget_tokens_min := -1
var output_limit := 0


func _init(p_data: Dictionary = {}) -> void:
	id = str(p_data.get("id", ""))
	name = str(p_data.get("name", id))
	release_date = str(p_data.get("release_date", ""))
	deprecated = p_data.get("status") == "deprecated"
	tool_call = bool(p_data.get("tool_call", false))
	reasoning = bool(p_data.get("reasoning", false))

	var limit = p_data.get("limit")
	if limit is Dictionary:
		output_limit = int(limit.get("output", 0))

	var options = p_data.get("reasoning_options")
	if options is Array:
		for option in options:
			if not option is Dictionary:
				continue
			match str(option.get("type", "")):
				"effort":
					var values = option.get("values")
					if values is Array:
						effort_values = PackedStringArray(values)
				"toggle":
					thinking_toggle = true
				"budget_tokens":
					budget_tokens_min = int(option.get("min", 0))


func supports_effort() -> bool:
	return not effort_values.is_empty()


func accepts_effort(p_effort: String) -> bool:
	return p_effort in effort_values


func supports_budget_tokens() -> bool:
	return budget_tokens_min >= 0
