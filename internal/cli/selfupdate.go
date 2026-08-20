package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"

	"gitlab.com/snopek-games/godai/internal/cli/output"
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

	out := printer(cmd)

	if rollback {
		return runRollback(out, exePath)
	}

	if !check {
		if hint := selfupdate.PackageManagerHint(exePath); hint != "" {
			return errors.New(hint)
		}
	}

	if allowUnverified {
		out.Warn("--dangerously-allow-unverified is set, so the download will be installed without checking it against the release checksums")
	}

	updater, err := selfupdate.New(selfupdate.Config{
		CurrentVersion:  core.Version,
		AllowUnverified: allowUnverified,
	})
	if err != nil {
		return err
	}

	if check {
		return runCheck(ctx, cmd, out, updater, exePath)
	}

	release, err := updater.DetectLatest(ctx)
	if err != nil {
		// No installable release at all means there's nothing newer, the same
		// way CheckCached treats it - not something to bother the user about.
		if errors.Is(err, selfupdate.ErrNoRelease) {
			return printUpToDate(out, "")
		}
		return err
	}

	if !updater.IsNewer(release) {
		return printUpToDate(out, release.Version.String())
	}

	out.Printf("updating %s to godai %s ...\n", exePath, out.Paint(output.Cyan, release.Version.String()))

	backupPath, err := updater.Update(ctx, release, exePath)
	if err != nil {
		return err
	}

	return out.Value(struct {
		Previous   string `json:"previous"`
		Version    string `json:"version"`
		Path       string `json:"path"`
		BackupPath string `json:"backup_path"`
	}{core.Version, release.Version.String(), exePath, backupPath}, func(io.Writer) error {
		out.Printf("updated %s to godai %s\n", exePath, out.Paint(output.Cyan, release.Version.String()))
		out.Printf("the previous version was kept at %s; run '%s' to restore it\n", backupPath, out.Paint(output.BoldCyan, exePath+" self-update --rollback"))
		return nil
	})
}

func printUpToDate(out *Printer, latest string) error {
	return out.Value(struct {
		Current string `json:"current"`
		Latest  string `json:"latest,omitempty"`
		Newer   bool   `json:"newer"`
	}{core.Version, latest, false}, func(io.Writer) error {
		out.Printf("godai %s is up-to-date\n", out.Paint(output.Cyan, core.Version))
		return nil
	})
}

func runCheck(ctx context.Context, cmd *cli.Command, out *Printer, updater *selfupdate.Updater, exePath string) error {
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
		// A cached "nothing newer" carries no release, leaving latest zero.
		if latest == (selfupdate.Version{}) {
			return printUpToDate(out, "")
		}
		return printUpToDate(out, latest.String())
	}

	return out.Value(struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
		Newer   bool   `json:"newer"`
	}{core.Version, latest.String(), true}, func(io.Writer) error {
		out.Printf("godai %s is available (currently running %s)\n", out.Paint(output.Cyan, latest.String()), out.Paint(output.Cyan, core.Version))
		out.Printf("%s\n", selfupdate.InstallInstruction(exePath))
		return nil
	})
}

func runRollback(out *Printer, exePath string) error {
	if err := selfupdate.Rollback(exePath); err != nil {
		return err
	}

	return out.Value(struct {
		RolledBack bool   `json:"rolled_back"`
		Path       string `json:"path"`
	}{true, exePath}, func(io.Writer) error {
		out.Printf("restored the previous version of %s\n", exePath)
		out.Printf("run '%s' to see which version is installed now, or '%s' again to undo this\n", out.Paint(output.BoldCyan, exePath+" --version"), out.Paint(output.BoldCyan, "self-update --rollback"))
		return nil
	})
}
