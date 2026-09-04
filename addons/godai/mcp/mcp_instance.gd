extends RefCounted

const Utils = preload("res://addons/godai/utils.gd")

const USER_PATH := "godai/instances"
const PROJECT_FILE := ".godot/godai-instance.json"
const TOKEN_LENGTH := 32

var instance_id: String
var secret: String

var _project_file_override: String
var _user_dir_override: String


func _init(p_project_file_override := "", p_user_dir_override := "") -> void:
	instance_id = _generate_string(TOKEN_LENGTH)
	secret = _generate_string(TOKEN_LENGTH)
	_project_file_override = p_project_file_override
	_user_dir_override = p_user_dir_override


static func _generate_string(p_len: int) -> String:
	const CHARS := "abcdefghijklmnopqrstuvwxyz0123456789"

	var rng := RandomNumberGenerator.new()
	rng.randomize()

	var ret: String
	for i in range(p_len):
		ret += CHARS[rng.randi_range(0, CHARS.length() - 1)]
	return ret


func get_user_file_path() -> String:
	var path := _user_dir_override if not _user_dir_override.is_empty() else OS.get_cache_dir() + "/" + USER_PATH
	if not DirAccess.dir_exists_absolute(path):
		DirAccess.make_dir_recursive_absolute(path)
	return path + "/" + instance_id + ".json"


func _get_project_file_path() -> String:
	if not _project_file_override.is_empty():
		return _project_file_override
	return Utils.get_project_path() + "/" + PROJECT_FILE


func claim_project() -> Error:
	var instance_file_path = _get_project_file_path()

	# If there's an existing project instance file, check if it's valid.
	if FileAccess.file_exists(instance_file_path):
		var content := FileAccess.get_file_as_string(instance_file_path)
		var data = JSON.parse_string(content)
		if data is Dictionary:
			var old_instance_id = data.get("instance_id", "")
			var old_pid = int(data.get("pid", 0))

			# If this is our instance, then we're good!
			if old_instance_id == instance_id and old_pid == OS.get_process_id():
				return OK

			# Our own pid with a different instance id is a stale claim left by a
			# previous incarnation of the plugin (e.g. a plugin reload).
			if old_pid != OS.get_process_id() and Utils.is_pid_running(old_pid):
				return ERR_ALREADY_IN_USE

		# If we made it this far, then the old instance is invalid, so remove it.
		var err = DirAccess.remove_absolute(instance_file_path)
		if err != OK:
			return err

	# Write a new project instance file.
	var f := FileAccess.open(instance_file_path, FileAccess.WRITE)
	if not f:
		return FileAccess.get_open_error()

	var data := {
		instance_id = instance_id,
		pid = OS.get_process_id(),
	}
	f.store_string(JSON.stringify(data))
	f.flush()

	return OK


func release_project() -> void:
	var instance_file_path = _get_project_file_path()

	if FileAccess.file_exists(instance_file_path):
		var content := FileAccess.get_file_as_string(instance_file_path)
		var data = JSON.parse_string(content)
		if data is Dictionary:
			var old_instance_id = data.get("instance_id", "")
			var old_pid = int(data.get("pid", 0))
			# This is our instance file, so delete it.
			if old_instance_id == instance_id and old_pid == OS.get_process_id():
				DirAccess.remove_absolute(instance_file_path)


func write_user_file(p_port: int) -> Error:
	var instance_file_path = get_user_file_path()

	var f := FileAccess.open(instance_file_path, FileAccess.WRITE)
	if not f:
		return FileAccess.get_open_error()

	var data := {
		instance_id = instance_id,
		pid = OS.get_process_id(),
		project_path = Utils.get_project_path(),
		secret = secret,
		port = p_port,
	}
	f.store_string(JSON.stringify(data))
	f.flush()

	return OK


func delete_user_file() -> void:
	var instance_file_path = get_user_file_path()
	if FileAccess.file_exists(instance_file_path):
		DirAccess.remove_absolute(instance_file_path)
