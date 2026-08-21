package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/cli/schemaflag"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

func editorToolCommand(configPath string) *cli.Command {
	defs := core.RemoteToolDefinitions()

	names := make([]string, 0, len(defs))
	for name, def := range defs {
		if def.DoNotForward {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	commands := make([]*cli.Command, 0, len(names))
	for _, name := range names {
		command, err := editorToolSubcommand(configPath, name, defs[name])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s skipping tool %q: %v\n", output.Paint(useColor(), output.Yellow, "warning:"), name, err)
			continue
		}
		commands = append(commands, command)
	}

	return &cli.Command{
		Name:        "editor-tool",
		Aliases:     []string{"et"},
		Usage:       "run the tools the Godot editor provides",
		Description: "These run inside a Godot editor, so the project has to be open. Each tool is a subcommand with its own flags: run `godai editor-tool <tool> --help` to see what it takes.",
		Commands:    commands,
	}
}

// Flags come from a function because urfave/cli mutates the flag structs it's given.
type toolSugar struct {
	flags func() []cli.Flag
	// The positional arguments the sugar's collect step consumes itself, shown
	// on the USAGE line the same way cliPositionalArguments entries are.
	argsUsage string
	collect   func(cmd *cli.Command, args core.Args) error
}

var toolSugars = map[string]toolSugar{
	"set_node_properties": {
		flags: func() []cli.Flag {
			return []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "properties",
					Aliases: []string{"property"},
					Usage:   "set a property as NODE_PATH:PROPERTY=VALUE, or PROPERTY=VALUE with --node-path",
				},
				&cli.StringFlag{
					Name:    "node-path",
					Aliases: []string{"node_path"},
					Usage:   "node path prepended to each --properties key",
				},
			}
		},
		argsUsage: "[NODE_PATH]",
		collect:   collectNodePropertiesSugar,
	},
}

func collectNodePropertiesSugar(cmd *cli.Command, args core.Args) error {
	positional := cmd.Args().Slice()
	if len(positional) > 1 {
		return newUsageError("unexpected arguments: %v", positional[1:])
	}

	prefix := cmd.String("node-path")
	if len(positional) == 1 {
		if cmd.IsSet("node-path") {
			return newUsageError("node_path was given both as an argument and with --node-path")
		}
		prefix = positional[0]
	}

	entries := cmd.StringSlice("properties")
	if len(entries) == 0 {
		if prefix != "" {
			return newUsageError("a node path only prefixes --properties keys; add --properties PROPERTY=VALUE")
		}
		return nil
	}

	nodes := map[string]map[string]string{}
	if raw, ok := args["nodes"]; ok {
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return newUsageError("--nodes: %v", err)
		}
	}

	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return newUsageError("--properties expects KEY=VALUE, got %q", entry)
		}
		if prefix != "" {
			key = prefix + ":" + key
		}
		nodePath, property, ok := strings.Cut(key, ":")
		if !ok || nodePath == "" || property == "" {
			return newUsageError("--properties %q doesn't say which node: write NODE_PATH:%s=... or pass --node-path", entry, key)
		}
		if nodes[nodePath] == nil {
			nodes[nodePath] = map[string]string{}
		}
		nodes[nodePath][property] = value
	}

	raw, err := json.Marshal(nodes)
	if err != nil {
		return err
	}
	args.SetRaw("nodes", raw)
	return nil
}

func editorToolSubcommand(configPath, name string, def *core.ToolDefinition) (*cli.Command, error) {
	flags, specs, err := schemaflag.Build(def.GetInputSchema())
	if err != nil {
		return nil, err
	}

	positionals, argsUsage, err := checkPositionals(def.CLIPositionalArguments, specs)
	if err != nil {
		return nil, err
	}

	resPaths, err := checkResPaths(def.CLIResPathArguments, specs)
	if err != nil {
		return nil, err
	}

	if sugar, ok := toolSugars[name]; ok {
		flags = append(flags, sugar.flags()...)
		if argsUsage == "" {
			argsUsage = sugar.argsUsage
		}
	}
	flags = append(flags,
		projectPathFlag(),
		&cli.StringMapFlag{
			Name:  "arg",
			Usage: "set an argument as NAME=VALUE, for anything the generated flags don't cover",
		},
		&cli.StringFlag{
			Name:  "args-json",
			Usage: "a JSON object of arguments, merged first so flags override it",
		},
		&cli.StringMapFlag{
			Name:  "arg-file",
			Usage: "read an argument from a file, as NAME=PATH",
		},
		&cli.BoolFlag{
			Name:  "stdin-json",
			Usage: "read a JSON object of arguments from stdin",
		},
	)

	var aliases []string
	if kebab := strings.ReplaceAll(name, "_", "-"); kebab != name {
		aliases = []string{kebab}
	}

	category := ""
	if len(def.Toolsets) > 0 {
		category = strings.ToUpper(def.Toolsets[0][:1]) + def.Toolsets[0][1:]
	}

	return &cli.Command{
		Name:        name,
		Aliases:     aliases,
		Category:    category,
		Usage:       def.Title,
		ArgsUsage:   argsUsage,
		Description: def.GetDescription(),
		Flags:       flags,
		// Godot variant values are full of commas ("Vector2(1, 2)",
		// "Color(1, 0, 0, 1)"), so a repeated flag means one value rather
		// than a comma-separated list.
		DisableSliceFlagSeparator: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runEditorTool(ctx, cmd, configPath, name, specs, positionals, resPaths)
		},
	}, nil
}

