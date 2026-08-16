package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gitlab.com/snopek-games/godai/internal/eval"

	"github.com/urfave/cli/v3"
)

func matrixCommand() *cli.Command {
	return &cli.Command{
		Name:        "matrix",
		Usage:       "run each model against every surface, then compare them",
		Description: "Runs one cell per model and surface, writing a results file for each, and prints the comparison at the end. Cells run one after another; --concurrency still runs the attempts within a cell simultaneously.",
		Flags: append([]cli.Flag{
			&cli.StringSliceFlag{
				Name:  "models",
				Usage: "models to run, as aliases or ids",
				Value: []string{"sonnet"},
			},
			&cli.StringSliceFlag{
				Name:  "surfaces",
				Usage: "surfaces to run each model against",
				Value: []string{eval.SurfaceMCP, eval.SurfaceNone},
			},
			&cli.StringFlag{
				Name:  "out-dir",
				Usage: "directory for the per-cell results files",
				Value: "results",
			},
			&cli.BoolFlag{
				Name:  "skip-existing",
				Usage: "keep the cells already in --out-dir, to pick up an interrupted matrix",
			},
			&cli.BoolFlag{
				Name:  "no-transcripts",
				Usage: "don't keep the per-attempt conversations",
			},
			&cli.StringFlag{
				Name:  "report",
				Usage: "also write the comparison to this file",
			},
			&cli.StringSliceFlag{
				Name:  "baseline",
				Usage: "results to measure against, adding a pass-rate delta and its p-value",
			},
			&cli.FloatFlag{
				Name:  "alpha",
				Usage: "how small a p-value counts as a real difference rather than noise",
				Value: 0.05,
			},
			&cli.BoolFlag{
				Name:  "fail-on-regression",
				Usage: "exit non-zero when a cell drops against its baseline by more than noise",
			},
		}, sharedFlags()...),
		Action: runMatrix,
	}
}

type cell struct{ model, surface string }

func runMatrix(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Present() {
		return fmt.Errorf("unexpected arguments: %v", cmd.Args().Slice())
	}

	models := cmd.StringSlice("models")
	surfaces := cmd.StringSlice("surfaces")
	if len(models) == 0 || len(surfaces) == 0 {
		return fmt.Errorf("a matrix needs at least one model and one surface")
	}
	if slices.Contains(models, "") {
		return fmt.Errorf("--models has an empty name in it: %q", strings.Join(models, ","))
	}
	for _, surface := range surfaces {
		if !eval.KnownSurface(surface) {
			return fmt.Errorf("unknown surface %q: try %s", surface, strings.Join(eval.Surfaces, ", "))
		}
	}

	var cells []cell
	for _, model := range models {
		for _, surface := range surfaces {
			cells = append(cells, cell{model, surface})
		}
	}

	outDir := cmd.String("out-dir")
	log.Printf("%d cells: %s x %s -> %s/", len(cells),
		strings.Join(models, ","), strings.Join(surfaces, ","), outDir)

	if err := checkCredentials(cmd.Bool("bare"), surfaces); err != nil {
		return err
	}
	fullTools := cmd.Bool("full-tools")
	if err := checkFullTools(fullTools, surfaces); err != nil {
		return err
	}

	specs, err := eval.LoadTasks(cmd.String("tasks"), cmd.StringSlice("tags"), cmd.StringSlice("ids"))
	if err != nil {
		return err
	}
	baseline, err := eval.LoadResults(cmd.StringSlice("baseline"))
	if err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	h, err := newHarness(cmd)
	if err != nil {
		return err
	}
	defer h.close()

	base := h.config(cmd)

	var results []*eval.Results
	for _, c := range cells {
		if ctx.Err() != nil {
			log.Print("interrupted: comparing the cells that finished")
			break
		}

		out := cellFile(outDir, c)
		if cmd.Bool("skip-existing") {
			if _, err := os.Stat(out); err == nil {
				log.Printf("%s/%s: keeping %s", c.model, c.surface, out)
				kept, err := eval.LoadResults([]string{out})
				if err != nil {
					return err
				}
				results = append(results, kept...)
				continue
			}
		}

		cfg := base
		cfg.Model = c.model
		cfg.Surface = c.surface
		cfg.FullTools = fullTools && c.surface != eval.SurfaceEditor
		if !cmd.Bool("no-transcripts") {
			cfg.TranscriptDir = defaultTranscriptDir(out)
		}

		r, err := runCell(ctx, cfg, specs, out)
		if err != nil {
			return err
		}
		results = append(results, r)
	}

	if len(results) == 0 {
		return fmt.Errorf("no cells ran")
	}

	comparison := eval.NewComparison(results, baseline, cmd.Float("alpha"))
	return reportComparison(comparison, cmd.String("report"), cmd.Bool("fail-on-regression"))
}

func cellFile(dir string, c cell) string {
	return filepath.Join(dir, fileSafe(c.model)+"-"+fileSafe(c.surface)+".json")
}

// Model ids carry colons and slashes.
func fileSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		}
		return '-'
	}, s)
}
