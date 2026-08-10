package main

import (
	"context"
	"errors"
	"fmt"
	"gitlab.com/snopek-games/godai/internal/godot"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := cli.Command{
		Name:  "godot-config-roundtrip-test",
		Usage: "check that Godot config files survive a parse/write round trip",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name: "path",
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "continue",
				Usage: "keep processing after first error",
			},
		},
		Action: runCommand,
	}

	ctx := context.Background()

	if err := cmd.Run(ctx, os.Args); err != nil {
		slog.Error("something went wrong", "error", err)
		os.Exit(1)
	}
}

func runCommand(ctx context.Context, cmd *cli.Command) error {
	root := cmd.StringArg("path")
	if root == "" {
		return errors.New("path is required")
	}

	cont := cmd.Bool("continue")
	var errs []error

	pathInfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	if pathInfo.IsDir() {
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}

		for _, e := range entries {
			if e.IsDir() {
				path := filepath.Join(root, e.Name())
				if !godot.IsValidProject(path) {
					continue
				}

				if err := processConfig(filepath.Join(path, "project.godot")); err != nil {
					err = fmt.Errorf("round-trip failed for '%s': %w", path, err)
					if !cont {
						return err
					} else {
						errs = append(errs, err)
					}
				}
			}
		}
	} else {
		if err := processConfig(root); err != nil {
			return err
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func processConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	s := string(b)
	cf, err := godot.ParseConfigFile(s)
	if err != nil {
		return err
	}

	configVersion, ok := cf.GetInt64("", "config_version")
	if ok && configVersion < 5 {
		return errors.New("cannot round-trip config version less than 5")
	}

	roundtrip, err := cf.String()
	if err != nil {
		return err
	}

	want := strings.Split(strings.TrimSpace(s), "\n")
	got := strings.Split(strings.TrimSpace(roundtrip), "\n")

	if diff := cmp.Diff(want, got); diff != "" {
		return fmt.Errorf("mismatch (-want +got):\n%s", diff)
	}

	return nil
}
