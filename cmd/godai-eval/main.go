package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"
)

const envFile = ".env"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := command().Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "godai-eval: "+err.Error())
		os.Exit(1)
	}
}

func loadEnvFile() {
	path, err := filepath.Abs(envFile)
	if err != nil {
		return
	}
	if err := godotenv.Load(path); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("%s: %v", path, err)
		}
		return
	}
	log.Printf("loaded %s", path)
}

func command() *cli.Command {
	cmd := &cli.Command{
		Name:                  "godai-eval",
		Usage:                 "measure how well a model drives Godot through godai",
		Version:               core.Version,
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			runCommand(),
			matrixCommand(),
			compareCommand(),
		},
		Action: showHelp,
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			loadEnvFile()
			return ctx, nil
		},
	}

	// urfave/cli prints its own errors and exits on its own. Handing them back
	// keeps the message and the exit code coming from one place.
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	return cmd
}

// Without this, an unknown subcommand is answered with "No help topic for ...".
func showHelp(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Present() {
		return fmt.Errorf("unknown command: %s", cmd.Args().First())
	}
	return cli.ShowRootCommandHelp(cmd)
}
