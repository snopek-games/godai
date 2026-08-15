package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Clipping is safe because the .jsonl beside it keeps the untruncated copy.
const maxBlockChars = 2000

func WriteTranscript(dir string, spec *Spec, att *Attempt, stream []byte) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	base := filepath.Join(dir, fmt.Sprintf("%s-r%d", spec.ID, att.Repeat))
	if len(stream) > 0 {
		if err := os.WriteFile(base+".jsonl", stream, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(base+".md", []byte(renderTranscript(spec, att, stream)), 0o644)
}

func renderTranscript(spec *Spec, att *Attempt, stream []byte) string {
	var b strings.Builder

	status := "FAIL"
	if att.Passed {
		status = "PASS"
	}
	fmt.Fprintf(&b, "# %s r%d — %s/%s\n\n", spec.ID, att.Repeat, att.Model, att.Surface)
	fmt.Fprintf(&b, "%s, %d/%d checks", status, att.ChecksPassed, att.ChecksTotal)
	if len(att.FailedChecks) > 0 {
		names := make([]string, len(att.FailedChecks))
		for i, c := range att.FailedChecks {
			names[i] = c.Name
		}
		fmt.Fprintf(&b, " — failed: %s", strings.Join(names, ", "))
	}
	b.WriteString("\n\n")
	if att.Error != "" {
		fmt.Fprintf(&b, "**Error:** %s\n\n", att.Error)
	}
	renderFailedChecks(&b, att)

	fmt.Fprintf(&b, "## Instruction\n\n%s\n\n## Conversation\n\n", spec.Instruction)
	if len(stream) > 0 {
		renderStream(&b, stream)
	} else {
		renderCalls(&b, att.Metrics)
	}
	return b.String()
}

func renderFailedChecks(b *strings.Builder, att *Attempt) {
	if len(att.FailedChecks) == 0 {
		return
	}
	b.WriteString("## Failed checks\n\n")
	for _, c := range att.FailedChecks {
		if c.Detail == "" {
			fmt.Fprintf(b, "- `%s`\n", c.Name)
			continue
		}
		fmt.Fprintf(b, "- `%s` — %s\n", c.Name, c.Detail)
	}
	b.WriteString("\n")
	if att.VerifyLog != "" {
		fmt.Fprintf(b, "### Verifier output\n\n```\n%s\n```\n\n", clip(att.VerifyLog, maxBlockChars))
	}
}

func renderStream(b *strings.Builder, stream []byte) {
	toolNames := map[string]string{}
	// Starting on "Main" keeps the heading out of the transcripts that never
	// delegate, which is most of them.
	lastText, lastSpeaker := "", "Main"
	segment := 0

	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)
	for sc.Scan() {
		var ev streamEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "assistant", "user":
			var msg apiMessage
			if json.Unmarshal(ev.Message, &msg) != nil {
				continue
			}
			if speaker := speakerOf(ev); speaker != lastSpeaker {
				fmt.Fprintf(b, "### %s\n\n", speaker)
				lastSpeaker = speaker
			}
			for _, blk := range msg.Content {
				if text := renderBlock(b, blk, toolNames); text != "" {
					lastText = text
				}
			}

		case "result":
			segment++
			fmt.Fprintf(b, "### End of session %d\n\n%s, %d turns, $%.4f\n",
				segment, ev.Subtype, ev.NumTurns, ev.TotalCostUSD)
			// The result field repeats the last message, unless the run ended some way the model didn't choose.
			if r := strings.TrimSpace(ev.Result); r != "" && r != lastText {
				fmt.Fprintf(b, "\n%s\n", clip(r, maxBlockChars))
			}
			b.WriteString("\n")
		}
	}
}

func speakerOf(ev streamEvent) string {
	if ev.ParentToolUseID == nil {
		return "Main"
	}
	id := *ev.ParentToolUseID
	return "Subagent " + id[max(0, len(id)-6):]
}

// Returns the assistant text it rendered, so the caller can recognise the final message.
func renderBlock(b *strings.Builder, blk contentBlock, toolNames map[string]string) string {
	switch blk.Type {
	case "text":
		if s := strings.TrimSpace(blk.Text); s != "" {
			fmt.Fprintf(b, "%s\n\n", s)
			return s
		}

	case "thinking":
		if s := strings.TrimSpace(blk.Thinking); s != "" {
			fmt.Fprintf(b, "> _thinking_\n>\n> %s\n\n", strings.ReplaceAll(clip(s, maxBlockChars), "\n", "\n> "))
		}

	case "tool_use":
		toolNames[blk.ID] = blk.Name
		fmt.Fprintf(b, "**→ %s**\n\n```json\n%s\n```\n\n", blk.Name, clip(string(blk.Input), maxBlockChars))

	case "tool_result":
		outcome := "ok"
		if blk.IsError {
			outcome = "ERROR"
		}
		fmt.Fprintf(b, "**← %s (%s)**\n\n", toolNames[blk.ToolUseID], outcome)
		if text := strings.TrimSpace(resultText(blk.Content)); text != "" {
			fmt.Fprintf(b, "```\n%s\n```\n\n", clip(text, maxBlockChars))
		}
	}
	return ""
}

func renderCalls(b *strings.Builder, m *RunMetrics) {
	if m == nil {
		return
	}
	for _, a := range NormalizeActions(m.ToolCalls) {
		fmt.Fprintf(b, "**→ %s**\n\n```sh\n%s\n```\n\n", a.Name, a.Raw)
	}
}

func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, blk := range blocks {
			if blk.Text != "" {
				parts = append(parts, blk.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	// Blocks carrying something other than text, such as a tool reference, are worth showing raw rather than dropping.
	return string(raw)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n... %d more bytes", len(s)-n)
}
