package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/selfupdate"

	"github.com/urfave/cli/v3"
)

func selfUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:  "self-update",
		Usage: "update godai to the latest release",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "check",
				Usage: "only report whether a newer release is available",
			},
			&cli.BoolFlag{
				Name:  "no-cache",
				Usage: "with --check, ask GitLab even when the release data was fetched within the last day (an actual update always asks)",
			},
			&cli.BoolFlag{
				Name:  "rollback",
				Usage: "restore the version that the last update replaced",
			},
			&cli.BoolFlag{
				Name:  "dangerously-allow-unverified",
				Usage: "DANGEROUS: install the download without checking it against the release checksums",
			},
		},
		Action: runSelfUpdate,
	}
}

func runSelfUpdate(ctx context.Context, cmd *cli.Command) error {
	logLevel := slog.LevelInfo
	if cmd.Bool("debug") {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	if cmd.Args().Present() {
		return newUsageError("unexpected arguments: %v", cmd.Args().Slice())
	}

	check := cmd.Bool("check")
	rollback := cmd.Bool("rollback")
	allowUnverified := cmd.Bool("dangerously-allow-unverified")
	if check && rollback {
		return newUsageError("--check and --rollback are mutually exclusive")
	}

	exePath, err := selfupdate.ExecutablePath()
	if err != nil {
		return err
	}

	if rollback {
		return runRollback(exePath)
	}

	if !check {
		if hint := selfupdate.PackageManagerHint(exePath); hint != "" {
			return errors.New(hint)
		}
	}

	if allowUnverified {
		fmt.Fprintln(os.Stderr,
			"DANGER: --dangerously-allow-unverified is set, so the download will be installed without checking it against the release checksums")
	}

	updater, err := selfupdate.New(selfupdate.Config{
		CurrentVersion:  core.Version,
		AllowUnverified: allowUnverified,
	})
	if err != nil {
		return err
	}

	if check {
		return runCheck(ctx, cmd, updater, exePath)
	}

	release, err := updater.DetectLatest(ctx)
	if err != nil {
		// No installable release at all means there's nothing newer, the same
		// way CheckCached treats it - not something to bother the user about.
		if errors.Is(err, selfupdate.ErrNoRelease) {
			fmt.Printf("godai %s is up-to-date\n", core.Version)
			return nil
		}
		return err
	}

	if !updater.IsNewer(release) {
		fmt.Printf("godai %s is up-to-date\n", core.Version)
		return nil
	}

	fmt.Printf("updating %s to godai %s ...\n", exePath, release.Version)

	backupPath, err := updater.Update(ctx, release, exePath)
	if err != nil {
		return err
	}

	fmt.Printf("updated %s to godai %s\n", exePath, release.Version)
	fmt.Printf("the previous version was kept at %s; run '%s self-update --rollback' to restore it\n", backupPath, exePath)
	return nil
}

func runCheck(ctx context.Context, cmd *cli.Command, updater *selfupdate.Updater, exePath string) error {
	cachePath, err := core.GetUpdateCheckCachePath()
	if err != nil {
		return err
	}

	interval := selfupdate.DefaultCheckInterval
	if cmd.Bool("no-cache") {
		interval = 0
	}

	latest, newer, err := updater.CheckCached(ctx, cachePath, interval)
	if err != nil {
		return err
	}

	if !newer {
		fmt.Printf("godai %s is up-to-date\n", core.Version)
		return nil
	}

	fmt.Printf("godai %s is available (currently running %s)\n", latest, core.Version)
	fmt.Printf("%s\n", selfupdate.InstallInstruction(exePath))
	return nil
}

func runRollback(exePath string) error {
	if err := selfupdate.Rollback(exePath); err != nil {
		return err
	}

	fmt.Printf("restored the previous version of %s\n", exePath)
	fmt.Printf("run '%s --version' to see which version is installed now, or 'self-update --rollback' again to undo this\n", exePath)
	return nil
}
