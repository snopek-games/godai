package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	SurfaceMCP    = "mcp"
	SurfaceCLI    = "cli"
	SurfaceEditor = "editor"

	SurfaceNone = "none"
)

var Surfaces = []string{SurfaceMCP, SurfaceCLI, SurfaceEditor, SurfaceNone}

func KnownSurface(name string) bool {
	return slices.Contains(Surfaces, name)
}

// Config is one matrix cell plus the environment it runs in.
type Config struct {
	Model     string // opus | sonnet | haiku | a full model id
	Surface   string // mcp | none
	Repeats   int
	GodotBin  string
	ClaudeBin string
	GodaiBin  string
	WorkRoot  string
	KeepWork  bool

	// Concurrency is how many attempts run at once; it sizes the MCP port
	// window their editors share.
	Concurrency int

	// OpenTimeout, when set, becomes GODAI_OPEN_TIMEOUT for every godai the
	// attempts run, since the stock default undershoots on a loaded machine.
	OpenTimeout time.Duration

	// Bare runs Claude Code with --bare, which skips hooks, plugins, CLAUDE.md
	// and auto-memory. It also refuses OAuth, so it needs ANTHROPIC_API_KEY.
	Bare bool

	// Solution drives each task with its own solution.sh instead of a model.
	Solution bool

	// Pristine runs no agent at all: the editor opens, fixture plugins fabricate
	// their state, and verification runs against the untouched project. A task
	// that passes is one whose verifiers check nothing.
	Pristine bool

	// FullTools restores the tools the surface holds back by default: Bash on
	// none; Write and Edit on cli; all three on mcp. The editor surface has no
	// equivalent. See baselineArgs, cliArgs and mcpArgs.
	FullTools bool

	// TranscriptDir collects one conversation per attempt; empty writes none.
	TranscriptDir string

	// Verbose narrates each conversation to LiveOut as it happens.
	Verbose bool
	LiveOut io.Writer
}

