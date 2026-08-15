package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// The addon talks to the Messages API, which takes model ids where Claude Code
// takes aliases. Anything not listed is passed through as an id.
var apiModels = map[string]string{
	"haiku":  "claude-haiku-4-5-20251001",
	"sonnet": "claude-sonnet-5",
	"opus":   "claude-opus-5",
}

func APIModel(model string) string {
	if id, ok := apiModels[model]; ok {
		return id
	}
	return model
}

// This surface opens the editor itself rather than leaving it to RunAttempt:
// the addon reads its prompt environment at startup.
func runEditorAgent(ctx context.Context, cfg Config, spec *Spec, work *Workspace) agentRun {
	m := &RunMetrics{Model: cfg.Model}

	stream := filepath.Join(work.Root, "agent-stream.jsonl")
	prompt := filepath.Join(work.Root, "agent-prompt.txt")
	env := []string{
		"GODAI_EVAL_PROMPT_FILE=" + prompt,
		"GODAI_EVAL_STREAM_FILE=" + stream,
		"GODAI_AUTO_APPROVE_TOOLS=1",
		"GODAI_ANTHROPIC_API_KEY=" + os.Getenv("ANTHROPIC_API_KEY"),
		"GODAI_ANTHROPIC_MODEL=" + APIModel(cfg.Model),
	}
	if err := work.OpenEditorWith(ctx, env); err != nil {
		return agentRun{metrics: m, code: -1, errs: []error{err}}
	}

	// The conversation lives in the editor process, so a restarted or crashed
	// editor starts the task over instead of resuming; end the attempt instead.
	// @todo Once the addon can resume the conversation after a restart, stop
	// ending the attempt here and follow the relaunched editor's PID instead.
	streamCtx, cancelStream := context.WithCancelCause(ctx)
	defer cancelStream(nil)
	go watchEditorExit(streamCtx, strayEditorPids(filepath.Base(work.Root)), cancelStream)

	// Writing the prompt is what starts the agent, and it waits until godai has
	// disconnected from the editor it just opened.
	if err := os.WriteFile(prompt, []byte(spec.Instruction), 0o644); err != nil {
		return agentRun{metrics: m, code: -1, errs: []error{err}}
	}

	var live io.Writer
	if cfg.Verbose {
		live = newLiveWriter(cfg.LiveOut, spec, work.Repeat)
	}

	blob, waitErr := awaitStream(streamCtx, stream, live)

	metrics, parseErr := ParseStream(bytes.NewReader(blob))
	metrics.Model = cfg.Model

	var errs []error
	if parseErr != nil {
		errs = append(errs, fmt.Errorf("parse stream: %w", parseErr))
	}
	if waitErr != nil {
		errs = append(errs, waitErr)
	} else if metrics.Segments == 0 {
		// ParseStream drops what it can't read rather than failing a run, which
		// would leave a malformed result event looking like a free, instant one.
		errs = append(errs, fmt.Errorf("no result event in %d unreadable lines", metrics.MalformedLines))
	} else if metrics.ResultIsError {
		errs = append(errs, fmt.Errorf("agent: %s", metrics.ResultMessage))
	}
	return agentRun{metrics: metrics, stream: blob, errs: errs}
}

const streamPoll = 250 * time.Millisecond

const editorWatchPoll = time.Second

var errEditorExited = errors.New("the editor exited before the agent finished")

func watchEditorExit(ctx context.Context, pids []int, cancel context.CancelCauseFunc) {
	if len(pids) == 0 {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(editorWatchPoll):
		}

		if !slices.ContainsFunc(pids, processAlive) {
			cancel(errEditorExited)
			return
		}
	}
}

// awaitStream follows the file the addon appends to until the run reports
// itself finished. Only whole lines count: the last one may still be arriving.
func awaitStream(ctx context.Context, path string, live io.Writer) ([]byte, error) {
	var seen []byte
	poll := func() (bool, error) {
		blob, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if end := bytes.LastIndexByte(blob, '\n') + 1; end > len(seen) {
			if live != nil {
				_, _ = live.Write(blob[len(seen):end])
			}
			seen = blob[:end]
			return hasResult(seen), nil
		}
		return false, nil
	}

	for {
		done, err := poll()
		if err != nil {
			return nil, err
		}
		if done {
			return seen, nil
		}

		select {
		case <-ctx.Done():
			// The result may have landed since the read above.
			if done, err := poll(); err == nil && done {
				return seen, nil
			}
			if len(seen) == 0 {
				return nil, fmt.Errorf("no agent output in %s: the addon never started", filepath.Base(path))
			}
			if cause := context.Cause(ctx); errors.Is(cause, errEditorExited) {
				return seen, cause
			}
			return seen, fmt.Errorf("agent did not finish")
		case <-time.After(streamPoll):
		}
	}
}

func hasResult(blob []byte) bool {
	for line := range bytes.Lines(blob) {
		var ev struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(bytes.TrimSpace(line), &ev) == nil && ev.Type == "result" {
			return true
		}
	}
	return false
}
