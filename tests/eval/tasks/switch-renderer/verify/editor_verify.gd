# The project setting alone doesn't prove the restart: the running editor only
# picks up the new rendering method if it was restarted after the change.

var method := RenderingServer.get_current_rendering_method()
var ok := method == "gl_compatibility"

print("GODAI_VERIFY_JSON:" + JSON.stringify({
	"passed": ok,
	"checks_passed": 1 if ok else 0,
	"checks_total": 1,
	"checks": [{
		"name": "editor_restarted_on_compatibility",
		"ok": ok,
		"detail": "" if ok else "the running editor is on '%s'; was it restarted after the change?" % method,
	}],
}))
