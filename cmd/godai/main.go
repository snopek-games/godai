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

	if err := cli.Root().Run(ctx, os.Args); err != nil {
		if code := cli.ExitCodeFor(err); code != cli.ExitInterrupted {
			cli.PrintError(os.Stderr, err, wantsJSON(os.Args))
			os.Exit(code)
		}
		os.Exit(cli.ExitInterrupted)
	}
}

// The parsed command isn't available once Run has returned with an error.
func wantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
		if arg == "--" {
			break
		}
	}
	return false
}