// Resolves a tool's cliPositionalArguments list against its flag specs, returning them
// alongside the usage line ("NODE_PATH [SCRIPT_PATH]") they render as.
func checkPositionals(names []string, specs []schemaflag.Spec) ([]schemaflag.Spec, string, error) {
	positionals := make([]schemaflag.Spec, 0, len(names))
	usage := make([]string, 0, len(names))

	for i, name := range names {
		spec, found := findSpec(specs, name)
		if !found {
			return nil, "", fmt.Errorf("cliPositionalArguments names %q, which is not in the input schema", name)
		}

		switch spec.Kind {
		case schemaflag.KindString:
		case schemaflag.KindStringList:
			if i != len(names)-1 {
				return nil, "", fmt.Errorf("cliPositionalArguments lists %q, but a list property has to come last", name)
			}
		default:
			return nil, "", fmt.Errorf("cliPositionalArguments names %q, which is not a string or list of strings", name)
		}

		placeholder := strings.ToUpper(spec.Property)
		if spec.Kind == schemaflag.KindStringList {
			placeholder += "..."
		}
		if !spec.Required {
			placeholder = "[" + placeholder + "]"
		}

		positionals = append(positionals, spec)
		usage = append(usage, placeholder)
	}

	return positionals, strings.Join(usage, " "), nil
}

func findSpec(specs []schemaflag.Spec, property string) (schemaflag.Spec, bool) {
	for _, spec := range specs {
		if spec.Property == property {
			return spec, true
		}
	}
	return schemaflag.Spec{}, false
}

func collectPositionals(cmd *cli.Command, positionals []schemaflag.Spec, args core.Args) error {
	rest := cmd.Args().Slice()

	for i, spec := range positionals {
		if len(rest) == 0 {
			return nil
		}
		if _, taken := args[spec.Property]; taken {
			return newUsageError("%s was given both as an argument and with --%s", spec.Property, spec.Flag)
		}

		if spec.Kind == schemaflag.KindStringList && i == len(positionals)-1 {
			if err := args.Set(spec.Property, rest); err != nil {
				return err
			}
			return nil
		}

		if err := args.Set(spec.Property, rest[0]); err != nil {
			return err
		}
		rest = rest[1:]
	}

	if len(rest) > 0 {
		return newUsageError("unexpected arguments: %v", rest)
	}
	return nil
}

func runEditorTool(ctx context.Context, cmd *cli.Command, configPath, name string, specs, positionals, resPaths []schemaflag.Spec) error {
	sugar, hasSugar := toolSugars[name]
	takesArgs := len(positionals) > 0 || (hasSugar && sugar.argsUsage != "")
	if !takesArgs && cmd.Args().Present() {
		return newUsageError("unexpected arguments: %v; name the project with `--project-path <PATH>`", cmd.Args().Slice())
	}

	return withSession(ctx, cmd, configPath, func(session *core.Session) error {
		args, err := collectArgs(cmd, specs)
		if err != nil {
			return err
		}
		if hasSugar {
			if err := sugar.collect(cmd, args); err != nil {
				return err
			}
		}
		if len(positionals) > 0 {
			if err := collectPositionals(cmd, positionals, args); err != nil {
				return err
			}
		}
		if err := schemaflag.CheckRequired(specs, args); err != nil {
			return newUsageError("%v", err)
		}

		projectPath, err := core.ResolveProjectPath(cmd.String("project-path"))
		if err != nil {
			return err
		}

		if err := normalizeResPathArgs(args, resPaths, projectPath); err != nil {
			return err
		}

		if lifecycle, ok := editorLifecycleTools[name]; ok {
			return runEditorLifecycleTool(ctx, cmd, session, projectPath, args, lifecycle)
		}

		result, err := session.CallEditorTool(ctx, projectPath, name, args, core.CallOptions{
			Wait: connectWait(cmd),
		})
		if err != nil {
			return err
		}

		if result.IsError {
			return core.NewUserError(orDefault(result.ErrorMessage(), "the editor reported an error"), core.ErrToolFailed, nil)
		}

		return printToolResult(printer(cmd), name, result)
	})
}

func collectArgs(cmd *cli.Command, specs []schemaflag.Spec) (core.Args, error) {
	args := core.Args{}

	if cmd.Bool("stdin-json") {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		if err := mergeJSONArgs(args, string(raw)); err != nil {
			return nil, err
		}
	}

	if blob := cmd.String("args-json"); blob != "" {
		if err := mergeJSONArgs(args, blob); err != nil {
			return nil, err
		}
	}

	for name, path := range cmd.StringMap("arg-file") {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, newUsageError("--arg-file %s: %v", name, err)
		}
		if err := args.Set(name, string(content)); err != nil {
			return nil, err
		}
	}

	for name, value := range cmd.StringMap("arg") {
		if err := args.Set(name, value); err != nil {
			return nil, err
		}
	}

	fromFlags, err := schemaflag.Collect(cmd, specs)
	if err != nil {
		return nil, newUsageError("%v", err)
	}
	for name, raw := range fromFlags {
		args.SetRaw(name, raw)
	}

	return args, nil
}

func mergeJSONArgs(args core.Args, blob string) error {
	parsed, err := core.ParseArgs(json.RawMessage(blob))
	if err != nil {
		return newUsageError("%v", err)
	}
	for name, raw := range parsed {
		args.SetRaw(name, raw)
	}
	return nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
