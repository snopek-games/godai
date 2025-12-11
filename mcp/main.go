package main

import (
	"context"
	"errors"
	"godai/mcp/server"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cmd := cli.Command{
		Name:  "godai-mcp",
		Usage: "MCP server for Godot",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "godot-path",
				Usage:   "default path to the Godot executable",
				Sources: cli.EnvVars("GODOT"),
			},
			&cli.StringFlag{
				Name:  "project-path",
				Usage: "base path where Godot projects usually live",
			},
			&cli.IntFlag{
				Name:  "editor-base-port",
				Usage: "base port used to connect to the Godot editor",
				Value: 12120,
			},
			&cli.IntFlag{
				Name:  "editor-port-count",
				Usage: "number of ports to try when connecting to the Godot editor",
				Value: 10,
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
			&cli.StringFlag{
				Name:    "x11-display",
				Usage:   "the x11 DISPLAY variable (may be needed on Linux to launch the editor)",
				Sources: cli.EnvVars("DISPLAY"),
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
		Action: runServer,
	}

	if err := cmd.Run(ctx, os.Args); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("something went wrong", "error", err)
			os.Exit(1)
		}
	}

	slog.Debug("exiting normally")
}

func runServer(ctx context.Context, cmd *cli.Command) error {
	config := &server.Config{
		EditorBasePort:   cmd.Int("editor-base-port"),
		EditorPortCount:  cmd.Int("editor-port-count"),
		EditorRetryDelay: time.Second * time.Duration(cmd.Int("editor-retry-delay")),
		EditorTimeout:    time.Second * time.Duration(cmd.Int("editor-timeout")),
		DefaultGodotPath: cmd.String("godot-path"),
		ProjectBasePath:  cmd.String("project-path"),
		X11Display:       cmd.String("x11-display"),
		Debug:            cmd.Bool("debug"),
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

	slog.Debug("server starting", "config", config)
	s := server.NewServer(config)
	return s.Run(ctx)
}
