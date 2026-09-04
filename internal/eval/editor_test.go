package eval

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestAwaitStreamContinuesPastATeardownResult(t *testing.T) {
	is := is.New(t)

	path := filepath.Join(t.TempDir(), "agent-stream.jsonl")
	f, err := os.Create(path)
	is.NoErr(err)
	defer f.Close()

	f.WriteString(`{"type":"system","subtype":"init","model":"m"}` + "\n" +
		`{"type":"result","subtype":"editor_teardown","num_turns":1,"usage":{"input_tokens":10}}` + "\n")

	go func() {
		time.Sleep(streamPoll * 2)
		f.WriteString(`{"type":"result","subtype":"success","num_turns":2,"usage":{"input_tokens":30}}` + "\n")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blob, err := awaitStream(ctx, path, nil)
	is.NoErr(err)

	m, err := ParseStream(bytes.NewReader(blob))
	is.NoErr(err)
	is.Equal(m.Segments, 2)
	is.Equal(m.NumTurns, 3)              // both segments' turns are summed
	is.Equal(m.Usage.InputTokens, 40)    // and their usage
	is.Equal(m.ResultSubtype, "success") // the final segment names the outcome
	is.True(!m.ResultIsError)
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

func TestWatchEditorExitCancelsWhenNoEditorComesBack(t *testing.T) {
	is := is.New(t)

	var scans atomic.Int32
	scan := func() []int {
		if scans.Add(1) == 1 {
			return []int{1234}
		}
		return nil
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go watchEditorExit(ctx, scan, 5*time.Millisecond, 20*time.Millisecond, cancel)

	select {
	case <-ctx.Done():
		is.True(errors.Is(context.Cause(ctx), errEditorExited))
	case <-time.After(10 * time.Second):
		t.Fatal("the watcher never noticed the editor was gone")
	}
}

func TestWatchEditorExitFollowsARestartedEditor(t *testing.T) {
	is := is.New(t)

	// The editor disappears for one scan (restart in progress), then its
	// replacement shows up and stays.
	var scans atomic.Int32
	scan := func() []int {
		if scans.Add(1) == 2 {
			return nil
		}
		return []int{1234}
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go watchEditorExit(ctx, scan, time.Millisecond, 20*time.Millisecond, cancel)

	select {
	case <-ctx.Done():
		t.Fatal("the watcher gave up on an editor that came back")
	case <-time.After(250 * time.Millisecond):
	}
	is.True(scans.Load() > 3) // the watcher kept scanning after the gap
}
