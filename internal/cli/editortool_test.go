package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
	"github.com/urfave/cli/v3"
)

func TestEditorToolCommandBuildsEveryTool(t *testing.T) {
	is := is.New(t)

	toolCommand := editorToolCommand("")

	forwardable := 0
	for _, def := range core.RemoteToolDefinitions() {
		if !def.DoNotForward {
			forwardable++
		}
	}

	is.True(forwardable > 0)
	is.Equal(len(toolCommand.Commands), forwardable)
}

func TestToolCommandsDisableSliceFlagSeparator(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		if !command.DisableSliceFlagSeparator {
			t.Errorf("tool command %q would split repeated flag values on commas", command.Name)
		}
	}
}

func TestToolCommandsHaveProjectPathFlag(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		if !hasFlag(command, "project-path") {
			t.Errorf("tool command %q is missing --project-path", command.Name)
		}
	}
}

func TestToolCommandsHaveEscapeHatches(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		for _, name := range []string{"arg", "args-json", "arg-file", "stdin-json"} {
			if !hasFlag(command, name) {
				t.Errorf("tool command %q is missing --%s", command.Name, name)
			}
		}
	}
}

func TestEditorToolCommandRoutesLifecycleTools(t *testing.T) {
	is := is.New(t)

	is.True(len(editorLifecycleTools) > 0)

	toolCommand := editorToolCommand("")
	for name := range editorLifecycleTools {
		def, ok := core.RemoteToolDefinitions()[name]
		is.True(ok)
		is.True(!def.DoNotForward)

		is.True(toolCommand.Command(name) != nil)
	}
}

func TestEditorToolCommandSkipsNonForwardedTools(t *testing.T) {
	is := is.New(t)

	toolCommand := editorToolCommand("")
	for name, def := range core.RemoteToolDefinitions() {
		if def.DoNotForward {
			is.True(toolCommand.Command(name) == nil)
		}
	}
}

func TestRequiredToolArgumentsAcceptTheEscapeHatches(t *testing.T) {
	is := is.New(t)

	base := []string{"godai", "editor-tool", "set_project_settings", "-p", filepath.Join(t.TempDir(), "nope")}

	err := runQuietly(t, base)
	is.Equal(ExitCodeFor(err), ExitUsage)
	is.True(strings.Contains(err.Error(), "settings"))

	for _, args := range [][]string{
		{"--settings", "application/config/name=\"demo\""},
		{"--args-json", `{"settings":{"application/config/name":"\"demo\""}}`},
		{"--arg", "settings={}"},
	} {
		if code := ExitCodeFor(runQuietly(t, append(base, args...))); code == ExitUsage {
			t.Errorf("%v: rejected as a usage error", args)
		}
	}
}

func TestToolCommandsHaveKebabCaseAliases(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		if !strings.Contains(command.Name, "_") {
			continue
		}
		if !slices.Contains(command.Aliases, strings.ReplaceAll(command.Name, "_", "-")) {
			t.Errorf("tool command %q has no kebab-case alias", command.Name)
		}
	}
}

func TestToolCommandsHaveACategory(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		if command.Category == "" {
			t.Errorf("tool command %q has no category", command.Name)
		}
	}
}

func TestPositionalArguments(t *testing.T) {
	is := is.New(t)

	base := []string{"godai", "editor-tool"}
	project := []string{"-p", filepath.Join(t.TempDir(), "nope")}

	// Anything that parses cleanly fails later than the usage check, since the
	// project path doesn't exist.
	for _, args := range [][]string{
		{"read_script", "res://spin.gd"},
		{"add_node", "MeshInstance3D", "--property", "name=Sphere"},
		{"attach_script", "Sphere", "res://spin.gd"},
		{"connect_signal", "Player", "pressed", "--to-node", ".", "--method", "_on_pressed"},
		{"get_node_properties", "Sphere", "Sphere:mesh", "Sphere:material_override"},
		{"get_project_settings", "application/config/name", "display/window/size/mode"},
		{"run_project"},
		{"set_node_properties", "Sphere", "--action", "Grow", "--property", "speed=1.0"},
	} {
		full := append(append(append([]string{}, base...), args...), project...)
		if code := ExitCodeFor(runQuietly(t, full)); code == ExitUsage {
			t.Errorf("%v: rejected as a usage error", args)
		}
	}

	err := runQuietly(t, append(append([]string{}, base...), "read_script", "res://a.gd", "--file-path", "res://b.gd"))
	is.Equal(ExitCodeFor(err), ExitUsage)
	is.True(strings.Contains(err.Error(), "both as an argument and with --file-path"))

	err = runQuietly(t, append(append([]string{}, base...), "read_script", "res://a.gd", "res://b.gd"))
	is.Equal(ExitCodeFor(err), ExitUsage)
	is.True(strings.Contains(err.Error(), "unexpected arguments"))

	// save_scene has no cliPositionalArguments, so positionals stay rejected.
	err = runQuietly(t, append(append([]string{}, base...), "save_scene", "res://demo.tscn"))
	is.Equal(ExitCodeFor(err), ExitUsage)
	is.True(strings.Contains(err.Error(), "unexpected arguments"))
}

func TestPositionalArgumentsShowInUsageLines(t *testing.T) {
	is := is.New(t)

	toolCommand := editorToolCommand("")

	for name, usage := range map[string]string{
		"read_script":          "FILE_PATH",
		"attach_script":        "NODE_PATH SCRIPT_PATH",
		"connect_signal":       "FROM_NODE SIGNAL",
		"get_node_properties":  "NODE_PATHS...",
		"get_editor_settings":  "[NAMES...]",
		"get_project_settings": "[NAMES...]",
		"run_project":          "[SCENE]",
		"save_scene":           "",
		"set_node_properties":  "[NODE_PATH]",
	} {
		is.Equal(toolCommand.Command(name).ArgsUsage, usage)
	}
}

