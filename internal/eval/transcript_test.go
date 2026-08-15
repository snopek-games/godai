package eval

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestTranscriptKeepsWhatExplainsAFailure(t *testing.T) {
	is := is.New(t)

	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1","model":"claude","mcp_servers":[{"name":"godai","status":"connected"}]}`,
		`{"type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"thinking","thinking":"The scene needs a Sprite2D first."},` +
			`{"type":"text","text":"I'll open the scene."},` +
			`{"type":"tool_use","id":"t1","name":"mcp__godai__add_node","input":{"node_type":"Sprit2D"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[` +
			`{"type":"tool_result","tool_use_id":"t1","is_error":true,` +
			`"content":[{"type":"text","text":"Failed to create 'Sprit2D'"}]}]}}`,
		`{"type":"result","subtype":"success","num_turns":1,"total_cost_usd":0.02}`,
	}, "\n")

	att := &Attempt{Repeat: 2, Model: "sonnet", Surface: SurfaceMCP, ChecksPassed: 1, ChecksTotal: 8,
		FailedChecks: []FailedCheck{{Name: "player_exists", Detail: "no node named Player in the saved scene"}},
		VerifyLog:    "ERROR: res://main.tscn:6 - Parse Error: Can't load"}
	spec := &Spec{ID: "add-player-sprite", Instruction: "Add a Sprite2D called Player"}

	md := renderTranscript(spec, att, []byte(stream))

	for _, want := range []string{
		"# add-player-sprite r2 — sonnet/mcp",
		"FAIL, 1/8 checks — failed: player_exists",
		"- `player_exists` — no node named Player in the saved scene",
		"ERROR: res://main.tscn:6 - Parse Error: Can't load",
		"Add a Sprite2D called Player",
		"The scene needs a Sprite2D first.",
		"I'll open the scene.",
		"mcp__godai__add_node",
		`"node_type":"Sprit2D"`,
		"(ERROR)",
		"Failed to create 'Sprit2D'",
		"success, 1 turns, $0.0200",
	} {
		is.True(strings.Contains(md, want))
	}
}

func TestTranscriptRendersOracleCalls(t *testing.T) {
	is := is.New(t)

	calls := []ToolCall{bashCall("/w/shim/godai --root /w editor-tool save_scene")}
	att := &Attempt{Repeat: 1, Passed: true, Metrics: &RunMetrics{ToolCalls: calls}}

	md := renderTranscript(&Spec{ID: "t"}, att, nil)

	is.True(strings.Contains(md, "**→ save_scene**"))
	is.True(strings.Contains(md, "editor-tool save_scene"))
}
