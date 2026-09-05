extends Node

const ModelInfo = preload("res://addons/godai/chat/model_info.gd")

const API_URL := "https://models.dev/api.json"
const BUNDLED_PATH := "res://addons/godai/chat/models.json"
const CACHE_FILE := "models.json"
const ETAG_FILE := "models.etag"
const STALE_SECONDS := 24 * 60 * 60

signal updated
signal refresh_finished(p_ok: bool)

var provider_ids: PackedStringArray
var cache_path: String
var etag_path: String

var _providers: Dictionary
var _http: HTTPRequest


func _init(p_provider_ids: PackedStringArray, p_cache_dir := "") -> void:
	provider_ids = p_provider_ids
	if not p_cache_dir.is_empty():
		set_cache_dir(p_cache_dir)


func set_cache_dir(p_dir: String) -> void:
	cache_path = p_dir.path_join(CACHE_FILE)
	etag_path = p_dir.path_join(ETAG_FILE)


static func trim(p_api: Dictionary, p_provider_ids: PackedStringArray) -> Dictionary:
	var trimmed := {}
	for id in p_provider_ids:
		if p_api.get(id) is Dictionary:
			trimmed[id] = p_api[id]
	return trimmed


func load_from_dict(p_api: Dictionary) -> void:
	_providers = trim(p_api, provider_ids)
	updated.emit()


## The bundled snapshot, with any provider that has a newer cached copy replaced.
func load_from_disk() -> void:
	var providers := {}
	var bundled = _read_json(BUNDLED_PATH)
	if bundled is Dictionary:
		providers = trim(bundled, provider_ids)
	if not cache_path.is_empty():
		var cached = _read_json(cache_path)
		if cached is Dictionary:
			providers.merge(trim(cached, provider_ids), true)
	_providers = providers
	updated.emit()


func get_models(p_provider_id: String) -> Array[ModelInfo]:
	var models: Array[ModelInfo]
	var entries = _model_entries(p_provider_id)
	for entry in entries.values():
		if not entry is Dictionary:
			continue
		var info := ModelInfo.new(entry)
		if info.tool_call and not info.deprecated:
			models.push_back(info)
	models.sort_custom(func (a: ModelInfo, b: ModelInfo):
		if a.release_date != b.release_date:
			return a.release_date > b.release_date
		return a.id < b.id)
	return models


func get_model(p_provider_id: String, p_model_id: String) -> ModelInfo:
	var entry = _model_entries(p_provider_id).get(p_model_id)
	if entry is Dictionary:
		return ModelInfo.new(entry)
	return null


func _model_entries(p_provider_id: String) -> Dictionary:
	var provider = _providers.get(p_provider_id)
	if provider is Dictionary and provider.get("models") is Dictionary:
		return provider["models"]
	return {}


func is_stale() -> bool:
	if cache_path.is_empty() or not FileAccess.file_exists(cache_path):
		return true
	return Time.get_unix_time_from_system() - FileAccess.get_modified_time(cache_path) > STALE_SECONDS


func is_refreshing() -> bool:
	return _http != null


func refresh_if_stale() -> void:
	if is_stale():
		refresh()


func refresh() -> void:
	if _http or cache_path.is_empty():
		return

	_http = HTTPRequest.new()
	add_child(_http)
	_http.request_completed.connect(_on_refresh_completed)

	var headers := PackedStringArray()
	if FileAccess.file_exists(etag_path):
		var etag := FileAccess.get_file_as_string(etag_path).strip_edges()
		if not etag.is_empty():
			headers.push_back("If-None-Match: " + etag)

	var err := _http.request(API_URL, headers)
	if err != OK:
		push_warning("Godai: unable to request %s: %s" % [API_URL, error_string(err)])
		_finish_refresh(false)


func _on_refresh_completed(p_result: int, p_code: int, p_headers: PackedStringArray, p_body: PackedByteArray) -> void:
	if p_result != OK:
		push_warning("Godai: fetching %s failed (HTTPRequest result %d)" % [API_URL, p_result])
		_finish_refresh(false)
		return

	if p_code == HTTPClient.RESPONSE_NOT_MODIFIED:
		_touch_cache()
		_finish_refresh(true)
		return

	var data = JSON.parse_string(p_body.get_string_from_utf8()) if p_code == HTTPClient.RESPONSE_OK else null
	if not data is Dictionary:
		push_warning("Godai: unexpected response from %s (HTTP %d)" % [API_URL, p_code])
		_finish_refresh(false)
		return

	var trimmed := trim(data, provider_ids)
	_write_cache(JSON.stringify(trimmed), _find_etag(p_headers))
	_providers.merge(trimmed, true)
	updated.emit()
	_finish_refresh(true)


func _finish_refresh(p_ok: bool) -> void:
	if _http:
		_http.queue_free()
		_http = null
	refresh_finished.emit(p_ok)


static func _find_etag(p_headers: PackedStringArray) -> String:
	for header in p_headers:
		if header.to_lower().begins_with("etag:"):
			return header.substr(5).strip_edges()
	return ""


func _write_cache(p_json: String, p_etag: String) -> void:
	DirAccess.make_dir_recursive_absolute(cache_path.get_base_dir())
	var f := FileAccess.open(cache_path, FileAccess.WRITE)
	if f:
		f.store_string(p_json)
	f = FileAccess.open(etag_path, FileAccess.WRITE)
	if f:
		f.store_string(p_etag)


## A 304 carries no body; rewriting the file is how its modification time moves.
func _touch_cache() -> void:
	if FileAccess.file_exists(cache_path):
		var content := FileAccess.get_file_as_string(cache_path)
		var f := FileAccess.open(cache_path, FileAccess.WRITE)
		if f:
			f.store_string(content)


static func _read_json(p_path: String):
	if not FileAccess.file_exists(p_path):
		return null
	return JSON.parse_string(FileAccess.get_file_as_string(p_path))
