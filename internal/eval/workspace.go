package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gitlab.com/snopek-games/godai/internal/isolation"
)

// Everything the editor and the import cache generate is noise in the diff we
// use for protected-path checks.
const workspaceGitignore = ".godot/\naddons/godai/\nverify/\neditor.log\n"

type Workspace struct {
	Root    string
	Project string
	Repeat  int

	cfg          Config
	isolationEnv []string
	editorClosed bool
	closeErr     error
}

// The baseline is committed after the addon is installed, because enabling the
// addon edits project.godot and that would dirty a protected path.
func StageWorkspace(ctx context.Context, cfg Config, spec *Spec, repeat int) (*Workspace, error) {
	root := cfg.WorkRoot
	if root == "" {
		root = os.TempDir()
	}
	dir, err := os.MkdirTemp(root, fmt.Sprintf("godai-eval-%s-r%d-", spec.ID, repeat))
	if err != nil {
		return nil, err
	}
	// Resolve symlinks (macOS puts temp dirs behind /var -> /private/var) so
	// the workspace paths compare equal to paths godai canonicalizes.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	w := &Workspace{Root: dir, Project: filepath.Join(dir, "project"), Repeat: repeat, cfg: cfg}

	w.isolationEnv, err = isolation.Env(dir)
	if err != nil {
		return w, err
	}
	if err := copyTree(spec.FixtureDir(), w.Project); err != nil {
		return w, fmt.Errorf("copy fixture: %w", err)
	}
	gitignore := filepath.Join(w.Project, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(workspaceGitignore), 0o644); err != nil {
		return w, err
	}

	if err := w.InstallAddon(ctx); err != nil {
		return w, err
	}
	if err := w.commitBaseline(); err != nil {
		return w, err
	}
	return w, nil
}

func (w *Workspace) InstallAddon(ctx context.Context) error {
	out, err := w.godai(ctx, "project", "install-addon", w.Project)
	if err != nil {
		return fmt.Errorf("install addon: %w: %s", err, out)
	}
	return nil
}

func (w *Workspace) Env() []string {
	return append(os.Environ(), w.IsolationEnv()...)
}

// The window must fit one editor per concurrent attempt, plus slack for
// stragglers from earlier attempts and the developer's own stock-port editor.
const (
	mcpBasePort   = 12120
	mcpPortBuffer = 10
)

func (w *Workspace) IsolationEnv() []string {
	env := append(slices.Clone(w.isolationEnv),
		fmt.Sprintf("GODAI_MCP_BASE_PORT=%d", mcpBasePort),
		fmt.Sprintf("GODAI_MCP_PORT_COUNT=%d", max(w.cfg.Concurrency, 1)+mcpPortBuffer),
		// Editors log to the workspace cache dir, so a connection timeout
		// shows what Godot printed instead of nothing.
		"GODAI_EDITOR_LOG=1",
	)
	if w.cfg.OpenTimeout > 0 {
		env = append(env, fmt.Sprintf("GODAI_OPEN_TIMEOUT=%g", w.cfg.OpenTimeout.Seconds()))
	}
	return env
}

// Copied in only once the agent is done, so it never had the checks it is
// graded against sitting in its project.
func (w *Workspace) InstallVerifier(spec *Spec) error {
	return copyTree(spec.VerifyDir(), filepath.Join(w.Project, "verify"))
}

// One NUL-separated argv per call, closed by a record separator: an argument
// holding a whole GDScript file survives nothing simpler.
// echoCommandsEnv makes the shim print each command before running it, for
// --verbose runs. Solution runs only: an agent on the CLI surface would see
// the echo in its tool output.
const echoCommandsEnv = "GODAI_EVAL_ECHO_COMMANDS"

const shimTemplate = `#!/bin/sh
export %s
{ printf '%%s\0' "$0" "$@"; printf '\036'; } >> %s
[ -z "${%s:-}" ] || printf '$ godai %%s\n' "$*" >&2
exec %s "$@"
`

