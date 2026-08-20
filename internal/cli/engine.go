package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/godot"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const progressInterval = 200 * time.Millisecond

func engineCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:  "engine",
		Usage: "download and manage versions of the Godot engine",
		Commands: []*cli.Command{
			engineListCommand(configPath),
			engineSearchCommand(configPath),
			engineInstallCommand(configPath),
			engineRemoveCommand(configPath),
			engineListTemplatesCommand(configPath),
			engineInstallTemplatesCommand(configPath),
			engineRemoveTemplatesCommand(configPath),
			engineUseCommand(configPath),
			engineWhichCommand(configPath),
			engineRunCommand(configPath),
			engineLinkCommand(configPath),
		},
	}
}

func engineListCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list the versions of Godot that are installed",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := rejectArgs(cmd); err != nil {
				return err
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				listings, err := session.ListEngines()
				if err != nil {
					return err
				}

				out := printer(cmd)
				if len(listings) == 0 && !cmd.Bool("json") {
					out.Printf("No versions of Godot are installed; `godai engine install <VERSION>` gets one.\n")
					return nil
				}

				return out.Value(struct {
					Versions []core.EngineListing `json:"versions"`
				}{listings}, func(io.Writer) error {
					rows := make([][]string, 0, len(listings))
					for _, listing := range listings {
						rows = append(rows, []string{engineLabel(listing), templatesCell(listing.Templates), defaultMark(listing.Default), listing.Path})
					}
					return out.Table([]string{"VERSION", "TEMPLATES", "DEFAULT", "PATH"}, rows)
				})
			})
		},
	}
}

func engineSearchCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:        "search",
		Usage:       "search the Godot versions available to install",
		ArgsUsage:   "[filter]",
		Description: "The filter matches from the start of the version, so `4.5` finds 4.5 and 4.5.1, but not 3.4.5.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "include dev, alpha, beta and rc releases",
			},
			&cli.BoolFlag{
				Name:  "refresh",
				Usage: "fetch the list of releases again rather than using the cached one",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := atMostOneArg(cmd, "filter"); err != nil {
				return err
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				results, err := session.SearchEngines(ctx, cmd.Args().First(), cmd.Bool("all"), cmd.Bool("refresh"))
				if err != nil {
					return err
				}

				out := printer(cmd)
				return out.Value(struct {
					Versions []core.EngineSearchResult `json:"versions"`
				}{results}, func(io.Writer) error {
					rows := make([][]string, 0, len(results))
					for _, r := range results {
						rows = append(rows, []string{r.Version, yesNo(r.Installed)})
					}
					return out.Table([]string{"VERSION", "INSTALLED"}, rows)
				})
			})
		},
	}
}

func engineInstallCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "download and install a version of Godot",
		ArgsUsage: "<version>",
		Description: "Versions are named the way the Godot downloads are: `4.5` or `4.4.1` for a stable release, `4.6-beta3` for a pre-release, and a `-mono` suffix for the .NET builds. A stable release can also be spelled out as `4.5-stable`, which is how Godai prints it back.\n\n" +
			"If no default version is set yet, the installed version will become the default. Use `godai engine use` to change the default.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "with-templates",
				Usage: "install the export templates for this version too",
			},
			allowUnverifiedFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			version, err := versionArg(cmd)
			if err != nil {
				return err
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				out := printer(cmd)
				out.Printf("Installing Godot %s...\n", out.Paint(output.Cyan, version.String()))

				result, err := session.InstallEngine(ctx, version.String(), downloadOptions(cmd))
				if err != nil {
					return err
				}
				out.Printf("Installed Godot %s to %s\n", out.Paint(output.Cyan, version.String()), result.Engine.Path)

				if result.BecameDefault {
					out.Printf("Godot %s is now the default\n", out.Paint(output.Cyan, result.Engine.Version))
				}

				if cmd.Bool("with-templates") {
					manager, err := session.EngineManager()
					if err != nil {
						return err
					}
					if err := installTemplatesIfNeeded(ctx, cmd, manager, version); err != nil {
						return err
					}
				}

				return out.Value(result.Engine, func(io.Writer) error { return nil })
			})
		},
	}
}

func engineRemoveCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove an installed or linked version of Godot",
		ArgsUsage: "<version>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "with-templates",
				Usage: "remove the export templates for this version too",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name, err := oneArg(cmd, "version")
			if err != nil {
				return err
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				result, err := session.RemoveEngine(name, cmd.Bool("with-templates"))
				if err != nil {
					return err
				}

				out := printer(cmd)
				if result.Linked {
					out.Printf("Unlinked %s\n", out.Paint(output.Cyan, result.Version))
				} else {
					out.Printf("Removed Godot %s\n", out.Paint(output.Cyan, result.Version))
				}

				if result.TemplatesRemoved {
					out.Printf("Removed the export templates for Godot %s\n", out.Paint(output.Cyan, result.Version))
				}

				if result.DefaultCleared {
					out.Warn("%s was the default, so there isn't one now; set another with `godai engine use <VERSION>`", result.Version)
				}

				return nil
			})
		},
	}
}

func engineListTemplatesCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "list-templates",
		Usage:     "list the export templates that are installed",
		ArgsUsage: "[version]",
		Description: "Export templates for a given version may be installed, even without the engine itself being installed at all.\n\n" +
			"Providing a version will list its templates platform by platform. Since Godot 4.7, export templates can be downloaded one platform at a time, so you may only have some, but not all.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := atMostOneArg(cmd, "version"); err != nil {
				return err
			}

			manager, err := engineManager(cmd, configPath)
			if err != nil {
				return err
			}

			if name := cmd.Args().First(); name != "" {
				version, err := godot.ParseEngineVersion(name)
				if err != nil {
					return newUsageError("%v", err)
				}

				templates, err := manager.Templates(version)
				if err != nil {
					return err
				}
				return printTemplateDetail(printer(cmd), templates)
			}

			installed, err := manager.ListTemplates()
			if err != nil {
				return err
			}
			return printTemplateList(printer(cmd), manager, installed)
		},
	}
}

func printTemplateList(out *Printer, manager *godot.EngineManager, installed []*godot.InstalledTemplates) error {
	type listing struct {
		Version    string   `json:"version"`
		Platforms  []string `json:"platforms"`
		Incomplete []string `json:"incomplete"`
		Complete   bool     `json:"complete"`
		Engine     bool     `json:"engine_installed"`
		Path       string   `json:"path"`
	}

	listings := make([]listing, 0, len(installed))
	for _, templates := range installed {
		listings = append(listings, listing{
			Version:    templates.Name,
			Platforms:  templates.Platforms(),
			Incomplete: templates.IncompletePlatforms(),
			Complete:   templates.Complete(),
			Engine:     manager.Installed(templates.Version),
			Path:       templates.Path,
		})
	}

	if len(listings) == 0 && !out.JSON {
		out.Printf("No export templates are installed; `godai engine install-templates <VERSION>` gets some.\n")
		return nil
	}

	anyIncomplete := false
	err := out.Value(struct {
		Templates []listing `json:"templates"`
	}{listings}, func(io.Writer) error {
		rows := make([][]string, 0, len(listings))
		for _, listing := range listings {
			platforms := "all"
			if !listing.Complete {
				platforms = strings.Join(markIncomplete(listing.Platforms, listing.Incomplete), ", ")
			}
			if len(listing.Incomplete) > 0 {
				anyIncomplete = true
			}
			rows = append(rows, []string{listing.Version, platforms, yesNo(listing.Engine)})
		}
		return out.Table([]string{"VERSION", "PLATFORMS", "ENGINE INSTALLED"}, rows)
	})
	if err != nil {
		return err
	}

	if anyIncomplete {
		out.Note("* some of that platform's templates are missing; `godai engine list-templates <VERSION>` shows which")
	}
	return nil
}

func markIncomplete(platforms, incomplete []string) []string {
	marked := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		if slices.Contains(incomplete, platform) {
			platform += "*"
		}
		marked = append(marked, platform)
	}
	return marked
}

func printTemplateDetail(out *Printer, templates *godot.InstalledTemplates) error {
	if templates.Empty() && !out.JSON {
		out.Printf("No export templates are installed for Godot %s\n", out.Paint(output.Cyan, templates.Name))
		return nil
	}

	err := out.Value(templates, func(io.Writer) error {
		rows := make([][]string, 0, len(templates.Sets))
		for _, set := range templates.Sets {
			rows = append(rows, []string{
				set.Name,
				fmt.Sprintf("%d/%d", len(set.Present), set.Total),
				templateSetState(set),
			})
		}
		return out.Table([]string{"TEMPLATE", "FILES", "STATUS"}, rows)
	})
	if err != nil {
		return err
	}

	if len(templates.Other) > 0 {
		out.Note("also there, but not something Godai knows about: %s", strings.Join(templates.Other, ", "))
	}
	if !templates.FromArchive && !templates.Complete() {
		out.Note("there's no %s, so these were downloaded a platform at a time rather than unpacked from a .tpz", "version.txt")
	}
	return nil
}

func templateSetState(set godot.TemplateSetStatus) string {
	switch {
	case set.Installed():
		return "installed"
	case set.Missing():
		return "missing"
	default:
		return "incomplete"
	}
}

func engineInstallTemplatesCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:        "install-templates",
		Usage:       "download and install the export templates for a version of Godot",
		ArgsUsage:   "<version>",
		Description: "To replace partial export templates with the complete set, run with `--force`.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "install over the export templates that are already there",
			},
			allowUnverifiedFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			manager, version, err := engineArgs(cmd, configPath)
			if err != nil {
				return err
			}
			return installTemplates(ctx, cmd, manager, version)
		},
	}
}

func engineRemoveTemplatesCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "remove-templates",
		Usage:     "remove the export templates for a version of Godot",
		ArgsUsage: "<version>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			manager, version, err := engineArgs(cmd, configPath)
			if err != nil {
				return err
			}

			if err := manager.RemoveTemplates(version); err != nil {
				return err
			}

			printer(cmd).Printf("Removed the export templates for Godot %s\n", version)
			return nil
		},
	}
}

func engineUseCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "use",
		Usage:     "make a version of Godot the default",
		ArgsUsage: "<version>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name, err := oneArg(cmd, "version")
			if err != nil {
				return err
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				if err := session.SetConfig(core.SavedConfig{GodotVersion: name}); err != nil {
					return err
				}
				printer(cmd).Printf("Godot %s is now the default\n", session.GetConfig().GodotVersion)
				return nil
			})
		},
	}
}

func engineWhichCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:        "which",
		Usage:       "print the path to a version of Godot",
		ArgsUsage:   "[version]",
		Description: "If the version is omitted, it will print the path to the Godot version that would be used in this context (the default, or the one pinned for the current project).",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := atMostOneArg(cmd, "version"); err != nil {
				return err
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				path, err := whichEngine(ctx, cmd, session)
				if err != nil {
					return err
				}
				fmt.Fprintln(os.Stdout, path)
				return nil
			})
		},
	}
}

func engineRunCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "run a version of Godot",
		ArgsUsage: "[version] [-- godot arguments...]",
		Description: "Arguments for Godot itself go after `--`, so that Godai doesn't try to read them:\n\n" +
			"    godai engine run 4.5 -- --headless --version\n\n" +
			"If the version is omitted, it will run the Godot version that would be used in this context (the default, or the one pinned for the current project).",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				path, err := whichEngine(ctx, cmd, session)
				if err != nil {
					return err
				}
				return runEngine(ctx, cmd, path, godotArgs(cmd))
			})
		},
	}
}

func engineLinkCommand(configPath string) *cli.Command {
	return &cli.Command{
		Name:        "link",
		Usage:       "give a name to a Godot executable Godai didn't install",
		ArgsUsage:   "<name> <path>",
		Description: "The name can be anything that isn't a Godot version, so that it can't be confused with one Godai could install. Remove a link with `godai engine remove <name>`.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 2 {
				return newUsageError("expected a name and a path to a Godot executable")
			}
			name, path := cmd.Args().Get(0), cmd.Args().Get(1)

			if _, err := godot.ParseEngineVersion(name); err == nil {
				return newUsageError("%s looks like a Godot version, so `godai engine install %s` would clash with it; pick another name", name, name)
			}

			if _, err := core.ResolveGodotExecutable(path); err != nil {
				return newUsageError("invalid Godot path: %v", err)
			}

			return withSession(ctx, cmd, configPath, func(session *core.Session) error {
				engine, err := session.LinkEngine(ctx, name, path)
				if err != nil {
					return err
				}

				reported := ""
				if engine.ReportedVersion != "" {
					reported = fmt.Sprintf(" (Godot %s)", engine.ReportedVersion)
				}
				printer(cmd).Printf("Linked %s to %s%s\n", name, engine.Path, reported)
				return nil
			})
		},
	}
}

func whichEngine(ctx context.Context, cmd *cli.Command, session *core.Session) (string, error) {
	if name := firstVersionArg(cmd); name != "" {
		engine, err := session.FindEngine(name)
		if err != nil {
			return "", err
		}
		return engine.Path, nil
	}

	projectPath, err := core.ResolveProjectPath("")
	if err != nil {
		// Outside a project there's no pin to honour, which is not a problem.
		projectPath = ""
	}

	return session.GodotExecutable(ctx, projectPath, core.EngineOptions{})
}

// We assume the first argument is a version if it doesn't start with a dash,
// because all Godot's builtin arguments start with a dash.
func firstVersionArg(cmd *cli.Command) string {
	first := cmd.Args().First()
	if strings.HasPrefix(first, "-") {
		return ""
	}
	return first
}

func godotArgs(cmd *cli.Command) []string {
	args := cmd.Args().Slice()
	if firstVersionArg(cmd) != "" {
		return args[1:]
	}
	return args
}

