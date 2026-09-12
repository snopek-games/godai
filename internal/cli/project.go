package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

func projectCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:  "project",
		Usage: "find, open and inspect Godot projects",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "list the Godot projects available to open",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := rejectArgs(cmd); err != nil {
						return err
					}

					return withSession(ctx, cmd, configPath, func(session *core.Session) error {
						projects, note, err := session.ListProjects()
						if err != nil {
							return err
						}

						out := printer(cmd)
						if err := printProjects(out, projects); err != nil {
							return err
						}
						if note != "" {
							out.Printf("\n")
							out.Note("%s", note)
						}
						return nil
					})
				},
			},
			{
				Name:      "open",
				Usage:     "open a Godot project in the editor",
				ArgsUsage: "[path]",
				Flags: []cli.Flag{
					projectPathFlag(),
					&cli.BoolFlag{
						Name:  "headless",
						Usage: "launch the editor without visual or audio output",
					},
					offscreenFlag("launch the editor rendering to a virtual display nobody can see (Linux only, needs Xvfb)"),
					offscreenSizeFlag(),
					&cli.BoolFlag{
						Name:  "auto-approve",
						Usage: "run tools in this editor without asking for approval",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := atMostOneArg(cmd, "project path"); err != nil {
						return err
					}
					if err := checkDisplayFlags(cmd); err != nil {
						return err
					}

					return withSession(ctx, cmd, configPath, func(session *core.Session) error {
						// The flag also reads GODAI_PROJECT_PATH, so an explicit
						// positional path has to win over it.
						hint := cmd.Args().First()
						if hint == "" {
							hint = cmd.String("project-path")
						}

						projectPath, err := core.ResolveProjectPath(hint)
						if err != nil {
							return err
						}

						result, err := session.OpenProject(ctx, projectPath, core.OpenProjectOptions{
							Headless:      cmd.Bool("headless"),
							Offscreen:     cmd.Bool("offscreen"),
							OffscreenSize: cmd.String("offscreen-size"),
							AutoApprove:   cmd.Bool("auto-approve"),
						})
						if err != nil {
							return err
						}

						out := printer(cmd)
						if err := out.Value(result, func(w io.Writer) error {
							if result.AlreadyOpen {
								_, err := fmt.Fprintf(w, "already open: %s\n", result.ProjectPath)
								return err
							}
							_, err := fmt.Fprintf(w, "opened: %s\n", result.ProjectPath)
							return err
						}); err != nil {
							return err
						}

						warnIgnoredOpenFlags(out, cmd, result)
						noteUnattendedEditors(out, session)
						return nil
					})
				},
			},
			{
				Name:      "install-addon",
				Usage:     "install and enable the godai addon in a project, without opening the editor",
				ArgsUsage: "[path]",
				Flags:     []cli.Flag{projectPathFlag()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := atMostOneArg(cmd, "project path"); err != nil {
						return err
					}

					return withSession(ctx, cmd, configPath, func(session *core.Session) error {
						hint := cmd.Args().First()
						if hint == "" {
							hint = cmd.String("project-path")
						}

						projectPath, err := core.ResolveProjectPath(hint)
						if err != nil {
							return err
						}

						installedPath, err := session.InstallAddon(projectPath)
						if err != nil {
							return err
						}

						result := struct {
							ProjectPath string `json:"project_path"`
						}{installedPath}
						return printer(cmd).Value(result, func(w io.Writer) error {
							_, err := fmt.Fprintf(w, "addon installed: %s\n", installedPath)
							return err
						})
					})
				},
			},
			{
				Name:        "pin-engine",
				Usage:       "record which version of Godot this project is built with",
				ArgsUsage:   "<version>",
				Description: "The version is written to " + core.ProjectConfigName + " in the project, which is meant to be committed, so that everyone working on it opens the same editor.",
				Flags:       []cli.Flag{projectPathFlag()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name, err := oneArg(cmd, "version")
					if err != nil {
						return err
					}

					return withSession(ctx, cmd, configPath, func(session *core.Session) error {
						projectPath, err := core.ResolveProjectPath(cmd.String("project-path"))
						if err != nil {
							return err
						}

						engine, err := session.FindEngine(name)
						if err != nil {
							return err
						}

						if err := core.SetProjectGodotVersion(projectPath, engine.Name); err != nil {
							return err
						}

						printer(cmd).Printf("%s now uses Godot %s\n", core.ProjectConfigPath(projectPath), engine.Name)
						return nil
					})
				},
			},
			{
				Name:  "unpin-engine",
				Usage: "stop recording which version of Godot this project is built with",
				Flags: []cli.Flag{projectPathFlag()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := rejectArgs(cmd); err != nil {
						return err
					}

					projectPath, err := core.ResolveProjectPath(cmd.String("project-path"))
					if err != nil {
						return err
					}

					pinned, err := core.UnsetProjectGodotVersion(projectPath)
					if err != nil {
						return err
					}

					out := printer(cmd)
					if !pinned {
						out.Printf("%s isn't pinned to a version of Godot\n", projectPath)
						return nil
					}

					out.Printf("%s is no longer pinned to a version of Godot\n", projectPath)
					return nil
				},
			},
		},
	}
}

