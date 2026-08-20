package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gitlab.com/snopek-games/godai/internal/eval"

	"github.com/urfave/cli/v3"
)

func sharedFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:  "repeats",
			Usage: "repeats per task",
			Value: 1,
		},
		&cli.StringFlag{
			Name:  "tasks",
			Usage: "directory holding the task definitions",
			Value: "tests/eval/tasks",
		},
		&cli.StringSliceFlag{
			Name:  "tags",
			Usage: "only run tasks with one of these tags",
		},
		&cli.StringSliceFlag{
			Name:  "ids",
			Usage: "only run these task ids",
		},
		&cli.IntFlag{
			Name:  "concurrency",
			Usage: "how many attempts to run at once",
			Value: 1,
		},
		&cli.FloatFlag{
			Name:  "open-timeout",
			Usage: "how long (in seconds) godai waits for a launched editor to import and connect; raise it with high --concurrency",
		},
		&cli.StringFlag{
			Name:  "godot",
			Usage: "path to a Godot executable, instead of the one godai would pick",
		},
		&cli.StringFlag{
			Name:  "claude",
			Usage: "path to the Claude Code executable",
			Value: "claude",
		},
		&cli.StringFlag{
			Name:  "godai",
			Usage: "path to a godai binary, instead of building ./cmd/godai",
		},
		&cli.StringFlag{
			Name:  "work",
			Usage: "root for the scratch directories (default: the system temp dir)",
		},
		&cli.BoolFlag{
			Name:  "keep-work",
			Usage: "keep the scratch directories, to poke at after a failure",
		},
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "narrate each conversation as it happens",
		},
		&cli.BoolFlag{
			Name:  "full-tools",
			Usage: "restore the held-back tools: Bash on none; Write and Edit on cli; all three on mcp",
		},
		&cli.BoolFlag{
			Name:  "bare",
			Usage: "run Claude Code with --bare, ignoring host hooks, plugins and CLAUDE.md (needs ANTHROPIC_API_KEY)",
		},
	}
}

func checkCredentials(bare bool, surfaces []string) error {
	if bare && os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("--bare needs ANTHROPIC_API_KEY, from the environment or %s: it never reads OAuth or the keychain", envFile)
	}
	if slices.Contains(surfaces, eval.SurfaceEditor) && os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("the %s surface needs ANTHROPIC_API_KEY, from the environment or %s: the addon calls the API itself", eval.SurfaceEditor, envFile)
	}
	claudeCode := slices.ContainsFunc(surfaces, func(s string) bool { return s != eval.SurfaceEditor })
	if !bare && claudeCode {
		log.Print("running without --bare: your hooks, plugins and CLAUDE.md are in play, so results are not reproducible across machines")
	}
	return nil
}

func checkFullTools(fullTools bool, surfaces []string) error {
	if !fullTools {
		return nil
	}
	supported := slices.ContainsFunc(surfaces, func(s string) bool { return s != eval.SurfaceEditor })
	if !supported {
		return fmt.Errorf("--full-tools does not apply to the %s surface, which runs godai's own agent",
			eval.SurfaceEditor)
	}
	if path, err := exec.LookPath("godai"); err == nil {
		log.Printf("--full-tools with godai at %s: the agent can shell out to it, so a cell measures more than its own surface", path)
	}
	return nil
}

type harness struct {
	godai string
	godot string
	built bool
}

func newHarness(cmd *cli.Command) (*harness, error) {
	h := &harness{godai: cmd.String("godai"), godot: cmd.String("godot")}

	if h.godai == "" {
		bin, err := buildGodai()
		if err != nil {
			return nil, err
		}
		h.godai, h.built = bin, true
	}
	if h.godot == "" {
		godot, err := resolveGodot(h.godai)
		if err != nil {
			h.close()
			return nil, err
		}
		h.godot = godot
	}

	log.Printf("godai=%s godot=%s", h.godai, h.godot)
	return h, nil
}

func (h *harness) close() {
	if h.built {
		os.Remove(h.godai)
	}
}

