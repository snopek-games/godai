package eval

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

const SolutionModel = "solution"

const PristineModel = "pristine"
const PristineSurface = SurfaceNone

// solution.sh drives the CLI, so an oracle lands in the same cell a CLI model run will.
const SolutionSurface = SurfaceCLI

func runSolution(ctx context.Context, spec *Spec, work *Workspace) agentRun {
	m := &RunMetrics{Model: SolutionModel}

	shim, err := work.GodaiShim()
	if err != nil {
		return agentRun{metrics: m, code: -1, errs: []error{fmt.Errorf("install godai shim: %w", err)}}
	}

	// The script runs from the scratch project, so a task directory given
	// relative to the repo would resolve against the wrong place.
	script, err := filepath.Abs(spec.SolutionPath())
	if err != nil {
		return agentRun{metrics: m, code: -1, errs: []error{err}}
	}

	cmd := exec.CommandContext(ctx, script, shim, work.Project)
	cmd.Dir = work.Project
	cmd.Env = work.Env()
	// The isolated workspace has no installed engines, so a script that
	// opens a project needs to be told which Godot to use.
	if work.cfg.GodotBin != "" {
		cmd.Env = append(cmd.Env, "GODOT="+work.cfg.GodotBin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if work.cfg.Verbose {
		cmd.Env = append(cmd.Env, echoCommandsEnv+"=1")
		cmd.Stdout = io.MultiWriter(&stdout, work.cfg.LiveOut)
		cmd.Stderr = io.MultiWriter(&stderr, work.cfg.LiveOut)
	}

	code, runErr := runProcessGroup(ctx, cmd)

	var errs []error
	switch {
	case runErr != nil && ctx.Err() != nil:
		errs = append(errs, fmt.Errorf("timeout after %ds", spec.TimeoutSec))
	case runErr != nil:
		errs = append(errs, fmt.Errorf("solution.sh exit %d: %s", code, tail(stderr.String(), 500)))
	}

	calls, err := work.ShimCalls()
	if err != nil {
		errs = append(errs, fmt.Errorf("read shim log: %w", err))
	}
	m.ToolCalls = calls
	m.NumTurns = len(calls)

	return agentRun{metrics: m, code: code, errs: errs}
}
