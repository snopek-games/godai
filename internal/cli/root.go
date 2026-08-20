package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

const (
	cliScanInterval   = 1 * time.Second
	cliRetryDelay     = 500 * time.Millisecond
	cliConnectWait    = 10 * time.Second
	cliListOpenWait   = 2 * time.Second
	cliOpenWait       = 120 * time.Second
	editorTimeout     = 15 * time.Second
	editorToolTimeout = 300 * time.Second
	serverScanTick    = 2 * time.Second
	serverRetryDelay  = 2 * time.Second
	serverOpenWait    = 30 * time.Second
)

// Help sorts the groups by name, so these are worded to also read well in
// alphabetical order.
const (
	flagCategoryGodot   = "Choosing a Godot version:"
	flagCategoryProject = "Choosing a project:"
	flagCategoryEditor  = "Connecting to the editor:"
	flagCategoryOutput  = "Output:"
)

func Root() *cli.Command {
	godotVersion, godotPath, projectBasePath, configPath := loadDefaults()
	instancesPath, _ := core.GetInstancesPath()

	root := &cli.Command{
		Name:                  "godai",
		Usage:                 "Godot automation CLI, with an MCP server for AI agents",
		Version:               core.Version,
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "root",
				Category: flagCategoryProject,
				Usage:    "file system path(s) with Godot project(s) that may be used",
			},
			&cli.BoolFlag{
				Name:     "global",
				Category: flagCategoryProject,
				Usage:    "use any Godot editor instance, wherever its project lives",
			},
			&cli.StringFlag{
				Name:     "godot-version",
				Category: flagCategoryGodot,
				Usage:    "version of Godot to use (see 'godai engine list'), overriding whatever version a project asks for",
				Value:    godotVersion,
			},
			// GODOT is read in loadDefaults() rather than declared as a source,
			// so that IsSet("godot-path") stays "the user typed this" and an
			// ambient GODOT doesn't get validated as if they had.
			&cli.StringFlag{
				Name:     "godot-path",
				Category: flagCategoryGodot,
				Usage:    "path to a Godot executable, used instead of any configured version [$GODOT]",
				Value:    godotPath,
			},
			&cli.BoolFlag{
				Name:     "no-auto-install",
				Category: flagCategoryGodot,
				Usage:    "don't download a version of Godot a project asks for but doesn't have",
			},
			&cli.StringFlag{
				Name:     "project-base-path",
				Category: flagCategoryProject,
				Usage:    "base path where your Godot projects usually live",
				Value:    projectBasePath,
			},
			&cli.StringFlag{
				Name:     "editor-instances-path",
				Category: flagCategoryEditor,
				Usage:    "directory where running Godot editors write their instance files",
				Value:    instancesPath,
			},
			// The real defaults live in durationFlag, which only applies them
			// when the flag isn't set, so there's no Value for urfave to print.
			&cli.FloatFlag{
				Name:        "editor-scan-interval",
				Category:    flagCategoryEditor,
				Usage:       "how often (in seconds) to scan for running Godot editors",
				DefaultText: secondsText(cliScanInterval),
			},
			&cli.FloatFlag{
				Name:        "editor-retry-delay",
				Category:    flagCategoryEditor,
				Usage:       "the delay (in seconds) between attempts to connect to the Godot editor",
				DefaultText: secondsText(cliRetryDelay),
			},
			&cli.FloatFlag{
				Name:     "editor-timeout",
				Category: flagCategoryEditor,
				Usage:    "the timeout (in seconds) when making a request to the editor",
				Value:    editorTimeout.Seconds(),
			},
			&cli.FloatFlag{
				Name:     "editor-tool-timeout",
				Category: flagCategoryEditor,
				Usage:    "the timeout (in seconds) when calling a tool in the editor, which may include waiting for the user to approve it",
				Value:    editorToolTimeout.Seconds(),
			},
			&cli.FloatFlag{
				Name:     "connect-timeout",
				Category: flagCategoryEditor,
				Usage:    "how long (in seconds) to wait for a Godot editor to connect",
				Value:    cliConnectWait.Seconds(),
			},
			&cli.FloatFlag{
				Name:        "open-timeout",
				Category:    flagCategoryEditor,
				Usage:       "how long (in seconds) to wait for an editor being opened to import its project and connect",
				Sources:     cli.EnvVars("GODAI_OPEN_TIMEOUT"),
				DefaultText: secondsText(cliOpenWait),
			},
			&cli.StringFlag{
				Name:     "x11-display",
				Category: flagCategoryEditor,
				Usage:    "the x11 DISPLAY variable (may be needed on Linux to launch the editor)",
				Sources:  cli.EnvVars("DISPLAY"),
				Value:    ":0",
			},
			&cli.StringFlag{
				Name:     "log-file",
				Category: flagCategoryOutput,
				Usage:    "a file to write the log output to",
			},
			&cli.BoolFlag{
				Name:     "json",
				Category: flagCategoryOutput,
				Usage:    "print results as JSON",
			},
			// No -v alias: urfave/cli gives that to --version, and having both
			// claim it makes which one you get depend on flag order.
			&cli.BoolFlag{
				Name:     "verbose",
				Category: flagCategoryOutput,
				Usage:    "print more about what's happening",
			},
			&cli.BoolFlag{
				Name:     "debug",
				Category: flagCategoryOutput,
				Usage:    "enable debug features and logging",
				Sources:  cli.EnvVars("DEBUG"),
			},
			&cli.BoolFlag{
				Name:     "no-input",
				Category: flagCategoryOutput,
				Usage:    "never prompt, even when there's a terminal to prompt on",
			},
		},
		Commands: []*cli.Command{
			mcpCommand(configPath),
			projectCommand(configPath),
			configCommand(configPath),
			editorToolCommand(configPath),
			editorCommand(configPath),
			engineCommand(configPath),
			selfUpdateCommand(),
		},
	}

	// urfave/cli prints its own errors and exits the process on its own.
	// Handing them back keeps every exit code and message coming from one place.
	root.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	root.Before = startUpdateNotice

	setErrorHandlers(root)
	wrapHelp()
	trimHelpGlobals()
	adjustFlagHelp()

	return root
}