// The shim logs every call before handing it on, prefixed with inject, and must
// be named godai because that is how the log identifies a godai call.
func (w *Workspace) GodaiShim(inject ...string) (string, error) {
	path := filepath.Join(w.Root, "shim", "godai")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	command := make([]string, 0, len(inject)+1)
	for _, arg := range append([]string{w.cfg.GodaiBin}, inject...) {
		command = append(command, shellQuote(arg))
	}

	exports := make([]string, 0, 5)
	for _, kv := range w.IsolationEnv() {
		name, value, _ := strings.Cut(kv, "=")
		exports = append(exports, name+"="+shellQuote(value))
	}

	script := fmt.Sprintf(shimTemplate,
		strings.Join(exports, " "), shellQuote(w.shimLog()), echoCommandsEnv, strings.Join(command, " "))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (w *Workspace) shimLog() string { return filepath.Join(w.Root, "godai-calls.log") }

// ShimCalls replays the shim's log in the shape the agent surfaces produce, so
// everything downstream scores both the same way.
func (w *Workspace) ShimCalls() ([]ToolCall, error) {
	blob, err := os.ReadFile(w.shimLog())
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var calls []ToolCall
	for record := range strings.SplitSeq(string(blob), "\x1e") {
		if record == "" {
			continue
		}
		argv := strings.Split(strings.TrimSuffix(record, "\x00"), "\x00")
		input, err := json.Marshal(map[string]string{"command": strings.Join(argv, " ")})
		if err != nil {
			return nil, err
		}
		calls = append(calls, ToolCall{Name: "Bash", Input: input, Turn: len(calls) + 1})
	}
	return calls, nil
}

func (w *Workspace) GodaiArgs(args ...string) []string {
	global := []string{
		"--root", w.Project,
		"--no-input",
		"--no-auto-install",
	}
	if w.cfg.GodotBin != "" {
		global = append(global, "--godot-path", w.cfg.GodotBin)
	}
	return append(global, args...)
}

func (w *Workspace) OpenEditor(ctx context.Context) error {
	return w.OpenEditorWith(ctx, nil)
}

// OpenEditorWith hands the editor extra environment, which is how the addon is
// told to run a prompt of its own.
func (w *Workspace) OpenEditorWith(ctx context.Context, env []string) error {
	// --auto-approve is required: nobody is there to answer the approval
	// dialog in a headless editor.
	out, err := w.godaiWith(ctx, env, "project", "open", w.Project, "--headless", "--auto-approve")
	if err != nil {
		return fmt.Errorf("open editor: %w: %s", err, out)
	}
	w.editorClosed = false
	return nil
}

// CloseEditor deliberately skips saving, so that a task asking the agent to
// save its work actually measures whether it did.
func (w *Workspace) CloseEditor(ctx context.Context) error {
	if w.editorClosed {
		return nil
	}
	w.editorClosed = true
	// A task may legitimately have the agent close (or restart-and-close) the
	// editor itself, so an editor that is already gone isn't an error.
	if !w.editorRunning(ctx) {
		return nil
	}
	out, err := w.godai(ctx, "editor", "close", w.Project, "--skip-save")
	if err != nil {
		w.closeErr = fmt.Errorf("close editor: %w: %s", err, out)
		return w.closeErr
	}
	return nil
}

func (w *Workspace) editorRunning(ctx context.Context) bool {
	out, err := w.godai(ctx, "--json", "editor", "list")
	if err != nil {
		return true // let `editor close` report what's actually wrong
	}
	return strings.Contains(out, filepath.Base(w.Root))
}

func (w *Workspace) godai(ctx context.Context, args ...string) (string, error) {
	return w.godaiWith(ctx, nil, args...)
}

// godaiJSON keeps stdout separate so `--json` output stays parseable, and
// folds stderr into the error when the command fails.
func (w *Workspace) godaiJSON(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, w.cfg.GodaiBin, w.GodaiArgs(append([]string{"--json"}, args...)...)...)
	cmd.Dir = w.Root
	cmd.Env = w.Env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()+stdout.String()))
	}
	return stdout.String(), nil
}

func (w *Workspace) godaiWith(ctx context.Context, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, w.cfg.GodaiBin, w.GodaiArgs(args...)...)
	cmd.Dir = w.Root
	cmd.Env = append(w.Env(), env...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (w *Workspace) commitBaseline() error {
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.email=eval@localhost", "-c", "user.name=eval", "commit", "-qm", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = w.Project
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v: %s", args, err, out)
		}
	}
	return nil
}

func (w *Workspace) DirtyPaths(paths []string) ([]string, error) {
	var dirty []string
	for _, p := range paths {
		cmd := exec.Command("git", "status", "--porcelain", "--", p)
		cmd.Dir = w.Project
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(out.String()) != "" {
			dirty = append(dirty, p)
		}
	}
	return dirty, nil
}

// Cleanup closes any editor still running before removing the scratch
// directory, so a failed attempt doesn't leave a headless Godot behind.
func (w *Workspace) Cleanup(ctx context.Context) {
	_ = w.CloseEditor(ctx)
	w.killStrayEditors()
	if !w.cfg.KeepWork {
		os.RemoveAll(w.Root)
	}
}

// godai detaches the editor into its own process group, so an editor that never finished starting is one `editor close` can't find and nothing else will end.
func (w *Workspace) killStrayEditors() {
	for _, pid := range strayEditorPids(filepath.Base(w.Root)) {
		if killProcess(pid) == nil && w.closeErr != nil {
			log.Printf("killed a Godot left behind by %s (pid %d): %v",
				filepath.Base(w.Root), pid, w.closeErr)
		}
	}
}

// A whole argument has to be --editor (the editor) or --editor-pid (a game the editor spawned), so godai's own --editor-timeout and friends can never make this kill the harness's godai instead.
func isEditorFor(args []string, scratchDir string) bool {
	editor, scratch := false, false
	for _, arg := range args {
		editor = editor || arg == "--editor" || arg == "--editor-pid"
		scratch = scratch || strings.Contains(arg, scratchDir)
	}
	return editor && scratch
}

func copyTree(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode())
	})
}
