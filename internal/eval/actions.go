package eval

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/mcp"
)

// Editor tools are named identically on both surfaces, so only the local
// commands below need mapping for CLI and MCP runs to be comparable.

const (
	ActionKindGodai   = "godai"
	ActionKindBuiltin = "builtin" // Read/Edit/Write/Glob: went around godai
	ActionKindShell   = "shell"

	// Reading help isn't a tool choice: MCP hands over for free what the CLI
	// surface has to go and read.
	ActionKindHelp = "help"

	mcpPrefix = "mcp__godai__"

	openProjectAction = "open_godot_project"
)

type Action struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Raw     string `json:"raw"`
	Input   string `json:"input,omitempty"`
	Turn    int    `json:"turn"`
	IsError bool   `json:"is_error"`
}

// Maps `godai <cmd> <sub>` onto the MCP tool that does the same thing.
var localCommandActions = map[string]string{
	"project list":         "list_projects",
	"project open":         "open_godot_project",
	"project pin-engine":   "pin_project_to_godot_version",
	"project unpin-engine": "unpin_project_from_godot_version",
	"editor list":          "list_open_projects",
	"engine list":          "list_installed_godot_versions",
	"engine search":        "search_available_godot_versions",
	"engine install":       "install_godot_version",
	"engine remove":        "remove_godot_version",
	"config":               "get_godai_settings",
}

// Every godai tool, mapped to whether it only reads.
var godaiTools = sync.OnceValue(func() map[string]bool {
	readOnly := map[string]bool{}
	for _, defs := range []map[string]*core.ToolDefinition{
		core.RemoteToolDefinitions(), mcp.GetLocalToolDefinitions(),
	} {
		for name, def := range defs {
			readOnly[name], _ = def.Annotations["readOnlyHint"].(bool)
		}
	}
	return readOnly
})

func isGodaiTool(name string) bool {
	_, ok := godaiTools()[name]
	return ok
}

func mutatingGodaiAction(name string) bool {
	readOnly, known := godaiTools()[name]
	return known && !readOnly
}

// mutatingBuiltins mean the agent edited the project directly instead of going
// through godai, which is worth measuring rather than forbidding.
var mutatingBuiltins = map[string]bool{
	"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true,
}

func NormalizeActions(calls []ToolCall) []Action {
	out := make([]Action, 0, len(calls))
	for _, c := range calls {
		a := Action{Turn: c.Turn, IsError: c.IsError, Input: string(c.Input)}
		switch {
		case strings.HasPrefix(c.Name, mcpPrefix):
			a.Kind, a.Name, a.Raw = ActionKindGodai, strings.TrimPrefix(c.Name, mcpPrefix), c.Name

		case c.Name == "Bash":
			cmd := bashCommand(c.Input)
			a.Raw = cmd
			switch name, ok := godaiAction(cmd); {
			case ok && asksForHelp(cmd):
				a.Kind, a.Name = ActionKindHelp, name
			case ok:
				a.Kind, a.Name = ActionKindGodai, name
			default:
				a.Kind, a.Name = ActionKindShell, firstWord(cmd)
			}

		case isGodaiTool(c.Name):
			a.Kind, a.Name, a.Raw = ActionKindGodai, c.Name, c.Name

		default:
			a.Kind, a.Name, a.Raw = ActionKindBuiltin, c.Name, c.Name
		}
		out = append(out, a)
	}
	return out
}

func bashCommand(input json.RawMessage) string {
	var v struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(input, &v)
	return strings.TrimSpace(v.Command)
}

// godaiAction pulls the action out of a shell command such as
// `cd /w && godai editor-tool add_node --node-type Timer`.
func godaiAction(cmd string) (string, bool) {
	for _, seg := range splitShell(cmd) {
		fields := skipAssignments(strings.Fields(seg))
		if len(fields) == 0 {
			continue
		}
		if f := fields[0]; f == "godai" || strings.HasSuffix(f, "/godai") {
			return localAction(commandWords(fields[1:])), true
		}
	}
	return "", false
}

// A command can be prefixed with environment assignments, which the program
// name comes after.
func skipAssignments(fields []string) []string {
	for i, f := range fields {
		name, _, ok := strings.Cut(f, "=")
		if !ok || name == "" || strings.ContainsAny(name, "-/.") {
			return fields[i:]
		}
	}
	return nil
}

func asksForHelp(cmd string) bool {
	for f := range strings.FieldsSeq(cmd) {
		if f == "--help" || f == "-h" || f == "help" {
			return true
		}
	}
	return false
}

// godaiCommands anchors the search: global flags come before the command, and
// which of them take a value isn't knowable from the command line alone.
var godaiCommands = map[string]bool{
	"mcp": true, "project": true, "config": true,
	"editor-tool": true, "editor": true, "engine": true, "self-update": true,
}