// urfave/cli only consults the handler on the command that failed to parse, so
// setting it on the root alone would leave subcommands exiting 1.
func setErrorHandlers(command *cli.Command) {
	command.OnUsageError = onUsageError
	if len(command.Commands) > 0 && command.Action == nil {
		command.Action = defaultCommandAction
	}
	for _, sub := range command.Commands {
		setErrorHandlers(sub)
	}
}

func onUsageError(_ context.Context, cmd *cli.Command, err error, _ bool) error {
	const unknownFlag = "flag provided but not defined: -"
	if msg := err.Error(); cmd != nil && strings.HasPrefix(msg, unknownFlag) {
		provided := strings.TrimLeft(strings.TrimPrefix(msg, unknownFlag), "-")
		if suggestion := cli.SuggestFlag(cmd.Flags, provided, false); suggestion != "" {
			return usageError{fmt.Errorf("%s (did you mean %s?)", msg, suggestion)}
		}
	}
	return usageError{err}
}

// Without this, urfave/cli answers an unknown subcommand with "No help topic
// for ..." and exit code 3, which is our "not configured" code.
func defaultCommandAction(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Present() {
		unknown := cmd.Args().First()
		if suggestion := cli.SuggestCommand(cmd.Commands, unknown); suggestion != "" {
			return newUsageError("unknown command: %s %s (did you mean %q?)", cmd.FullName(), unknown, suggestion)
		}
		return newUsageError("unknown command: %s %s", cmd.FullName(), unknown)
	}
	if cmd.Root() == cmd {
		return cli.ShowRootCommandHelp(cmd)
	}
	return cli.ShowSubcommandHelp(cmd)
}

func loadDefaults() (godotVersion, godotPath, projectBasePath, configPath string) {
	// @todo Allow overriding this via an environment variable
	configPath, err := core.GetConfigPath()
	if err == nil {
		if sc, err := core.LoadConfig(configPath); err == nil {
			godotVersion = sc.GodotVersion
			projectBasePath = sc.ProjectBasePath
		}
	}

	return godotVersion, os.Getenv("GODOT"), projectBasePath, configPath
}

func setupLogging(cmd *cli.Command) error {
	level := slog.LevelWarn
	switch {
	case cmd.Bool("debug"):
		level = slog.LevelDebug
	case cmd.Bool("verbose"):
		level = slog.LevelInfo
	}

	if logFile := cmd.String("log-file"); logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})))
		return nil
	}

	if cmd.Bool("debug") {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
		return nil
	}

	slog.SetDefault(slog.New(output.NewLogHandler(os.Stderr, level, useColor())))
	return nil
}

