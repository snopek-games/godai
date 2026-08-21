package eval

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

// One --model has to mean one model on every surface, or the comparison is
// between two models rather than two surfaces.
func TestAPIModelResolvesTheAliasesClaudeCodeTakes(t *testing.T) {
	is := is.New(t)

	is.Equal(APIModel("haiku"), "claude-haiku-4-5-20251001")
	is.Equal(APIModel("claude-opus-5"), "claude-opus-5")
}

func TestAwaitStreamWaitsForAWholeResultLine(t *testing.T) {
	is := is.New(t)

	path := filepath.Join(t.TempDir(), "agent-stream.jsonl")
	f, err := os.Create(path)
	is.NoErr(err)
	defer f.Close()

	result := `{"type":"result","subtype":"success","num_turns":2}`
	go func() {
		for _, chunk := range []string{
			`{"type":"system","subtype":"init","model":"claude-haiku-4-5-20251001"}` + "\n",
			`{"type":"assistant","message":{"role":"assistant","content":[]}}` + "\n",
			result[:20],
			result[20:] + "\n",
		} {
			time.Sleep(streamPoll / 2)
			f.WriteString(chunk)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var live bytes.Buffer
	blob, err := awaitStream(ctx, path, newLiveWriter(&live, &Spec{ID: "add-player-sprite"}, 1))
	is.NoErr(err)

	m, err := ParseStream(bytes.NewReader(blob))
	is.NoErr(err)
	is.Equal(m.MalformedLines, 0) // the split result line was read whole
	is.Equal(m.NumTurns, 2)
	is.Equal(m.Model, "claude-haiku-4-5-20251001")
	is.True(strings.Contains(live.String(), "session started"))
}

func TestAwaitStreamSaysWhenTheAddonNeverStarted(t *testing.T) {
	is := is.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), streamPoll)
	defer cancel()

	_, err := awaitStream(ctx, filepath.Join(t.TempDir(), "agent-stream.jsonl"), nil)
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "never started"))
}

// A restart_editor (or crash) mid-task ends the wait with a distinct error
// instead of running out the task timeout.
func TestAwaitStreamReportsEditorExit(t *testing.T) {
	is := is.New(t)

	path := filepath.Join(t.TempDir(), "agent-stream.jsonl")
	lines := `{"type":"system","subtype":"init","model":"m"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[]}}` + "\n"
	is.NoErr(os.WriteFile(path, []byte(lines), 0o644))

	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		time.Sleep(streamPoll / 2)
		cancel(errEditorExited)
	}()

	blob, err := awaitStream(ctx, path, nil)
	is.True(errors.Is(err, errEditorExited))
	is.Equal(string(blob), lines) // keeps what streamed before the exit
}

func TestWatchEditorExitCancelsWhenTheProcessDies(t *testing.T) {
	skipWithoutUnixHarness(t)

	is := is.New(t)

	cmd := exec.Command("sleep", "0.2")
	is.NoErr(cmd.Start())
	go cmd.Wait() // reap, or the zombie still answers signal 0

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go watchEditorExit(ctx, []int{cmd.Process.Pid}, cancel)

	select {
	case <-ctx.Done():
		is.True(errors.Is(context.Cause(ctx), errEditorExited))
	case <-time.After(10 * time.Second):
		t.Fatal("the watcher never noticed the process exit")
	}
}
