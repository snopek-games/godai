package cli

import (
	"context"
	"io"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

func projectPathFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "project-path",
		Aliases: []string{"p"},
		Usage:   "the Godot project to act on (defaults to the one you're in)",
		Sources: cli.EnvVars("GODAI_PROJECT_PATH"),
	}
}

type editorLifecycle struct {
	verb string
	run  func(*core.Session, context.Context, string, core.Args) error
}

// These editor tools will break the connection to the editor, so they need to
// be run in a special way.
var editorLifecycleTools = map[string]editorLifecycle{
	"close_editor":   {"closed", (*core.Session).CloseEditor},
	"restart_editor": {"restarted", (*core.Session).RestartEditor},
}

func editorCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:  "editor",
		Usage: "control a running Godot editor",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "list the projects currently open in a Godot editor",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := rejectArgs(cmd); err != nil {
						return err
					}

					return withSession(ctx, cmd, configPath, func(session *core.Session) error {
						waitCtx, cancel := context.WithTimeout(ctx, listOpenWait(cmd))
						defer cancel()

						_, _ = session.WaitForAnyEditor(waitCtx)

						projects, err := session.ListOpenProjects(ctx)
						if err != nil {
							return err
						}
						return printProjects(printer(cmd), projects)
					})
				},
			},
			{
				Name:      "restart",
				Usage:     "restart the Godot editor and wait for it to come back",
				ArgsUsage: "[path]",
				Flags:     []cli.Flag{projectPathFlag()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runEditorAction(ctx, cmd, configPath, editorLifecycleTools["restart_editor"], core.Args{})
				},
			},
			{
				Name:      "close",
				Usage:     "close the Godot editor and wait for it to shut down",
				ArgsUsage: "[path]",
				Flags: []cli.Flag{
					projectPathFlag(),
					&cli.BoolFlag{
						Name:  "skip-save",
						Usage: "close without saving unsaved changes",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := core.Args{}
					if cmd.Bool("skip-save") {
						if err := args.Set("skip_save", true); err != nil {
							return err
						}
					}
					return runEditorAction(ctx, cmd, configPath, editorLifecycleTools["close_editor"], args)
				},
			},
		},
	}
}

func runEditorAction(ctx context.Context, cmd *cli.Command, configPath string, tool editorLifecycle, args core.Args) error {
	if err := atMostOneArg(cmd, "project path"); err != nil {
		return err
	}

	return withSession(ctx, cmd, configPath, func(session *core.Session) error {
		// The flag also reads GODAI_PROJECT_PATH, so an explicit positional
		// path has to win over it.
		hint := cmd.Args().First()
		if hint == "" {
			hint = cmd.String("project-path")
		}

		projectPath, err := core.ResolveProjectPath(hint)
		if err != nil {
			return err
		}

		return runEditorLifecycleTool(ctx, cmd, session, projectPath, args, tool)
	})
}

func runEditorLifecycleTool(ctx context.Context, cmd *cli.Command, session *core.Session, projectPath string, args core.Args, tool editorLifecycle) error {
	waitCtx, cancel := context.WithTimeout(ctx, connectWait(cmd))
	defer cancel()
	if _, err := session.WaitForEditor(waitCtx, projectPath); err != nil {
		return err
	}

	if err := tool.run(session, ctx, projectPath, args); err != nil {
		return err
	}

	out := printer(cmd)
	return out.Value(map[string]any{"project_path": projectPath, "success": true}, func(w io.Writer) error {
		out.Printf("%s: %s\n", tool.verb, projectPath)
		return nil
	})
}
