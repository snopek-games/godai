package cli

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

const (
	cliScanInterval  = 1 * time.Second
	cliRetryDelay    = 500 * time.Millisecond
	cliConnectWait   = 10 * time.Second
	cliListOpenWait  = 2 * time.Second
	cliOpenWait      = 120 * time.Second
	serverScanTick   = 2 * time.Second
	serverRetryDelay = 2 * time.Second
	serverOpenWait   = 30 * time.Second
)

func Root() *cli.Command {
	defaultGodotPath, projectBasePath, configPath := loadDefaults()
	instancesPath, _ := core.GetInstancesPath()

	root := &cli.Command{
		Name:                  "godai",
		Usage:                 "Godot automation CLI, with an MCP server for AI assistants",
		Version:               core.Version,
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "root",
				Usage: "file system path(s) with Godot project(s) that may be used",
			},
			&cli.BoolFlag{
				Name:  "global",
				Usage: "use any Godot editor instance, wherever its project lives",
			},
			// GODOT is read in loadDefaults() rather than declared as a source,
			// so that IsSet("godot-path") stays "the user typed this" and an
			// ambient GODOT doesn't get validated as if they had.
			&cli.StringFlag{
				Name:  "godot-path",
				Usage: "default path to the Godot executable [$GODOT]",
				Value: defaultGodotPath,
			},
			&cli.StringFlag{
				Name:  "project-base-path",
				Usage: "base path where your Godot projects usually live",
				Value: projectBasePath,
			},
			&cli.StringFlag{
				Name:  "editor-instances-path",
				Usage: "directory where running Godot editors write their instance files",
				Value: instancesPath,
			},
			&cli.IntFlag{
				Name:  "editor-scan-interval",
				Usage: "how often (in seconds) to scan for running Godot editors",
			},
			&cli.IntFlag{
				Name:  "editor-retry-delay",
				Usage: "the delay (in seconds) between attempts to connect to the Godot editor",
			},
			&cli.IntFlag{
				Name:  "editor-timeout",
				Usage: "the timeout (in seconds) when making a request to the editor",
				Value: 15,
			},
			&cli.IntFlag{
				Name:  "editor-tool-timeout",
				Usage: "the timeout (in seconds) when calling a tool in the editor, which may include waiting for the user to approve it",
				Value: 300,
			},
			&cli.IntFlag{
				Name:  "connect-timeout",
				Usage: "how long (in seconds) to wait for a Godot editor to connect",
				Value: int(cliConnectWait / time.Second),
			},
			&cli.StringFlag{
				Name:    "x11-display",
				Usage:   "the x11 DISPLAY variable (may be needed on Linux to launch the editor)",
				Sources: cli.EnvVars("DISPLAY"),
				Value:   ":0",
			},
			&cli.StringFlag{
				Name:  "log-file",
				Usage: "a file to write the log output to",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "print results as JSON",
			},
			// No -v alias: urfave/cli gives that to --version, and having both
			// claim it makes which one you get depend on flag order.
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "print more about what's happening",
			},
			&cli.BoolFlag{
				Name:    "debug",
				Usage:   "enable debug features and logging",
				Sources: cli.EnvVars("DEBUG"),
			},
			&cli.BoolFlag{
				Name:  "no-input",
				Usage: "never prompt, even when there's a terminal to prompt on",
			},
		},
		Commands: []*cli.Command{
			mcpCommand(configPath),
			projectCommand(configPath),
			configCommand(configPath),
			editorToolCommand(configPath),
			editorCommand(configPath),
			selfUpdateCommand(),
		},
	}

	// urfave/cli prints its own errors and exits the process on its own.
	// Handing them back keeps every exit code and message coming from one place.
	root.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	setErrorHandlers(root)
	wrapHelp()

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

func onUsageError(_ context.Context, _ *cli.Command, err error, _ bool) error {
	return usageError{err}
}

// Without this, urfave/cli answers an unknown subcommand with "No help topic
// for ..." and exit code 3, which is our "not configured" code.
func defaultCommandAction(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Present() {
		return newUsageError("unknown command: %s %s", cmd.FullName(), cmd.Args().First())
	}
	if cmd.Root() == cmd {
		return cli.ShowRootCommandHelp(cmd)
	}
	return cli.ShowSubcommandHelp(cmd)
}

func loadDefaults() (godotPath, projectBasePath, configPath string) {
	// @todo Allow overriding this via an environment variable
	configPath, err := core.GetConfigPath()
	if err == nil {
		if sc, err := core.LoadConfig(configPath); err == nil {
			if sc.DefaultGodotPath != "" {
				godotPath = sc.DefaultGodotPath
			}
			projectBasePath = sc.ProjectBasePath
		}
	}

	if env := os.Getenv("GODOT"); env != "" {
		godotPath = env
	}

	execNames := []string{"godot4", "godot"}
	for _, execName := range execNames {
		if godotPath == "" {
			if path, err := exec.LookPath(execName); err == nil {
				godotPath = path
			}
		}
	}

	return godotPath, projectBasePath, configPath
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
		EditorTimeout:       durationFlag(cmd, "editor-timeout", 15*time.Second),
		EditorToolTimeout:   durationFlag(cmd, "editor-tool-timeout", 300*time.Second),
		OpenProjectTimeout:  cliOpenWait,
		DefaultGodotPath:    cmd.String("godot-path"),
		ProjectBasePath:     cmd.String("project-base-path"),
		X11Display:          cmd.String("x11-display"),
		Debug:               cmd.Bool("debug"),
		SavedConfigPath:     configPath,
		CloseHeadlessOnExit: false,
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
	if cmd.IsSet("godot-path") && config.DefaultGodotPath != "" {
		resolved, err := core.ResolveGodotExecutable(config.DefaultGodotPath)
		if err != nil {
			return newUsageError("invalid Godot path: %v", err)
		}
		config.DefaultGodotPath = resolved
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
	return time.Duration(cmd.Int(name)) * time.Second
}

func connectWait(cmd *cli.Command) time.Duration {
	wait := time.Duration(cmd.Int("connect-timeout")) * time.Second
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

	return fn(session)
}

type Printer = output.Printer