// These only apply to an editor we launch ourselves, so an editor that was
// already running keeps whatever it started with.
func warnIgnoredOpenFlags(out *Printer, cmd *cli.Command, result *core.OpenProjectResult) {
	if !result.AlreadyOpen {
		return
	}

	ignored := []string{}
	if cmd.Bool("headless") && !result.Headless {
		ignored = append(ignored, "--headless")
	}
	if cmd.Bool("offscreen") && !result.Offscreen {
		ignored = append(ignored, "--offscreen")
	}
	if cmd.IsSet("offscreen-size") {
		ignored = append(ignored, "--offscreen-size")
	}
	if cmd.Bool("auto-approve") {
		ignored = append(ignored, "--auto-approve")
	}
	if len(ignored) == 0 {
		return
	}

	out.Warn("that editor was already running, so %s had no effect; close it with `godai editor close %s` and open it again",
		strings.Join(ignored, " and "), result.ProjectPath)
}

func printProjects(out *Printer, projects []core.ProjectInfo) error {
	return out.Value(struct {
		Projects []core.ProjectInfo `json:"projects"`
	}{projects}, func(io.Writer) error {
		rows := make([][]string, 0, len(projects))
		for _, p := range projects {
			rows = append(rows, []string{p.ProjectName, p.ProjectPath})
		}
		return out.Table([]string{"NAME", "PATH"}, rows)
	})
}

func orUnknown(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func noteUnattendedEditors(out *Printer, session *core.Session) {
	for _, editor := range session.UnattendedEditors() {
		out.Note("launched an %s editor for %s; close it with `godai editor close %s`", editor.Display, editor.ProjectPath, editor.ProjectPath)
	}
}

func offscreenFlag(usage string) cli.Flag {
	return &cli.BoolFlag{Name: "offscreen", Usage: usage}
}

func offscreenSizeFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "offscreen-size",
		Usage: "the virtual display's size in pixels, as WIDTHxHEIGHT, for --offscreen",
		Value: core.DefaultOffscreenSize,
	}
}

func checkDisplayFlags(cmd *cli.Command) error {
	if cmd.Bool("headless") && cmd.Bool("offscreen") {
		return newUsageError("--headless and --offscreen are mutually exclusive")
	}
	if cmd.IsSet("offscreen-size") && !cmd.Bool("offscreen") {
		return newUsageError("--offscreen-size only applies with --offscreen")
	}
	if cmd.Bool("offscreen") {
		if err := core.CheckOffscreenSize(cmd.String("offscreen-size")); err != nil {
			return newUsageError("%s", err.Error())
		}
	}
	return nil
}
