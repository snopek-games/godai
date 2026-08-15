# Run by the harness via execute_editor_script while the editor is still open;
# a snippet, not a standalone script, so no extends and no functions.

var checks: Array = []

var player_script: Script = load("res://player.gd")
var open_scripts: Array = EditorInterface.get_script_editor().get_open_scripts()
var script_open: bool = player_script != null and open_scripts.has(player_script)
checks.append({
	"name": "player_script_open",
	"ok": script_open,
	"detail": "" if script_open else "res://player.gd is not open in the script editor",
})

var stats: Resource = load("res://player_stats.tres")
var inspected: Object = EditorInterface.get_inspector().get_edited_object()
var stats_inspected: bool = stats != null and inspected == stats
checks.append({
	"name": "player_stats_in_inspector",
	"ok": stats_inspected,
	"detail": "" if stats_inspected else "res://player_stats.tres is not the object in the inspector",
})

var passed_count: int = 0
for c in checks:
	if c["ok"]:
		passed_count += 1

print("GODAI_VERIFY_JSON:" + JSON.stringify({
	"passed": passed_count == checks.size(),
	"checks_passed": passed_count,
	"checks_total": checks.size(),
	"checks": checks,
}))
