package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// A watched run is being read as it scrolls, so a line has to fit on a line.
const liveClip = 160

type liveWriter struct {
	prefix string
	out    io.Writer
	line   []byte
	tools  map[string]string
}

func newLiveWriter(out io.Writer, spec *Spec, repeat int) *liveWriter {
	return &liveWriter{
		prefix: fmt.Sprintf("%s r%d| ", spec.ID, repeat),
		out:    out,
		tools:  map[string]string{},
	}
}

func (w *liveWriter) Write(p []byte) (int, error) {
	w.line = append(w.line, p...)
	for {
		i := bytes.IndexByte(w.line, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.report(w.line[:i])
		w.line = w.line[i+1:]
	}
}

func (w *liveWriter) report(line []byte) {
	var ev streamEvent
	if len(line) == 0 || json.Unmarshal(line, &ev) != nil {
		return
	}
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" {
			w.say("session started (%d tools)", len(ev.Tools))
		}

	case "assistant", "user":
		var msg apiMessage
		if json.Unmarshal(ev.Message, &msg) != nil {
			return
		}
		for _, blk := range msg.Content {
			w.block(blk)
		}

	case "result":
		w.say("done: %s, %d turns, $%.4f", ev.Subtype, ev.NumTurns, ev.TotalCostUSD)
	}
}

func (w *liveWriter) block(blk contentBlock) {
	switch blk.Type {
	case "text":
		if s := oneLine(blk.Text); s != "" {
			w.say("%s", s)
		}
	case "thinking":
		if s := oneLine(blk.Thinking); s != "" {
			w.say("(thinking) %s", s)
		}
	case "tool_use":
		w.tools[blk.ID] = blk.Name
		w.say("-> %s %s", blk.Name, oneLine(string(blk.Input)))
	case "tool_result":
		if blk.IsError {
			w.say("<- %s ERROR %s", w.tools[blk.ToolUseID], oneLine(resultText(blk.Content)))
			return
		}
		w.say("<- %s ok", w.tools[blk.ToolUseID])
	}
}

func (w *liveWriter) say(format string, args ...any) {
	fmt.Fprintf(w.out, "%s%s\n", w.prefix, fmt.Sprintf(format, args...))
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > liveClip {
		return s[:liveClip] + "…"
	}
	return s
}
