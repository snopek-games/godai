package main

import (
	"context"
	"errors"
	"fmt"
	"gitlab.com/snopek-games/godai/mcp/server"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var defaultGodotPath string
	var projectBasePath string

	if path, err := exec.LookPath("godot"); err == nil {
		defaultGodotPath = path
	}

	// @todo Allow overriding this via an environment variable
	configPath, err := server.GetConfigPath()
	if err == nil {
		if sc, err := server.LoadConfig(configPath); err == nil {
			defaultGodotPath = sc.DefaultGodotPath
			projectBasePath = sc.ProjectBasePath
		}
	}

	// The directory where running Godot editors advertise themselves.
	instancesPath, _ := server.GetInstancesPath()

	cmd := cli.Command{
		Name:    "godai-mcp",
		Usage:   "MCP server for Godot",
		Version: server.GodaiVersion,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "root",
				Usage: "file system path(s) with Godot project(s) the server is allowed to connect to",
			},
			&cli.BoolFlag{
				Name:  "global",
				Usage: "connect to all Godot editor instances",
			},
			&cli.StringFlag{
				Name:    "godot-path",
				Usage:   "default path to the Godot executable",
				Value:   defaultGodotPath,
				Sources: cli.EnvVars("GODOT"),
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
				Value: 2,
			},
			&cli.IntFlag{
				Name:  "editor-retry-delay",
				Usage: "the delay (in seconds) between attempts to connect to the Godot editor",
				Value: 2,
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
				Name:    "debug",
				Usage:   "enable debug features and logging",
				Sources: cli.EnvVars("DEBUG"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runServer(ctx, cmd, configPath)
		},
	}

	if err := cmd.Run(ctx, os.Args); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("something went wrong", "error", err)
			os.Exit(1)
		}
	}

	slog.Debug("exiting normally")
}

func runServer(ctx context.Context, cmd *cli.Command, configPath string) error {
	config := &server.Config{
		Global:              cmd.Bool("global"),
		RootPaths:           cmd.StringSlice("root"),
		EditorInstancesPath: cmd.String("editor-instances-path"),
		EditorScanInterval:  time.Second * time.Duration(cmd.Int("editor-scan-interval")),
		EditorRetryDelay:    time.Second * time.Duration(cmd.Int("editor-retry-delay")),
		EditorTimeout:       time.Second * time.Duration(cmd.Int("editor-timeout")),
		EditorToolTimeout:   time.Second * time.Duration(cmd.Int("editor-tool-timeout")),
		DefaultGodotPath:    cmd.String("godot-path"),
		ProjectBasePath:     cmd.String("project-base-path"),
		X11Display:          cmd.String("x11-display"),
		Debug:               cmd.Bool("debug"),
		SavedConfigPath:     configPath,
	}

	var output io.Writer
	logFile := cmd.String("log-file")
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		output = f
	} else {
		output = os.Stderr
	}

	logLevel := slog.LevelInfo
	if config.Debug {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if config.Global {
		if len(config.RootPaths) > 0 {
			return fmt.Errorf("--global and --root are mutually exclusive")
		}
	} else if len(config.RootPaths) == 0 {
		config.RootPaths = tryDiscoverRootPaths()
	}

	if config.DefaultGodotPath != "" {
		if err := server.ValidateGodotExecutable(config.DefaultGodotPath); err != nil {
			return fmt.Errorf("invalid Godot path: %w", err)
		}
	}

	if config.ProjectBasePath != "" {
		if err := server.ValidateDirectory(config.ProjectBasePath); err != nil {
			return fmt.Errorf("invalid project base path: %w", err)
		}
	}

	slog.Debug("server starting", "config", config)
	s := server.NewServer(config)
	return s.Run(ctx)
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
