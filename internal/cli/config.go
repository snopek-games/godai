package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

func configCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "config",
		Usage:     "read and change Godai's own settings",
		ArgsUsage: "[setting...]",
		Description: "These are Godai's settings, not anything inside a Godot project or the editor. They're stored in " + configPathOrDefault(configPath) + ".\n\n" +
			"The settings are " + strings.Join(core.SettingNames, " and ") + ", named the same way here, in the config file and in the MCP tools.\n\n" +
			"With nothing else to do, every setting is printed. Naming settings prints just their values, one per line, which is what you want in a script.",
		// A path can contain a comma, so a repeated flag means one setting
		// rather than a comma-separated list.
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringMapFlag{
				Name:  "set",
				Usage: "change a setting, as NAME=VALUE",
			},
			&cli.StringSliceFlag{
				Name:  "unset",
				Usage: "clear a setting, so Godai works it out again on its own",
			},
		},
		ShellComplete: func(_ context.Context, cmd *cli.Command) {
			for _, name := range core.SettingNames {
				fmt.Fprintln(cmd.Root().Writer, name)
			}
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				return runConfig(cmd, session)
			})
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "set up Godai interactively",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := rejectArgs(cmd); err != nil {
						return err
					}

					if cmd.Bool("no-input") || !isInteractive() {
						return newUsageError("`godai config init` needs a terminal; use `godai engine use <VERSION>` instead")
					}

					return withSession(ctx, cmd, configPath, func(session *core.Session) error {
						return runConfigInit(ctx, cmd, session)
					})
				},
			},
		},
	}
}

func runConfig(cmd *cli.Command, session *core.Session) error {
	requested := cmd.Args().Slice()
	updates := cmd.StringMap("set")
	cleared := cmd.StringSlice("unset")

	if len(requested) > 0 && (len(updates) > 0 || len(cleared) > 0) {
		return newUsageError("name settings to read them, or use --set/--unset to change them, but not both at once")
	}

	if len(updates) > 0 {
		update := core.SavedConfig{}
		for _, name := range slices.Sorted(maps.Keys(updates)) {
			if err := core.CheckSettingName(name); err != nil {
				return newUsageError("%v", err)
			}
			if updates[name] == "" {
				return newUsageError("no value for %s; use `--unset %s` to clear it", name, name)
			}
			if err := update.SetSetting(name, updates[name]); err != nil {
				return newUsageError("%v", err)
			}
		}
		if err := session.SetConfig(update); err != nil {
			return err
		}
	}

	if len(cleared) > 0 {
		for _, name := range cleared {
			if err := core.CheckSettingName(name); err != nil {
				return newUsageError("%v", err)
			}
		}
		if err := session.UnsetConfig(cleared); err != nil {
			return err
		}
	}

	if len(requested) > 0 {
		return printSettings(printer(cmd), session.GetConfig(), requested)
	}
	return printConfig(printer(cmd), session.GetConfig())
}

func runConfigInit(ctx context.Context, cmd *cli.Command, session *core.Session) error {
	current := session.GetConfig()
	prompter := &ttyPrompter{}

	versions, err := installedEngineNames(session)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return core.NewUserError("no version of Godot is installed", core.ErrNotConfigured, []string{
			"See what there is: `godai engine search`",
			"Install one: `godai engine install <VERSION>`",
		})
	}

	answers, err := prompter.Prompt(ctx, "Let's set up Godai!", map[string]any{
		"type":     "object",
		"required": []any{core.SettingGodotVersion},
		"properties": map[string]any{
			core.SettingGodotVersion: map[string]any{
				"type":        "string",
				"description": "Which version of Godot to use by default",
				"enum":        versions,
				"default":     orFirst(current.GodotVersion, versions),
			},
			core.SettingProjectBasePath: map[string]any{
				"type":        "string",
				"description": "The base path where your Godot projects usually live",
				"default":     current.ProjectBasePath,
			},
		},
	})
	if err != nil {
		return err
	}

	update := core.SavedConfig{}
	if v, ok := answers[core.SettingGodotVersion].(string); ok {
		update.GodotVersion = v
	}
	if v, ok := answers[core.SettingProjectBasePath].(string); ok {
		update.ProjectBasePath = v
	}

	if err := session.SetConfig(update); err != nil {
		return err
	}

	out := printer(cmd)
	out.Printf("\nSaved to %s\n\n", out.Paint(output.Cyan, configPathOrDefault(session.Config().SavedConfigPath)))
	return printConfig(out, session.GetConfig())
}

func printConfig(out *Printer, sc core.SavedConfig) error {
	return out.Value(sc, func(w io.Writer) error {
		rows := make([][]string, 0, len(core.SettingNames))
		for _, name := range core.SettingNames {
			value, err := sc.Setting(name)
			if err != nil {
				return err
			}
			rows = append(rows, []string{name, orUnset(value)})
		}
		return out.Table(nil, rows)
	})
}

func printSettings(out *Printer, sc core.SavedConfig, names []string) error {
	values := map[string]string{}
	ordered := make([]string, 0, len(names))
	for _, name := range names {
		value, err := sc.Setting(name)
		if err != nil {
			return newUsageError("%v", err)
		}
		values[name] = value
		ordered = append(ordered, value)
	}

	return out.Value(values, func(w io.Writer) error {
		for _, value := range ordered {
			out.Printf("%s\n", value)
		}
		return nil
	})
}

func installedEngineNames(session *core.Session) ([]string, error) {
	manager, err := session.EngineManager()
	if err != nil {
		return nil, err
	}

	engines, err := manager.List()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(engines))
	for _, engine := range engines {
		names = append(names, engine.Name)
	}
	return names, nil
}

func orFirst(value string, options []string) string {
	if value != "" {
		return value
	}
	return options[0]
}

func orUnset(value string) string {
	if value == "" {
		return "(unset)"
	}
	return value
}

func configPathOrDefault(path string) string {
	if path == "" {
		return "the Godai config file"
	}
	return path
}
