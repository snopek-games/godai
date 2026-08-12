package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

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
			fmt.Fprintf(os.Stderr, "warning: skipping tool %q: %v\n", name, err)
			continue
		}
		commands = append(commands, command)
	}

	return &cli.Command{
		Name:        "editor-tool",
		Usage:       "run the tools the Godot editor provides",
		Description: "These run inside a Godot editor, so the project has to be open. Each tool is a subcommand with its own flags: run `godai editor-tool <tool> --help` to see what it takes.",
		Commands:    commands,
	}
}

func editorToolSubcommand(configPath, name string, def *core.ToolDefinition) (*cli.Command, error) {
	flags, specs, err := schemaflag.Build(def.GetInputSchema())
	if err != nil {
		return nil, err
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

	return &cli.Command{
		Name:        name,
		Usage:       def.Title,
		Description: def.GetDescription(),
		Flags:       flags,
		// Godot variant values are full of commas ("Vector2(1, 2)",
		// "Color(1, 0, 0, 1)"), so a repeated flag means one value rather
		// than a comma-separated list.
		DisableSliceFlagSeparator: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runEditorTool(ctx, cmd, configPath, name, specs)
		},
	}, nil
}

func runEditorTool(ctx context.Context, cmd *cli.Command, configPath, name string, specs []schemaflag.Spec) error {
	if cmd.Args().Present() {
		return newUsageError("unexpected arguments: %v; name the project with `--project-path <PATH>`", cmd.Args().Slice())
	}

	return withSession(ctx, cmd, configPath, func(session *core.Session) error {
		args, err := collectArgs(cmd, specs)
		if err != nil {
			return err
		}
		if err := schemaflag.CheckRequired(specs, args); err != nil {
			return newUsageError("%v", err)
		}

		projectPath, err := core.ResolveProjectPath(cmd.String("project-path"))
		if err != nil {
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

		return printToolResult(printer(cmd), result)
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

func printToolResult(out *Printer, result *core.ToolResult) error {
	if out.JSON {
		// The editor's result goes through untouched.
		_, err := out.Out.Write(append([]byte(result.Raw), '\n'))
		return err
	}

	if result.Unparsed {
		out.Printf("%s\n", result.Raw)
		return nil
	}

	if len(result.StructuredContent) > 0 {
		pretty := &bytes.Buffer{}
		if err := json.Indent(pretty, result.StructuredContent, "", "  "); err == nil {
			out.Printf("%s\n", pretty.String())
			return nil
		}
	}

	if text := result.Text(); text != "" {
		out.Printf("%s\n", text)
	}
	return nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