func TestSetNodePropertiesSugarBuildsTheNodesArgument(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		nodes string // "" means the sugar left args["nodes"] alone
		err   string
	}{
		{
			name:  "inline node paths",
			args:  []string{"--properties", "Sphere:speed=0.25", "--properties", "Enemy:mesh:radius=2.0"},
			nodes: `{"Enemy":{"mesh:radius":"2.0"},"Sphere":{"speed":"0.25"}}`,
		},
		{
			name:  "node-path prefixes every key",
			args:  []string{"--node-path", "Sphere", "--properties", "mesh:radius=1.0", "--properties", "mesh:height=2.0"},
			nodes: `{"Sphere":{"mesh:height":"2.0","mesh:radius":"1.0"}}`,
		},
		{
			name:  "scene root via node-path",
			args:  []string{"--node-path", ".", "--properties", "speed=0.25"},
			nodes: `{".":{"speed":"0.25"}}`,
		},
		{
			name:  "value keeps everything after the first equals",
			args:  []string{"--properties", `Sphere:material_override=Object(StandardMaterial3D,"albedo_color":Color(0, 1, 0, 1))`},
			nodes: `{"Sphere":{"material_override":"Object(StandardMaterial3D,\"albedo_color\":Color(0, 1, 0, 1))"}}`,
		},
		{
			name: "no properties leaves args alone",
			args: nil,
		},
		{
			name: "missing value",
			args: []string{"--properties", "Sphere:speed"},
			err:  "expects KEY=VALUE",
		},
		{
			name: "missing node path",
			args: []string{"--properties", "speed=0.25"},
			err:  "doesn't say which node",
		},
		{
			name: "node-path without properties",
			args: []string{"--node-path", "Sphere"},
			err:  "add --properties",
		},
		{
			name:  "positional node path",
			args:  []string{"Sphere", "--properties", "mesh:radius=1.0"},
			nodes: `{"Sphere":{"mesh:radius":"1.0"}}`,
		},
		{
			name: "positional and flag node paths conflict",
			args: []string{"Sphere", "--node-path", "Enemy", "--properties", "speed=1.0"},
			err:  "both as an argument and with --node-path",
		},
		{
			name: "too many positionals",
			args: []string{"Sphere", "Enemy", "--properties", "speed=1.0"},
			err:  "unexpected arguments",
		},
		{
			name: "positional node path without properties",
			args: []string{"Sphere"},
			err:  "add --properties",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			args := core.Args{}
			var collectErr error
			command := &cli.Command{
				Flags:                     toolSugars["set_node_properties"].flags(),
				DisableSliceFlagSeparator: true,
				Action: func(_ context.Context, cmd *cli.Command) error {
					collectErr = collectNodePropertiesSugar(cmd, args)
					return nil
				},
			}
			is.NoErr(command.Run(context.Background(), append([]string{"set_node_properties"}, tc.args...)))

			if tc.err != "" {
				is.True(collectErr != nil)
				is.True(strings.Contains(collectErr.Error(), tc.err))
				return
			}
			is.NoErr(collectErr)

			if tc.nodes == "" {
				_, ok := args["nodes"]
				is.True(!ok)
				return
			}
			is.Equal(string(args["nodes"]), tc.nodes)
		})
	}
}

func TestSetNodePropertiesSugarMergesIntoNodes(t *testing.T) {
	is := is.New(t)

	args := core.Args{}
	args.SetRaw("nodes", json.RawMessage(`{"Sphere": {"speed": "1.0"}, "Enemy": {"speed": "2.0"}}`))

	command := &cli.Command{
		Flags:                     toolSugars["set_node_properties"].flags(),
		DisableSliceFlagSeparator: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			return collectNodePropertiesSugar(cmd, args)
		},
	}
	is.NoErr(command.Run(context.Background(), []string{"set_node_properties",
		"--properties", "Sphere:speed=0.25",
		"--properties", "Sphere:visible=true"}))

	is.Equal(string(args["nodes"]), `{"Enemy":{"speed":"2.0"},"Sphere":{"speed":"0.25","visible":"true"}}`)
}

func hasFlag(command *cli.Command, name string) bool {
	for _, flag := range command.Flags {
		for _, candidate := range flag.Names() {
			if candidate == name {
				return true
			}
		}
	}
	return false
}

func TestRootCommandSet(t *testing.T) {
	is := is.New(t)

	root := Root()
	for _, name := range []string{"mcp", "project", "config", "editor-tool", "editor", "self-update"} {
		if root.Command(name) == nil {
			t.Errorf("root is missing the %q command", name)
		}
	}

	project := root.Command("project")
	for _, name := range []string{"list", "open"} {
		if project.Command(name) == nil {
			t.Errorf("`godai project` is missing the %q command", name)
		}
	}

	editor := root.Command("editor")
	for _, name := range []string{"list", "restart", "close"} {
		if editor.Command(name) == nil {
			t.Errorf("`godai editor` is missing the %q command", name)
		}
	}

	config := root.Command("config")
	if config.Command("init") == nil {
		t.Error("`godai config` is missing the \"init\" command")
	}
	for _, name := range []string{"set", "unset"} {
		if !hasFlag(config, name) {
			t.Errorf("`godai config` is missing --%s", name)
		}
	}

	is.True(strings.Contains(root.Usage, "Godot"))
}