// Attempt is one (task, repeat) run, and the record everything else aggregates.
type Attempt struct {
	TaskID  string `json:"task_id"`
	TaskRev string `json:"task_rev"`
	Model   string `json:"model"`
	Surface string `json:"surface"`
	Repeat  int    `json:"repeat"`

	Passed       bool          `json:"passed"`
	ChecksPassed int           `json:"checks_passed"`
	ChecksTotal  int           `json:"checks_total"`
	FailedChecks []FailedCheck `json:"failed_checks,omitempty"`
	VerifyLog    string        `json:"verify_log,omitempty"`

	Metrics *RunMetrics `json:"metrics"`
	Actions ActionStats `json:"actions"`

	ExitCode  int       `json:"exit_code"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

type FailedCheck struct {
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

func (f *FailedCheck) UnmarshalJSON(blob []byte) error {
	if err := json.Unmarshal(blob, &f.Name); err == nil {
		return nil
	}
	type plain FailedCheck
	return json.Unmarshal(blob, (*plain)(f))
}

func (f FailedCheck) String() string {
	if f.Detail == "" {
		return f.Name
	}
	return f.Name + ": " + f.Detail
}

// PartialCredit has far more resolution than Passed, so prefer it when judging
// small harness changes.
func (a *Attempt) PartialCredit() float64 {
	if a.ChecksTotal == 0 {
		return 0
	}
	return float64(a.ChecksPassed) / float64(a.ChecksTotal)
}

func (a *Attempt) addError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if a.Error == "" {
		a.Error = msg
	} else {
		a.Error += "; " + msg
	}
}

func RunAttempt(ctx context.Context, cfg Config, spec *Spec, repeat int) *Attempt {
	att := &Attempt{
		TaskID: spec.ID, TaskRev: spec.Rev, Model: cfg.Model,
		Surface: cfg.Surface, Repeat: repeat, StartedAt: time.Now(),
	}

	// Deferred, so that an attempt that returns early on a harness error still gets written.
	var stream []byte
	defer func() {
		if err := WriteTranscript(cfg.TranscriptDir, spec, att, stream); err != nil {
			att.addError("write transcript: %v", err)
		}
	}()

	work, err := StageWorkspace(ctx, cfg, spec, repeat)
	if work != nil {
		defer work.Cleanup(context.WithoutCancel(ctx))
	}
	if err != nil {
		att.addError("stage workspace: %v", err)
		return att
	}

	// The editor surface opens its own editor: the addon's prompt arrives in
	// environment variables, which have to be there at startup.
	if cfg.Surface != SurfaceEditor {
		if err := work.OpenEditor(ctx); err != nil {
			att.addError("%v", err)
			return att
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	run := runAgent(runCtx, cfg, spec, work)
	wall := time.Since(start)
	att.ExitCode = run.code
	stream = run.stream

	metrics := run.metrics
	metrics.WallTime, metrics.WallTimeMS = wall, wall.Milliseconds()
	att.Metrics = metrics

	// A timed-out or budget-capped run is a real failure with real metrics
	// attached, and partial work still scores partial credit.
	for _, err := range run.errs {
		att.addError("%v", err)
	}

	// An editor the agent wrongly closed still has to fail these checks, so an
	// error is reported as a failed check rather than aborting the attempt.
	var editorReport *VerifyReport
	if spec.HasEditorVerify() {
		var err error
		if editorReport, err = VerifyEditor(ctx, work, spec); err != nil {
			att.addError("editor verify: %v", err)
			editorReport = &VerifyReport{ChecksTotal: 1,
				Checks: []VerifyCheck{{Name: "editor_verify", Detail: err.Error()}}}
		}
	}

	// Closing before verifying keeps a second Godot off the same project, and
	// the editor's unsaved state out of the result.
	if err := work.CloseEditor(context.WithoutCancel(ctx)); err != nil {
		att.addError("%v", err)
	}

	if cfg.Surface == SurfaceMCP {
		if err := checkMCPHealth(metrics); err != nil {
			att.addError("%v", err)
			return att
		}
	}

	att.Actions = ScoreActions(NormalizeActions(metrics.ToolCalls), spec)

	if cfg.Surface == SurfaceCLI {
		if err := checkShimAnswered(work, att.Actions.GodaiCalls+att.Actions.HelpCalls); err != nil {
			att.addError("%v", err)
			return att
		}
	}

	if dirty, err := work.DirtyPaths(spec.ProtectedPaths); err != nil {
		att.addError("protected paths: %v", err)
	} else if len(dirty) > 0 {
		att.addError("modified protected paths: %s", strings.Join(dirty, ", "))
		return att
	}

	if err := work.InstallVerifier(spec); err != nil {
		att.addError("install verifier: %v", err)
		return att
	}

	report, err := Verify(ctx, cfg.GodotBin, work.Project, work.Env())
	if err != nil {
		att.addError("verify: %v", err)
		return att
	}
	if editorReport != nil {
		report = MergeReports(editorReport, report)
	}
	att.Passed = report.Passed
	att.ChecksPassed = report.ChecksPassed
	att.ChecksTotal = report.ChecksTotal
	for _, c := range report.Checks {
		if !c.OK {
			att.FailedChecks = append(att.FailedChecks, FailedCheck{Name: c.Name, Detail: c.Detail})
		}
	}
	if !report.Passed {
		att.VerifyLog = report.Log
	}
	return att
}

type agentRun struct {
	metrics *RunMetrics
	stream  []byte // empty on the solution surface, which has no model to record
	code    int
	errs    []error
}

type surface struct {
	args []string
	env  []string
}

func surfaceFor(cfg Config, work *Workspace) (surface, error) {
	switch cfg.Surface {
	case SurfaceMCP:
		return surface{args: mcpArgs(cfg, work)}, nil
	case SurfaceCLI:
		return cliSurface(cfg, work)
	case SurfaceNone:
		return surface{args: baselineArgs(cfg.FullTools)}, nil
	}
	return surface{}, fmt.Errorf("unknown surface %q", cfg.Surface)
}

func runAgent(ctx context.Context, cfg Config, spec *Spec, work *Workspace) agentRun {
	if cfg.Pristine {
		return agentRun{metrics: &RunMetrics{Model: PristineModel}}
	}
	if cfg.Solution {
		return runSolution(ctx, spec, work)
	}
	if cfg.Surface == SurfaceEditor {
		return runEditorAgent(ctx, cfg, spec, work)
	}

	s, err := surfaceFor(cfg, work)
	if err != nil {
		return agentRun{metrics: &RunMetrics{}, errs: []error{err}}
	}

	stream, code, runErr := runClaudeCode(ctx, cfg, spec, work, s)
	metrics, parseErr := ParseStream(bytes.NewReader(stream))

	var errs []error
	if parseErr != nil {
		errs = append(errs, fmt.Errorf("parse stream: %w", parseErr))
	}
	if runErr != nil {
		errs = append(errs, runErr)
	}
	return agentRun{metrics: metrics, stream: stream, code: code, errs: errs}
}

// mcpArgs reaches godai only as MCP tools. --strict-mcp-config keeps whatever
// MCP servers the developer has configured from changing the result.
func mcpArgs(cfg Config, work *Workspace) []string {
	env := map[string]string{}
	for _, kv := range work.IsolationEnv() {
		name, value, _ := strings.Cut(kv, "=")
		env[name] = value
	}

	server := map[string]any{
		"mcpServers": map[string]any{
			"godai": map[string]any{
				"command": cfg.GodaiBin,
				// --headless --auto-approve so an editor the agent opens works
				// like the one the harness opened: no display, nobody to
				// answer approval dialogs.
				"args": work.GodaiArgs("mcp", "--no-update-check", "--headless", "--auto-approve"),
				"env":  env,
			},
		},
	}
	blob, _ := json.Marshal(server)
	args := []string{
		"--mcp-config", string(blob),
		"--strict-mcp-config",
	}
	if cfg.FullTools {
		return append(args, "--allowedTools", "mcp__godai,Read,Write,Edit,Glob,Grep,Bash")
	}
	// Bash needs the explicit deny: in dontAsk mode read-only commands slip
	// through otherwise, teaching the model that shelling out half-works.
	return append(args,
		"--allowedTools", "mcp__godai,Read,Glob,Grep",
		"--disallowedTools", "Bash",
	)
}

// A baseline that can shell out to godai measures godai, hence the default deny.
func baselineArgs(fullTools bool) []string {
	args := []string{
		"--mcp-config", `{"mcpServers":{}}`,
		"--strict-mcp-config",
	}
	tools := "Read,Write,Edit,Glob,Grep"
	if fullTools {
		tools += ",Bash"
	} else {
		args = append(args, "--disallowedTools", "Bash")
	}
	return append(args, "--allowedTools", tools)
}

func runClaudeCode(ctx context.Context, cfg Config, spec *Spec, work *Workspace, s surface) ([]byte, int, error) {
	args := []string{
		"-p", spec.Instruction,
		"--model", cfg.Model,
		"--permission-mode", "dontAsk",
		"--max-budget-usd", fmt.Sprintf("%.2f", spec.MaxBudgetUSD),
		"--output-format", "stream-json",
		"--verbose",
		"--disable-slash-commands",
		"--setting-sources", "",
	}
	if cfg.Bare {
		args = append(args, "--bare")
	}
	args = append(args, s.args...)

	cmd := exec.CommandContext(ctx, cfg.ClaudeBin, args...)
	cmd.Dir = work.Project
	env := os.Environ()
	if !cfg.Bare {
		// claude -p prefers ANTHROPIC_API_KEY over the host login when it's set.
		env = slices.DeleteFunc(env, func(v string) bool { return strings.HasPrefix(v, "ANTHROPIC_API_KEY=") })
	}
	cmd.Env = append(env, "GODAI_PROJECT_PATH="+work.Project)
	cmd.Env = append(cmd.Env, s.env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if cfg.Verbose {
		cmd.Stdout = io.MultiWriter(&stdout, newLiveWriter(cfg.LiveOut, spec, work.Repeat))
	}

	code, err := runProcessGroup(ctx, cmd)
	switch {
	case err != nil && ctx.Err() != nil:
		return stdout.Bytes(), code, fmt.Errorf("timeout after %ds", spec.TimeoutSec)
	case err != nil:
		return stdout.Bytes(), code, fmt.Errorf("claude exit %d: %s", code, tail(stderr.String(), 500))
	}
	return stdout.Bytes(), code, nil
}

// Runs cmd in its own process group, so a timeout takes the Godot children with
// it rather than leaving them behind.
func runProcessGroup(ctx context.Context, cmd *exec.Cmd) (int, error) {
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return -1, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return cmd.ProcessState.ExitCode(), err
	case <-ctx.Done():
		killProcessGroup(cmd.Process.Pid)
		<-done
		return -1, ctx.Err()
	}
}

// Catches the silent-degradation case: with no godai tools, Claude Code
// solves the task by editing files and the run still looks like a pass.
func checkMCPHealth(m *RunMetrics) error {
	if len(m.MCPErrors) > 0 {
		return fmt.Errorf("godai MCP server failed to load: %+v", m.MCPErrors)
	}
	for _, s := range m.MCPServersLoaded {
		if s.Name != "godai" {
			continue
		}
		if strings.EqualFold(s.Status, "connected") {
			return nil
		}
		return fmt.Errorf("godai MCP server status %q", s.Status)
	}
	return fmt.Errorf("godai MCP server absent from system/init")
}
