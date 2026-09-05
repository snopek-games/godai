extends GutTest

const ModelCatalog = preload("res://addons/godai/chat/model_catalog.gd")
const Fixture = preload("res://tests/gut/fixtures/models_dev_fixture.gd")

const IDS: PackedStringArray = ["anthropic", "openai", "google"]

var _temp_dir: String
var _catalog: ModelCatalog


func before_each() -> void:
	_temp_dir = OS.get_cache_dir() + "/godai-test-models-%d" % Time.get_ticks_usec()
	_catalog = ModelCatalog.new(IDS, _temp_dir)
	add_child_autofree(_catalog)
	_catalog.load_from_dict(Fixture.API)


func after_each() -> void:
	var dir := DirAccess.open(_temp_dir)
	if dir:
		for fn in dir.get_files():
			dir.remove(fn)
		DirAccess.remove_absolute(_temp_dir)


func _ids(p_models: Array) -> Array:
	return p_models.map(func (m): return m.id)


func test_trim_keeps_only_the_listed_providers() -> void:
	assert_eq(ModelCatalog.trim(Fixture.API, IDS).keys(), ["anthropic", "openai", "google"])
	assert_eq(ModelCatalog.trim(Fixture.API, PackedStringArray(["nope"])), {})


func test_get_models_filters_and_sorts_newest_first() -> void:
	assert_eq(_ids(_catalog.get_models("anthropic")), ["claude-new", "claude-old", "claude-plain"],
		"deprecated and tool-less models are left out")
	assert_eq(_ids(_catalog.get_models("openai")), ["gpt-new", "gpt-budget"])
	assert_eq(_catalog.get_models("other").size(), 0, "providers outside the list are trimmed")
	assert_eq(_catalog.get_models("nope").size(), 0)


func test_get_model_parses_reasoning_options() -> void:
	var new_model := _catalog.get_model("anthropic", "claude-new")
	assert_eq(new_model.name, "Claude New")
	assert_true(new_model.reasoning)
	assert_true(new_model.supports_effort())
	assert_eq(new_model.effort_values, PackedStringArray(["low", "high"]))
	assert_true(new_model.accepts_effort("high"))
	assert_false(new_model.accepts_effort("max"))
	assert_true(new_model.thinking_toggle)
	assert_false(new_model.supports_budget_tokens())
	assert_eq(new_model.output_limit, 128000)

	var old_model := _catalog.get_model("anthropic", "claude-old")
	assert_false(old_model.supports_effort())
	assert_false(old_model.thinking_toggle)
	assert_true(old_model.supports_budget_tokens())
	assert_eq(old_model.budget_tokens_min, 1024)

	var plain := _catalog.get_model("anthropic", "claude-plain")
	assert_false(plain.reasoning)
	assert_eq(plain.output_limit, 4096)


func test_get_model_keeps_deprecated_models_reachable() -> void:
	assert_true(_catalog.get_model("anthropic", "claude-dead").deprecated)
	assert_null(_catalog.get_model("anthropic", "nope"))
	assert_null(_catalog.get_model("nope", "claude-new"))


func test_load_from_disk_uses_the_bundled_snapshot() -> void:
	var fresh := ModelCatalog.new(IDS)
	add_child_autofree(fresh)

	fresh.load_from_disk()

	assert_true(fresh.get_models("anthropic").size() > 0)
	assert_true(fresh.get_models("openai").size() > 0)
	assert_true(fresh.get_models("google").size() > 0)


func test_cache_overrides_the_bundled_snapshot() -> void:
	_catalog._write_cache(JSON.stringify(Fixture.API), '"abc"')

	var other := ModelCatalog.new(IDS, _temp_dir)
	add_child_autofree(other)
	other.load_from_disk()

	assert_not_null(other.get_model("anthropic", "claude-new"))
	assert_false(other.is_stale())
	assert_eq(FileAccess.get_file_as_string(other.etag_path), '"abc"')


func test_is_stale_without_a_cache() -> void:
	assert_true(_catalog.is_stale())
	assert_true(autofree(ModelCatalog.new(IDS)).is_stale(), "no cache dir at all")


func test_refresh_without_a_cache_dir_does_nothing() -> void:
	var fresh := ModelCatalog.new(IDS)
	add_child_autofree(fresh)

	fresh.refresh()

	assert_false(fresh.is_refreshing())


func test_find_etag() -> void:
	assert_eq(ModelCatalog._find_etag(PackedStringArray(["Content-Type: application/json", 'ETag: "x"'])), '"x"')
	assert_eq(ModelCatalog._find_etag(PackedStringArray(["Content-Type: application/json"])), "")
