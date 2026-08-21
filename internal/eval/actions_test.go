package eval

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func bashCall(command string) ToolCall {
	input, _ := json.Marshal(map[string]string{"command": command})
	return ToolCall{Name: "Bash", Input: input}
}

// Every comparison between the CLI and MCP surfaces is meaningless if the same
// operation doesn't normalize to the same action name.
func TestSurfacesNormalizeIdentically(t *testing.T) {
	is := is.New(t)

	pairs := []struct{ mcpTool, shell string }{
		{"add_node", "godai editor-tool add_node --node-type Sprite2D --parent-path ."},
		{"save_scene", "godai editor-tool save_scene"},
		{"get_current_scene_tree", "cd /w/project && godai editor-tool get_current_scene_tree"},
		{"open_godot_project", "godai project open /tmp/x --headless"},
		{"list_open_projects", "godai editor list"},
		{"list_installed_godot_versions", "/usr/local/bin/godai engine list"},
		{"save_scene", "godai --root /w/project --no-input editor-tool save_scene -p /w/project"},
		{"get_godai_settings", "godai --json config"},
	}

	for _, p := range pairs {
		mcp := NormalizeActions([]ToolCall{{Name: mcpPrefix + p.mcpTool}})
		cli := NormalizeActions([]ToolCall{bashCall(p.shell)})

		is.Equal(mcp[0].Kind, ActionKindGodai)
		is.Equal(cli[0].Kind, ActionKindGodai)
		is.Equal(cli[0].Name, mcp[0].Name)
		is.Equal(mcp[0].Name, p.mcpTool)
	}
}

func TestNormalizeClassifiesTheEditorSurface(t *testing.T) {
	is := is.New(t)

	actions := NormalizeActions([]ToolCall{
		{Name: "open_scene"},
		{Name: "add_node"},
		{Name: "Write"},
	})

	is.Equal(actions[0].Kind, ActionKindGodai)
	is.Equal(actions[0].Name, "open_scene")
	is.Equal(actions[1].Kind, ActionKindGodai)
	is.Equal(actions[2].Kind, ActionKindBuiltin)
}

func TestNormalizeClassifiesNonGodaiCalls(t *testing.T) {
	is := is.New(t)

	actions := NormalizeActions([]ToolCall{
		{Name: "Read"},
		{Name: "Write"},
		bashCall("ls -la"),
	})

	is.Equal(actions[0].Kind, ActionKindBuiltin)
	is.Equal(actions[1].Kind, ActionKindBuiltin)
	is.Equal(actions[2].Kind, ActionKindShell)
	is.Equal(actions[2].Name, "ls")
}

// Mentioning godai in an argument used to score as a godai call, which on the
// CLI surface also failed the attempt for never reaching the shim.
func TestNamingGodaiIsNotCallingIt(t *testing.T) {
	is := is.New(t)

	for _, cmd := range []string{"ls addons/godai", "grep -r godai .", "cat godai/README.md"} {
		action := NormalizeActions([]ToolCall{bashCall(cmd)})[0]
		is.Equal(action.Kind, ActionKindShell)
	}

	env := NormalizeActions([]ToolCall{bashCall("XDG_CACHE_HOME=/w/cache godai editor-tool save_scene")})[0]
	is.Equal(env.Kind, ActionKindGodai)
	is.Equal(env.Name, "save_scene")
}

// Reading the project through godai is not an edit, so it can't dilute the
// share of edits that went around godai.
func TestBypassRateCountsOnlyMutatingActions(t *testing.T) {
	is := is.New(t)

	actions := NormalizeActions([]ToolCall{
		{Name: mcpPrefix + "get_current_scene_tree"},
		{Name: mcpPrefix + "get_node_properties"},
		bashCall("godai editor list"),
		{Name: "Write"},
	})

	st := ScoreActions(actions, &Spec{})
	is.Equal(st.GodaiCalls, 3)
	is.Equal(st.MutatingCalls, 1) // just the Write; the godai calls only read
	is.Equal(st.BypassRate, 1.0)
}

func TestBaselineDeniesBashUnlessFullTools(t *testing.T) {
	is := is.New(t)

	denied := strings.Join(baselineArgs(false), " ")
	is.True(strings.Contains(denied, "--disallowedTools Bash"))
	is.True(!strings.Contains(denied, "Grep,Bash")) // Bash is not in the allowed list either

	allowed := strings.Join(baselineArgs(true), " ")
	is.True(!strings.Contains(allowed, "--disallowedTools"))
	is.True(strings.Contains(allowed, "Grep,Bash"))
}