func commandWords(fields []string) []string {
	for i, f := range fields {
		if !godaiCommands[f] {
			continue
		}
		if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
			return []string{f, fields[i+1]}
		}
		return []string{f}
	}
	return nil
}

func localAction(words []string) string {
	if len(words) == 0 {
		return "godai"
	}
	if words[0] == "editor-tool" {
		if len(words) > 1 {
			return words[1]
		}
		return "editor-tool"
	}
	joined := strings.Join(words, " ")
	if name, ok := localCommandActions[joined]; ok {
		return name
	}
	if name, ok := localCommandActions[words[0]]; ok {
		return name
	}
	return strings.ReplaceAll(joined, " ", "_")
}

func splitShell(cmd string) []string {
	r := strings.NewReplacer("&&", "\x00", "||", "\x00", ";", "\x00", "|", "\x00")
	return strings.Split(r.Replace(cmd), "\x00")
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// ActionStats scores tool selection independently of task success: it moves
// well before the pass rate, so tune tool names and descriptions against it.
type ActionStats struct {
	TotalCalls   int `json:"total_calls"`
	GodaiCalls   int `json:"godai_calls"`
	HelpCalls    int `json:"help_calls"`
	ShellCalls   int `json:"shell_calls"`
	BuiltinCalls int `json:"builtin_calls"`
	ErrorCalls   int `json:"error_calls"`

	// The first choice reads most cleanly on tool-description quality: later
	// calls are contaminated by what the earlier ones returned.
	FirstGodaiAction   string `json:"first_godai_action"`
	FirstActionCorrect bool   `json:"first_action_correct"`

	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`

	// RedundantCalls counts calls repeated with identical arguments.
	RedundantCalls int `json:"redundant_calls"`

	// BypassRate is meaningless when MutatingCalls is 0; the summary skips it then.
	MutatingCalls int     `json:"mutating_calls"`
	BypassRate    float64 `json:"bypass_rate"`
	ErrorRate     float64 `json:"error_rate"`

	Forbidden  []string `json:"forbidden_used,omitempty"`
	Unexpected []string `json:"unexpected_actions,omitempty"`
	Missing    []string `json:"missing_actions,omitempty"`
}

func ScoreActions(actions []Action, spec *Spec) ActionStats {
	st := ActionStats{TotalCalls: len(actions)}

	expected := toSet(spec.ExpectedActions)
	forbidden := toSet(spec.ForbiddenActions)

	seen := map[string]int{}
	used := map[string]bool{}
	bypassed := 0

	for _, a := range actions {
		if a.IsError {
			st.ErrorCalls++
		}
		switch a.Kind {
		case ActionKindGodai:
			st.GodaiCalls++
			if mutatingGodaiAction(a.Name) {
				st.MutatingCalls++
			}
			if forbidden[a.Name] {
				st.Forbidden = append(st.Forbidden, a.Name)
			}
			// Opening the project before starting is normal, and doesn't represent
			// a tool choice for the first action or precision. A task where opening
			// a project is the point lists it in expected_actions, which reverts to
			// normal scoring.
			if a.Name == openProjectAction && !expected[openProjectAction] {
				break
			}
			used[a.Name] = true
			if st.FirstGodaiAction == "" {
				st.FirstGodaiAction = a.Name
				st.FirstActionCorrect = expected[a.Name]
			}
		case ActionKindHelp:
			st.HelpCalls++
		case ActionKindShell:
			st.ShellCalls++
		default:
			st.BuiltinCalls++
			if mutatingBuiltins[a.Name] {
				st.MutatingCalls++
				bypassed++
			}
		}

		key := a.Kind + "|" + a.Name + "|" + a.Input
		seen[key]++
		if seen[key] > 1 {
			st.RedundantCalls++
		}
	}

	if len(expected) > 0 {
		hits := 0
		for name := range used {
			if expected[name] {
				hits++
			} else {
				st.Unexpected = append(st.Unexpected, name)
			}
		}
		for name := range expected {
			if !used[name] {
				st.Missing = append(st.Missing, name)
			}
		}
		if len(used) > 0 {
			st.Precision = float64(hits) / float64(len(used))
		}
		st.Recall = float64(hits) / float64(len(expected))
	}

	if st.MutatingCalls > 0 {
		st.BypassRate = float64(bypassed) / float64(st.MutatingCalls)
	}
	if st.TotalCalls > 0 {
		st.ErrorRate = float64(st.ErrorCalls) / float64(st.TotalCalls)
	}

	sort.Strings(st.Unexpected)
	sort.Strings(st.Missing)
	return st
}
