package cli

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/mcp"
	"gitlab.com/snopek-games/godai/internal/selfupdate"

	"github.com/urfave/cli/v3"
)

func mcpCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:  "mcp",
		Usage: "run the MCP server on stdin/stdout, for use by an AI assistant",
		Description: "Speaks the Model Context Protocol over stdio, exposing the same operations " +
			"as the other subcommands as MCP tools.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-update-check",
				Usage: "don't check whether a newer release of godai is available",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runServer(ctx, cmd, configPath)
		},
	}
}

func runServer(ctx context.Context, cmd *cli.Command, configPath string) error {
	if err := rejectArgs(cmd); err != nil {
		return err
	}

	if err := setupServerLogging(cmd); err != nil {
		return err
	}

	config := core.Config{
		Scope:               core.ScopeRoots,
		RootPaths:           cmd.StringSlice("root"),
		EditorInstancesPath: cmd.String("editor-instances-path"),
		EditorScanInterval:  durationFlag(cmd, "editor-scan-interval", serverScanTick),
		EditorRetryDelay:    durationFlag(cmd, "editor-retry-delay", serverRetryDelay),
		EditorTimeout:       durationFlag(cmd, "editor-timeout", 15*time.Second),
		EditorToolTimeout:   durationFlag(cmd, "editor-tool-timeout", 300*time.Second),
		OpenProjectTimeout:  serverOpenWait,
		DefaultGodotPath:    cmd.String("godot-path"),
		ProjectBasePath:     cmd.String("project-base-path"),
		X11Display:          cmd.String("x11-display"),
		UpdateCheckInterval: updateCheckInterval(cmd.Bool("no-update-check")),
		Debug:               cmd.Bool("debug"),
		SavedConfigPath:     configPath,
		CloseHeadlessOnExit: true,
	}

	if cmd.Bool("global") {
		if len(config.RootPaths) > 0 {
			return newUsageError("--global and --root are mutually exclusive")
		}
		config.Scope = core.ScopeGlobal
	} else if len(config.RootPaths) == 0 {
		config.RootPaths = tryDiscoverRootPaths()
	}

	if err := validatePathFlags(cmd, &config); err != nil {
		return err
	}

	slog.Debug("server starting", "config", config)

	session, err := core.New(config)
	if err != nil {
		return err
	}

	return mcp.NewServer(session).Run(ctx)
}

func setupServerLogging(cmd *cli.Command) error {
	var out io.Writer = os.Stderr
	if logFile := cmd.String("log-file"); logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		out = f
	}

	level := slog.LevelInfo
	if cmd.Bool("debug") {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})))
	return nil
}

func tryDiscoverRootPaths() []string {
	claudeProjectDir := os.Getenv("CLAUDE_PROJECT_DIR")
	if claudeProjectDir != "" {
		return []string{claudeProjectDir}
	}

	cwd, err := os.Getwd()
	if err == nil {
		return []string{cwd}
	}

	return []string{}
}

func updateCheckInterval(disabled bool) time.Duration {
	if disabled {
		return 0
	}
	return selfupdate.DefaultCheckInterval
}
