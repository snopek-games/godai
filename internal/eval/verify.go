package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const verifySentinel = "GODAI_VERIFY_JSON:"

type VerifyReport struct {
	Passed       bool          `json:"passed"`
	ChecksPassed int           `json:"checks_passed"`
	ChecksTotal  int           `json:"checks_total"`
	Checks       []VerifyCheck `json:"checks"`

	// Stderr output from Godot.
	Log string `json:"-"`
}

type VerifyCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Verify runs the task's verify.gd against the project the agent left behind.
// That is GDScript an LLM just wrote, so treat it as untrusted code.
// env should be the workspace's, so checks against editor state (settings,
// caches) read the run's isolated XDG dirs rather than the developer's.
func Verify(ctx context.Context, godotBin, project string, env []string) (*VerifyReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, godotBin,
		"--headless", "--path", project, "--script", "res://verify/verify.gd")
	cmd.Env = env
	setProcessGroup(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	// The verifier exits non-zero on a failing task, so a run error only
	// matters when we also got no report back.
	rep, err := reportFromLines(strings.Split(stdout.String(), "\n"))
	if err != nil {
		return nil, err
	}
	if rep == nil {
		return nil, fmt.Errorf("verifier produced no report (err=%v): %s",
			runErr, tail(stderr.String()+stdout.String(), 800))
	}
	rep.Log = tail(strings.TrimSpace(stderr.String()), 2000)
	return rep, nil
}

func reportFromLines(lines []string) (*VerifyReport, error) {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, verifySentinel) {
			continue
		}
		var rep VerifyReport
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, verifySentinel)), &rep); err != nil {
			return nil, fmt.Errorf("bad verifier JSON: %w", err)
		}
		return &rep, nil
	}
	return nil, nil
}

// VerifyEditor runs the task's editor_verify.gd inside the editor the agent
// worked in, which is why it has to run before that editor closes.
func VerifyEditor(ctx context.Context, w *Workspace, spec *Spec) (*VerifyReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	// godai runs from the workspace root, so the task path has to be absolute.
	script, err := filepath.Abs(spec.EditorVerifyPath())
	if err != nil {
		return nil, err
	}
	out, err := w.godaiJSON(ctx, "editor-tool", "execute_editor_script",
		"--arg-file", "code="+script, "--project-path", w.Project)
	if err != nil {
		return nil, fmt.Errorf("execute editor_verify.gd: %w", err)
	}

	var run struct {
		Success bool     `json:"success"`
		Output  []string `json:"output"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(out), &run); err != nil {
		return nil, fmt.Errorf("bad editor-tool JSON: %w: %s", err, tail(out, 800))
	}
	if len(run.Errors) > 0 {
		return nil, fmt.Errorf("editor_verify.gd: %s", strings.Join(run.Errors, "; "))
	}
	if !run.Success {
		return nil, fmt.Errorf("editor_verify.gd failed: %s", tail(strings.Join(run.Output, "\n"), 800))
	}

	rep, err := reportFromLines(run.Output)
	if err != nil {
		return nil, err
	}
	if rep == nil {
		return nil, fmt.Errorf("editor_verify.gd produced no report: %s",
			tail(strings.Join(run.Output, "\n"), 800))
	}
	return rep, nil
}

func MergeReports(reports ...*VerifyReport) *VerifyReport {
	merged := &VerifyReport{Passed: true}
	var logs []string
	for _, rep := range reports {
		merged.Passed = merged.Passed && rep.Passed
		merged.ChecksPassed += rep.ChecksPassed
		merged.ChecksTotal += rep.ChecksTotal
		merged.Checks = append(merged.Checks, rep.Checks...)
		if rep.Log != "" {
			logs = append(logs, rep.Log)
		}
	}
	merged.Log = strings.Join(logs, "\n")
	return merged
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
