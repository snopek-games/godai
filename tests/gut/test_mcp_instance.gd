extends GutTest

const MCPInstance = preload("res://addons/godai/mcp/mcp_instance.gd")

var _temp_dir: String


func before_each() -> void:
	_temp_dir = OS.get_cache_dir() + "/godai-test-mcp-instance-%d" % Time.get_ticks_usec()
	DirAccess.make_dir_recursive_absolute(_temp_dir)


func after_each() -> void:
	var dir := DirAccess.open(_temp_dir)
	if dir:
		dir.list_dir_begin()
		var fn := dir.get_next()
		while fn != "":
			if not dir.current_is_dir():
				dir.remove(fn)
			fn = dir.get_next()
	DirAccess.remove_absolute(_temp_dir)


func _project_file() -> String:
	return _temp_dir + "/instance.json"


func _make_instance() -> MCPInstance:
	return MCPInstance.new(_project_file(), _temp_dir)


func _read_json(p_path: String) -> Dictionary:
	var data = JSON.parse_string(FileAccess.get_file_as_string(p_path))
	return data if data is Dictionary else {}


func test_claim_project_writes_instance_file() -> void:
	var instance := _make_instance()

	assert_eq(instance.claim_project(), OK)

	var data := _read_json(_project_file())
	assert_eq(data.get("instance_id"), instance.instance_id)
	assert_eq(int(data.get("pid", 0)), OS.get_process_id())


func test_claim_project_is_idempotent() -> void:
	var instance := _make_instance()

	assert_eq(instance.claim_project(), OK)
	assert_eq(instance.claim_project(), OK)


func test_claim_project_fails_when_a_live_instance_holds_the_project() -> void:
	var live_pid: int
	if OS.get_name() == "Windows":
		live_pid = OS.create_process("ping", ["-n", "31", "127.0.0.1"])
	else:
		live_pid = OS.create_process("sleep", ["30"])
	assert_gt(live_pid, 0, "the scratch process standing in for another editor spawned")

	var f := FileAccess.open(_project_file(), FileAccess.WRITE)
	f.store_string(JSON.stringify({instance_id = "other-editor", pid = live_pid}))
	f.close()

	var instance := _make_instance()
	assert_eq(instance.claim_project(), ERR_ALREADY_IN_USE)

	OS.kill(live_pid)


func test_claim_project_replaces_an_earlier_instance_from_this_process() -> void:
	var old := _make_instance()
	assert_eq(old.claim_project(), OK)

	var instance := _make_instance()
	assert_eq(instance.claim_project(), OK)
	assert_eq(_read_json(_project_file()).get("instance_id"), instance.instance_id)

	old.release_project()
	assert_true(FileAccess.file_exists(_project_file()),
		"the old instance's release leaves the new claim alone")


func test_claim_project_replaces_a_stale_instance_file() -> void:
	var f := FileAccess.open(_project_file(), FileAccess.WRITE)
	f.store_string(JSON.stringify({instance_id = "stale", pid = -1}))
	f.close()

	var instance := _make_instance()
	assert_eq(instance.claim_project(), OK)

	assert_eq(_read_json(_project_file()).get("instance_id"), instance.instance_id)


func test_release_project_only_deletes_own_file() -> void:
	var other := _make_instance()
	assert_eq(other.claim_project(), OK)

	var instance := _make_instance()
	instance.release_project()
	assert_true(FileAccess.file_exists(_project_file()))

	other.release_project()
	assert_false(FileAccess.file_exists(_project_file()))


func test_user_file_contains_required_keys() -> void:
	var instance := _make_instance()

	assert_eq(instance.write_user_file(1234), OK)

	var data := _read_json(instance.get_user_file_path())
	assert_eq(data.get("instance_id"), instance.instance_id)
	assert_eq(int(data.get("pid", 0)), OS.get_process_id())
	assert_eq(data.get("project_path"), ProjectSettings.globalize_path("res://").simplify_path())
	assert_eq(data.get("secret"), instance.secret)
	assert_eq(int(data.get("port", 0)), 1234)

	instance.delete_user_file()
	assert_false(FileAccess.file_exists(instance.get_user_file_path()))
