package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/snopek-games/godai/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := cli.Root()

	code := 0
	if err := root.Run(ctx, os.Args); err != nil {
		code = cli.ExitCodeFor(err)
		if code != cli.ExitInterrupted {
			cli.PrintError(os.Stderr, err, root.Bool("json"), root.Bool("verbose") || root.Bool("debug"))
		}
	}

	if code != cli.ExitInterrupted {
		cli.PrintUpdateNotice(os.Stderr)
	}
	if code != 0 {
		os.Exit(code)
	}
}
