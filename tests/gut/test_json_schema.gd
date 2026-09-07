extends GutTest

const JsonSchema = preload("res://addons/godai/ui/json_view/json_schema.gd")

const SCHEMA := {
	type = "object",
	properties = {
		name = {type = "string", description = "The name"},
		code = {type = "string", contentMediaType = "text/x-gdscript"},
		items = {type = "array", items = {type = "integer", description = "An item"}},
		nodes = {
			type = "object",
			additionalProperties = {
				oneOf = [
					{type = "object", description = "A map"},
					{type = "string", description = "An error"},
				],
			},
		},
	},
}


func test_child_walks_properties_items_and_additional_properties() -> void:
	assert_eq(JsonSchema.description(JsonSchema.child(SCHEMA, "name")), "The name")
	assert_eq(JsonSchema.description(JsonSchema.child(JsonSchema.child(SCHEMA, "items"), 0)), "An item")
	assert_true(JsonSchema.child(JsonSchema.child(SCHEMA, "nodes"), "Player").has("oneOf"))
	assert_eq(JsonSchema.child(SCHEMA, "unknown"), {})
	assert_eq(JsonSchema.child({}, 3), {})


func test_for_value_picks_the_one_of_branch_matching_the_value() -> void:
	var node_schema := JsonSchema.child(JsonSchema.child(SCHEMA, "nodes"), "Player")
	assert_eq(JsonSchema.description(JsonSchema.for_value(node_schema, {})), "A map")
	assert_eq(JsonSchema.description(JsonSchema.for_value(node_schema, "boom")), "An error")
	assert_eq(JsonSchema.for_value(node_schema, 3.0), node_schema, "no matching branch keeps the schema")
	assert_eq(JsonSchema.for_value(SCHEMA, {}), SCHEMA)


func test_media_type_reads_content_media_type() -> void:
	assert_eq(JsonSchema.media_type(JsonSchema.child(SCHEMA, "code")), "text/x-gdscript")
	assert_eq(JsonSchema.media_type(JsonSchema.child(SCHEMA, "name")), "")


func test_matches_type_treats_json_numbers_as_integers_and_accepts_type_lists() -> void:
	assert_true(JsonSchema.matches_type(1.0, "integer"))
	assert_true(JsonSchema.matches_type(2, "number"))
	assert_false(JsonSchema.matches_type("1", "integer"))
	assert_true(JsonSchema.matches_type(null, "null"))
	assert_true(JsonSchema.matches_type(true, ["string", "boolean"]))
	assert_false(JsonSchema.matches_type([], ["string", "boolean"]))
	assert_true(JsonSchema.matches_type([], ""), "no declared type matches anything")
