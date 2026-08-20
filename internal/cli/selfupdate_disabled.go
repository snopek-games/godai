//go:build !selfupdate

package cli

import (
	"context"
	"errors"

	"github.com/urfave/cli/v3"
)

func selfUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:            "self-update",
		Usage:           "update godai to the latest release",
		Hidden:          true,
		SkipFlagParsing: true,
		Action: func(context.Context, *cli.Command) error {
			return errors.New("this build of godai was compiled without self-update")
		},
	}
}
