package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"gitlab.com/snopek-games/godai/internal/eval"

	"github.com/urfave/cli/v3"
)

func runCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "run the eval tasks against a model and a godai surface",
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:  "model",
				Usage: "model alias or id",
				Value: "sonnet",
			},
			&cli.StringFlag{
				Name:  "surface",
				Usage: "the godai surface under test: mcp, cli, editor, or none",
				Value: eval.SurfaceMCP,
			},
			&cli.StringFlag{
				Name:  "out",
				Usage: "where to write the results JSON",
				Value: "results.json",
			},
			&cli.StringFlag{
				Name:  "transcripts",
				Usage: "directory for the per-attempt conversations; defaults to <out>-transcripts, empty writes none",
			},
			&cli.BoolFlag{
				Name:  "solution",
				Usage: "drive each task with its solution.sh instead of a model, to check that the oracle still solves it",
			},
			&cli.BoolFlag{
				Name:  "pristine",
				Usage: "run no agent at all and expect every task to fail: a task that passes untouched verifies nothing",
			},
		}, sharedFlags()...),
		Action: run,
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Present() {
		return fmt.Errorf("unexpected arguments: %v", cmd.Args().Slice())
	}

	solution := cmd.Bool("solution")
	pristine := cmd.Bool("pristine")
	surface := cmd.String("surface")
	if solution && pristine {
		return fmt.Errorf("--solution and --pristine are mutually exclusive")
	}
	if pristine && cmd.IsSet("surface") {
		return fmt.Errorf("--pristine runs no agent, so there is no surface to pick")
	}

	credentialed := []string{surface}
	if solution || pristine {
		credentialed = nil
	}
	if err := checkCredentials(cmd.Bool("bare"), credentialed); err != nil {
		return err
	}

	specs, err := eval.LoadTasks(cmd.String("tasks"), cmd.StringSlice("tags"), cmd.StringSlice("ids"))
	if err != nil {
		return err
	}
	switch {
	case solution:
		if cmd.IsSet("surface") && surface != eval.SolutionSurface {
			return fmt.Errorf("--solution drives the %s surface, not %s", eval.SolutionSurface, surface)
		}
		for _, spec := range specs {
			if _, err := os.Stat(spec.SolutionPath()); err != nil {
				return fmt.Errorf("--solution: %s: %w", spec.ID, err)
			}
		}
	case pristine:
	default:
		if !eval.KnownSurface(surface) {
			return fmt.Errorf("unknown surface %q: try %s", surface, strings.Join(eval.Surfaces, ", "))
		}
	}

	fullTools := cmd.Bool("full-tools")
	if err := checkFullTools(fullTools, []string{surface}); err != nil {
		return err
	}

	h, err := newHarness(cmd)
	if err != nil {
		return err
	}
	defer h.close()

	model := cmd.String("model")
	if solution {
		model, surface = eval.SolutionModel, eval.SolutionSurface
	}
	if pristine {
		model, surface = eval.PristineModel, eval.PristineSurface
	}

	out := cmd.String("out")
	transcripts := cmd.String("transcripts")
	if !cmd.IsSet("transcripts") {
		transcripts = defaultTranscriptDir(out)
	}
	if transcripts != "" {
		log.Printf("transcripts -> %s/", transcripts)
	}

	cfg := h.config(cmd)
	cfg.Model = model
	cfg.Surface = surface
	cfg.Solution = solution
	cfg.Pristine = pristine
	cfg.FullTools = fullTools
	cfg.TranscriptDir = transcripts

	if pristine {
		log.Printf("pristine: running %d tasks with no agent; every one of them SHOULD fail", len(specs))
	}

	results, err := runCell(ctx, cfg, specs, out)
	if err != nil {
		return err
	}

	if solution {
		if failed := failedTasks(results.Attempts); len(failed) > 0 {
			return fmt.Errorf("solutions failed: %s", strings.Join(failed, ", "))
		}
		return nil
	}

	if pristine {
		if vacuous := passedTasks(results.Attempts); len(vacuous) > 0 {
			return fmt.Errorf("tasks passed with no agent; this means their verifiers don't check anything: %s", strings.Join(vacuous, ", "))
		}
		log.Printf("pristine: all %d tasks failed with no agent; this means all verifiers do check something", len(specs))
		return nil
	}

	if s := results.Summary; s.HarnessErrs == s.Trials && s.Trials > 0 {
		return fmt.Errorf("every attempt errored: the harness or the credentials are broken")
	}
	return nil
}

func failedTasks(attempts []*eval.Attempt) []string {
	return tasksWhere(attempts, func(a *eval.Attempt) bool { return !a.Passed })
}

func passedTasks(attempts []*eval.Attempt) []string {
	return tasksWhere(attempts, func(a *eval.Attempt) bool { return a.Passed })
}

func tasksWhere(attempts []*eval.Attempt, match func(*eval.Attempt) bool) []string {
	var ids []string
	seen := map[string]bool{}
	for _, a := range attempts {
		if match(a) && !seen[a.TaskID] {
			seen[a.TaskID] = true
			ids = append(ids, a.TaskID)
		}
	}
	return ids
}