func TestMCPSurfaceHoldsBackToolsUnlessFullTools(t *testing.T) {
	is := is.New(t)
	work := &Workspace{}

	locked := strings.Join(mcpArgs(Config{}, work), " ")
	is.True(strings.Contains(locked, "--allowedTools mcp__godai,Read,Glob,Grep"))
	is.True(strings.Contains(locked, "--disallowedTools Bash"))

	full := strings.Join(mcpArgs(Config{FullTools: true}, work), " ")
	is.True(strings.Contains(full, "--allowedTools mcp__godai,Read,Write,Edit,Glob,Grep,Bash"))
	is.True(!strings.Contains(full, "--disallowedTools"))
}

func TestHelpIsNotAnAction(t *testing.T) {
	is := is.New(t)

	actions := NormalizeActions([]ToolCall{
		bashCall("godai --help"),
		bashCall("godai editor-tool add_node --help"),
		bashCall("godai editor-tool add_node --node-type Sprite2D"),
	})
	is.Equal(actions[0].Kind, ActionKindHelp)
	is.Equal(actions[1].Kind, ActionKindHelp)
	is.Equal(actions[2].Kind, ActionKindGodai)

	st := ScoreActions(actions, &Spec{ExpectedActions: []string{"add_node"}})
	is.Equal(st.HelpCalls, 2)
	is.Equal(st.GodaiCalls, 1)
	is.Equal(st.FirstGodaiAction, "add_node")
	is.True(st.FirstActionCorrect)
	is.Equal(st.Precision, 1.0) // help calls don't count against precision
}

func TestRedundantCallsComparesArguments(t *testing.T) {
	is := is.New(t)

	call := func(name, input string) ToolCall {
		return ToolCall{Name: name, Input: json.RawMessage(input)}
	}
	actions := NormalizeActions([]ToolCall{
		call("Write", `{"file_path":"player.gd"}`),
		call("Write", `{"file_path":"enemy.gd"}`),
		call(mcpPrefix+"add_node", `{"node_type":"Sprite2D"}`),
		call(mcpPrefix+"add_node", `{"node_type":"Sprite2D"}`),
		bashCall("ls -la"),
		bashCall("ls"),
	})

	is.Equal(ScoreActions(actions, &Spec{}).RedundantCalls, 1) // only the identical add_node pair
}

func TestScoreActionsMeasuresSelectionAndBypass(t *testing.T) {
	is := is.New(t)

	spec := &Spec{
		ExpectedActions:  []string{"open_scene", "add_node", "save_scene"},
		ForbiddenActions: []string{"execute_editor_script"},
	}
	actions := NormalizeActions([]ToolCall{
		{Name: mcpPrefix + "open_scene"},
		{Name: mcpPrefix + "add_node"},
		{Name: mcpPrefix + "execute_editor_script"},
		{Name: "Write"},
	})

	st := ScoreActions(actions, spec)

	is.Equal(st.FirstGodaiAction, "open_scene")
	is.True(st.FirstActionCorrect)
	is.Equal(st.Missing, []string{"save_scene"})
	is.Equal(st.Unexpected, []string{"execute_editor_script"})
	is.Equal(st.Forbidden, []string{"execute_editor_script"})
	is.Equal(st.Recall, 2.0/3.0)
	is.Equal(st.Precision, 2.0/3.0)
	is.Equal(st.BypassRate, 1.0/3.0) // one Write among three mutating actions; open_scene only reads
}

// The harness opens the editor before the agent starts, so an agent that makes
// sure of that first shouldn't score worse than one that assumes it.
func TestOpeningTheProjectIsNotAToolChoice(t *testing.T) {
	is := is.New(t)

	spec := &Spec{ExpectedActions: []string{"open_scene", "save_scene"}}
	st := ScoreActions(NormalizeActions([]ToolCall{
		{Name: mcpPrefix + "open_godot_project"},
		{Name: mcpPrefix + "open_scene"},
		{Name: mcpPrefix + "save_scene"},
	}), spec)

	is.Equal(st.FirstGodaiAction, "open_scene")
	is.True(st.FirstActionCorrect)
	is.Equal(st.Precision, 1.0)
	is.Equal(st.Recall, 1.0)
	is.Equal(len(st.Unexpected), 0)
	is.Equal(st.GodaiCalls, 3) // still a call, just not a choice being graded
}

// A task whose point is opening a project lists open_godot_project in
// expected_actions, which restores normal scoring.
func TestOpeningTheProjectCountsWhenATaskExpectsIt(t *testing.T) {
	is := is.New(t)

	spec := &Spec{ExpectedActions: []string{"open_godot_project", "set_project_settings"}}
	st := ScoreActions(NormalizeActions([]ToolCall{
		{Name: mcpPrefix + "open_godot_project"},
		{Name: mcpPrefix + "set_project_settings"},
	}), spec)

	is.Equal(st.FirstGodaiAction, "open_godot_project")
	is.True(st.FirstActionCorrect)
	is.Equal(st.Precision, 1.0)
	is.Equal(st.Recall, 1.0)
}