func printer(cmd *cli.Command) *output.Printer {
	return output.NewPrinter(os.Stdout, os.Stderr, cmd.Bool("json"))
}

func sessionConfig(cmd *cli.Command, configPath string) (core.Config, error) {
	config := core.Config{
		Scope:               core.ScopeGlobal,
		RootPaths:           cmd.StringSlice("root"),
		EditorInstancesPath: cmd.String("editor-instances-path"),
		EditorScanInterval:  durationFlag(cmd, "editor-scan-interval", cliScanInterval),
		EditorRetryDelay:    durationFlag(cmd, "editor-retry-delay", cliRetryDelay),
		EditorTimeout:       durationFlag(cmd, "editor-timeout", editorTimeout),
		EditorToolTimeout:   durationFlag(cmd, "editor-tool-timeout", editorToolTimeout),
		OpenProjectTimeout:  durationFlag(cmd, "open-timeout", cliOpenWait),
		GodotVersion:        cmd.String("godot-version"),
		GodotPath:           cmd.String("godot-path"),
		NoAutoInstall:       cmd.Bool("no-auto-install"),
		ProjectBasePath:     cmd.String("project-base-path"),
		X11Display:          cmd.String("x11-display"),
		Debug:               cmd.Bool("debug"),
		SavedConfigPath:     configPath,
		CloseHeadlessOnExit: false,

		// The saved default is the flag's value when it isn't given, so IsSet
		// is what separates "use this one" from "this is my usual one".
		GodotVersionIsExplicit: cmd.IsSet("godot-version"),
	}

	if sc, err := core.LoadConfig(configPath); err == nil {
		config.UpdateCheck = sc.UpdateCheck
	}

	if cmd.Bool("global") {
		if len(config.RootPaths) > 0 {
			return config, newUsageError("--global and --root are mutually exclusive")
		}
	} else if len(config.RootPaths) > 0 {
		config.Scope = core.ScopeRoots
	}

	if err := validatePathFlags(cmd, &config); err != nil {
		return config, err
	}

	return config, nil
}

func validatePathFlags(cmd *cli.Command, config *core.Config) error {
	if cmd.IsSet("godot-path") && config.GodotPath != "" {
		resolved, err := core.ResolveGodotExecutable(config.GodotPath)
		if err != nil {
			return newUsageError("invalid Godot path: %v", err)
		}
		config.GodotPath = resolved
	}
	if cmd.IsSet("project-base-path") && config.ProjectBasePath != "" {
		if err := core.ValidateDirectory(config.ProjectBasePath); err != nil {
			return newUsageError("invalid project base path: %v", err)
		}
	}
	return nil
}

func rejectArgs(cmd *cli.Command) error {
	if cmd.Args().Present() {
		return newUsageError("unexpected arguments: %v", cmd.Args().Slice())
	}
	return nil
}

func atMostOneArg(cmd *cli.Command, what string) error {
	if cmd.Args().Len() > 1 {
		return newUsageError("expected at most one %s", what)
	}
	return nil
}

func durationFlag(cmd *cli.Command, name string, fallback time.Duration) time.Duration {
	if !cmd.IsSet(name) {
		return fallback
	}
	return seconds(cmd.Float(name))
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

// Matches how urfave/cli renders a float flag's own default.
func secondsText(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'g', -1, 64)
}

func connectWait(cmd *cli.Command) time.Duration {
	wait := seconds(cmd.Float("connect-timeout"))
	if wait <= 0 {
		return cliConnectWait
	}
	return wait
}

func listOpenWait(cmd *cli.Command) time.Duration {
	if cmd.IsSet("connect-timeout") {
		return connectWait(cmd)
	}
	return cliListOpenWait
}

func withSession(ctx context.Context, cmd *cli.Command, configPath string, fn func(*core.Session) error) error {
	if err := setupLogging(cmd); err != nil {
		return err
	}

	config, err := sessionConfig(cmd, configPath)
	if err != nil {
		return err
	}

	session, err := core.New(config)
	if err != nil {
		return err
	}
	defer session.Close()

	session.SetPrompter(newPrompter(cmd))
	session.SetInstallReporter(installReporter(cmd))

	return fn(session)
}

type Printer = output.Printer
