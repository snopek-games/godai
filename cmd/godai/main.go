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

	code := 0
	if err := cli.Root().Run(ctx, os.Args); err != nil {
		code = cli.ExitCodeFor(err)
		if code != cli.ExitInterrupted {
			cli.PrintError(os.Stderr, err, wantsJSON(os.Args))
		}
	}

	if code != cli.ExitInterrupted {
		cli.PrintUpdateNotice(os.Stderr)
	}
	if code != 0 {
		os.Exit(code)
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
