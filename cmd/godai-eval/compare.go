package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gitlab.com/snopek-games/godai/internal/eval"

	"github.com/urfave/cli/v3"
)

func compareCommand() *cli.Command {
	return &cli.Command{
		Name:        "compare",
		Usage:       "compare results from different models and surfaces",
		ArgsUsage:   "<results.json|dir>...",
		Description: "Reads any number of results files, merges the ones from the same model and surface, and reports them as a matrix. Give it a directory to read every results file in it.",
		Flags: []cli.Flag{
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
			&cli.StringFlag{
				Name:  "out",
				Usage: "also write the report to this file",
			},
		},
		Action: runCompare,
	}
}

func runCompare(_ context.Context, cmd *cli.Command) error {
	paths := cmd.Args().Slice()
	if len(paths) == 0 {
		return fmt.Errorf("nothing to compare: name some results files or a directory of them")
	}

	results, err := eval.LoadResults(paths)
	if err != nil {
		return err
	}
	baseline, err := eval.LoadResults(cmd.StringSlice("baseline"))
	if err != nil {
		return fmt.Errorf("baseline: %w", err)
	}

	comparison := eval.NewComparison(results, baseline, cmd.Float("alpha"))
	return reportComparison(comparison, cmd.String("out"), cmd.Bool("fail-on-regression"))
}

func reportComparison(c *eval.Comparison, out string, failOnRegression bool) error {
	report := c.Markdown()
	fmt.Print(report)

	if out != "" {
		if err := os.WriteFile(out, []byte(report), 0o644); err != nil {
			return err
		}
	}

	if regressed := c.Regressions(); len(regressed) > 0 && failOnRegression {
		return fmt.Errorf("regressed against the baseline: %s", strings.Join(keyNames(regressed), ", "))
	}
	return nil
}

func keyNames(keys []eval.CellKey) []string {
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, k.String())
	}
	return names
}