func installTemplatesIfNeeded(ctx context.Context, cmd *cli.Command, manager *godot.EngineManager, version godot.EngineVersion) error {
	templates, err := manager.Templates(version)
	if err != nil {
		return err
	}

	switch templates.State() {
	case godot.TemplatesAll:
		return nil
	case godot.TemplatesPartial:
		printer(cmd).Note("the export templates for %s are already there for %s; `godai engine install-templates %s --force` replaces them with the full set",
			version, strings.Join(templates.Platforms(), ", "), version)
		return nil
	}

	return installTemplates(ctx, cmd, manager, version)
}

func installTemplates(ctx context.Context, cmd *cli.Command, manager *godot.EngineManager, version godot.EngineVersion) error {
	out := printer(cmd)
	out.Printf("Installing the export templates for Godot %s...\n", out.Paint(output.Cyan, version.String()))

	if err := manager.InstallTemplates(ctx, version, downloadOptions(cmd)); err != nil {
		return err
	}

	out.Printf("Installed the export templates to %s\n", manager.TemplatesPath(version))
	return nil
}

func engineManager(cmd *cli.Command, configPath string) (*godot.EngineManager, error) {
	if err := setupLogging(cmd); err != nil {
		return nil, err
	}
	return core.NewEngineManager(configPath)
}

func engineArgs(cmd *cli.Command, configPath string) (*godot.EngineManager, godot.EngineVersion, error) {
	version, err := versionArg(cmd)
	if err != nil {
		return nil, godot.EngineVersion{}, err
	}

	manager, err := engineManager(cmd, configPath)
	if err != nil {
		return nil, godot.EngineVersion{}, err
	}

	return manager, version, nil
}

func versionArg(cmd *cli.Command) (godot.EngineVersion, error) {
	name, err := oneArg(cmd, "version")
	if err != nil {
		return godot.EngineVersion{}, err
	}

	version, err := godot.ParseEngineVersion(name)
	if err != nil {
		return godot.EngineVersion{}, newUsageError("%v", err)
	}
	return version, nil
}

func oneArg(cmd *cli.Command, what string) (string, error) {
	if cmd.Args().Len() != 1 {
		return "", newUsageError("expected one %s", what)
	}
	return cmd.Args().First(), nil
}

func allowUnverifiedFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "allow-unverified",
		Usage: "install the download without checking it against the published checksums",
	}
}

func downloadOptions(cmd *cli.Command) godot.DownloadOptions {
	return godot.DownloadOptions{
		Progress:        downloadProgress(cmd),
		AllowUnverified: cmd.Bool("allow-unverified"),
		Replace:         cmd.Bool("force"),
	}
}

func engineLabel(listing core.EngineListing) string {
	switch {
	case !listing.Linked:
		return listing.Version
	case listing.ReportedVersion == "":
		return listing.Version + " (linked)"
	default:
		return fmt.Sprintf("%s (linked: %s)", listing.Version, listing.ReportedVersion)
	}
}

func templatesCell(state godot.TemplateState) string {
	if state == "" {
		return "-"
	}
	return string(state)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func defaultMark(b bool) string {
	if b {
		return "*"
	}
	return ""
}

func runEngine(ctx context.Context, cmd *cli.Command, path string, args []string) error {
	godotCmd := exec.CommandContext(ctx, path, args...)
	godotCmd.Stdin = os.Stdin
	godotCmd.Stdout = os.Stdout
	godotCmd.Stderr = os.Stderr
	godotCmd.Env = append(os.Environ(), "DISPLAY="+cmd.String("x11-display"))

	if err := godotCmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return propagatedExit{exitErr.ExitCode()}
		}
		return err
	}

	return nil
}

func installReporter(cmd *cli.Command) core.InstallReporter {
	return func(version string) godot.Progress {
		printer(cmd).Note("Godot %s isn't installed yet, so Godai is downloading it", version)
		return downloadProgress(cmd)
	}
}

func downloadProgress(cmd *cli.Command) godot.Progress {
	if cmd.Bool("json") || !term.IsTerminal(int(os.Stderr.Fd())) {
		return nil
	}

	last := time.Time{}
	return func(downloaded, total int64) {
		finished := total > 0 && downloaded >= total
		if !finished && time.Since(last) < progressInterval {
			return
		}
		last = time.Now()

		if total > 0 {
			fmt.Fprintf(os.Stderr, "\r  %s of %s (%d%%)  ",
				humanBytes(downloaded), humanBytes(total), downloaded*100/total)
		} else {
			fmt.Fprintf(os.Stderr, "\r  %s  ", humanBytes(downloaded))
		}

		if finished {
			fmt.Fprintln(os.Stderr)
		}
	}
}

func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	value, exp := float64(n)/unit, 0
	for value >= unit && exp < 3 {
		value /= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", value, "kMGT"[exp])
}