func (h *harness) config(cmd *cli.Command) eval.Config {
	return eval.Config{
		Repeats:     int(cmd.Int("repeats")),
		GodotBin:    h.godot,
		ClaudeBin:   cmd.String("claude"),
		GodaiBin:    h.godai,
		WorkRoot:    cmd.String("work"),
		KeepWork:    cmd.Bool("keep-work"),
		Bare:        cmd.Bool("bare"),
		Concurrency: int(cmd.Int("concurrency")),
		OpenTimeout: time.Duration(cmd.Float("open-timeout") * float64(time.Second)),
		Verbose:     cmd.Bool("verbose"),
		LiveOut:     os.Stderr,
	}
}

func runCell(ctx context.Context, cfg eval.Config, specs []*eval.Spec, out string) (*eval.Results, error) {
	started := time.Now()
	attempts := runAttempts(ctx, cfg, specs)

	results := &eval.Results{
		File: out, Model: cfg.Model, Surface: cfg.Surface, Repeats: cfg.Repeats, StartedAt: started,
	}
	for _, a := range attempts {
		if a != nil {
			results.Attempts = append(results.Attempts, a)
		}
	}
	results.Summary = eval.Summarize(results.Attempts)

	if err := eval.WriteJSON(out, results); err != nil {
		return nil, err
	}

	s := results.Summary
	log.Printf("pass %.0f%% (%d/%d) | partial %.2f | first-action %.2f | bypass %.2f | $%.2f -> %s",
		s.PassRate*100, s.Passes, s.Trials, s.PartialCredit,
		s.MeanFirstActionCorrect, s.MeanBypassRate, s.TotalCostUSD, out)

	return results, nil
}

// Returns whatever finished, so cancelling partway still writes results.
func runAttempts(ctx context.Context, cfg eval.Config, specs []*eval.Spec) []*eval.Attempt {
	type job struct {
		spec   *eval.Spec
		repeat int
	}
	var jobs []job
	for _, s := range specs {
		for r := 1; r <= cfg.Repeats; r++ {
			jobs = append(jobs, job{s, r})
		}
	}

	concurrency := max(cfg.Concurrency, 1)
	log.Printf("%s/%s: %d tasks x %d repeats = %d attempts (concurrency %d)",
		cfg.Model, cfg.Surface, len(specs), cfg.Repeats, len(jobs), concurrency)

	attempts := make([]*eval.Attempt, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i, j := range jobs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			a := eval.RunAttempt(ctx, cfg, j.spec, j.repeat)
			attempts[i] = a

			mu.Lock()
			defer mu.Unlock()
			done++
			log.Printf("[%d/%d] %s", done, len(jobs), describe(a, j.spec))
		})
	}
	wg.Wait()

	return attempts
}

func describe(a *eval.Attempt, spec *eval.Spec) string {
	status := "FAIL"
	if a.Passed {
		status = "PASS"
	}
	extra := ""
	if a.Metrics != nil {
		extra = fmt.Sprintf(" turns=%d tok=%d %.0fs $%.3f",
			a.Metrics.NumTurns, a.Metrics.TotalTokens,
			a.Metrics.WallTime.Seconds(), a.Metrics.TotalCostUSD)
	}
	return fmt.Sprintf("%s %s r%d (%d/%d checks)%s %s",
		status, spec.ID, a.Repeat, a.ChecksPassed, a.ChecksTotal, extra, a.Error)
}

// Derived from the results path, so two runs in one directory don't collide.
func defaultTranscriptDir(out string) string {
	base := filepath.Base(out)
	return filepath.Join(filepath.Dir(out), strings.TrimSuffix(base, filepath.Ext(base))+"-transcripts")
}

// Builds the working tree, so an eval measures your code and not whatever
// godai is on PATH.
func buildGodai() (string, error) {
	bin := filepath.Join(os.TempDir(), fmt.Sprintf("godai-eval-bin-%d", os.Getpid()))
	build := exec.Command("go", "build", "-tags", "selfupdate", "-o", bin, "./cmd/godai")
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build godai: %v: %s", err, out)
	}
	return bin, nil
}

func resolveGodot(godaiBin string) (string, error) {
	out, err := exec.Command(godaiBin, "engine", "which").Output()
	if err != nil {
		return "", fmt.Errorf("no --godot given and `godai engine which` failed: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("no --godot given and `godai engine which` found nothing")
	}
	return path, nil
}
