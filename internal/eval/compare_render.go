package eval

import (
	"fmt"
	"strings"
)

// One table with every metric in it is too wide to read, in markdown or in a
// terminal, so the report is a grid plus a table per theme.
func (c *Comparison) Markdown() string {
	var b strings.Builder
	b.WriteString("## godai eval\n")

	c.passGrid().write(&b, "Pass rate")
	c.outcome().write(&b, "Outcome")
	c.selection().write(&b, "Tool selection")
	c.cost().write(&b, "Cost")
	c.trouble().write(&b, "Trouble")
	c.byTask().write(&b, "By task")
	c.failures().write(&b, "Failures and errors")

	if len(c.Notes) > 0 {
		b.WriteString("\n### Worth knowing\n\n")
		for _, note := range c.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}
	return b.String()
}

func (c *Comparison) passGrid() *table {
	surfaces := c.SurfaceColumns()
	t := newTable(append([]string{"model"}, surfaces...))

	for _, model := range c.Models() {
		row := []string{model}
		for _, surface := range surfaces {
			cell := c.Cell(model, surface)
			if cell == nil {
				row = append(row, "—")
				continue
			}
			row = append(row, percent(cell.PassRate()))
		}
		t.row(row...)
	}
	return t
}

func (c *Comparison) outcome() *table {
	header := []string{"model", "surface", "pass", "partial", "every repeat"}
	if len(c.Baseline) > 0 {
		header = append(header, "Δ", "p")
	}
	t := newTable(header)

	for _, cell := range c.Cells {
		s := cell.Summary
		row := []string{
			cell.Key.Model, cell.Key.Surface,
			fmt.Sprintf("%s (%d/%d)", percent(cell.PassRate()), s.Passes, s.Trials),
			ratio(s.PartialCredit),
			ratio(s.PassAllRepeats),
		}
		if len(c.Baseline) > 0 {
			delta, p := "—", "—"
			if d, ok := c.Delta(cell); ok {
				delta, p = fmt.Sprintf("%s %+.0f", arrow(d.Points), d.Points), fmt.Sprintf("%.3f", d.P)
				if d.P < c.Alpha {
					delta = "**" + delta + "**"
				}
			}
			row = append(row, delta, p)
		}
		t.row(row...)
	}
	return t
}

func (c *Comparison) selection() *table {
	t := newTable([]string{"model", "surface", "first action", "precision", "recall", "bypass"})
	for _, cell := range c.Cells {
		s := cell.Summary
		t.row(cell.Key.Model, cell.Key.Surface,
			ratio(s.MeanFirstActionCorrect), ratio(s.MeanActionPrecision),
			ratio(s.MeanActionRecall), ratio(s.MeanBypassRate))
	}
	return t
}

func (c *Comparison) cost() *table {
	t := newTable([]string{"model", "surface", "turns", "tokens", "wall", "cost"})
	for _, cell := range c.Cells {
		s := cell.Summary
		t.row(cell.Key.Model, cell.Key.Surface,
			fmt.Sprintf("%.1f", s.MeanTurns),
			fmt.Sprintf("%.0f", s.MeanTokens),
			fmt.Sprintf("%.0fs", s.MeanWallSec),
			cost(s.TotalCostUSD))
	}
	return t
}

func (c *Comparison) trouble() *table {
	t := newTable([]string{"model", "surface", "tool errors", "help calls", "harness errors"})
	for _, cell := range c.Cells {
		s := cell.Summary
		t.row(cell.Key.Model, cell.Key.Surface,
			ratio(s.MeanToolErrorRate),
			fmt.Sprintf("%.1f", s.MeanHelpCalls),
			fmt.Sprintf("%d/%d", s.HarnessErrs, s.Trials))
	}
	return t
}

func (c *Comparison) byTask() *table {
	header := []string{"task"}
	for _, cell := range c.Cells {
		header = append(header, cell.Key.String())
	}
	t := newTable(header)

	for _, id := range c.Tasks {
		row := []string{id}
		for _, cell := range c.Cells {
			passes, trials := cell.Task(id)
			if trials == 0 {
				row = append(row, "—")
				continue
			}
			row = append(row, fmt.Sprintf("%s (%d/%d)", percent(rate(passes, trials)), passes, trials))
		}
		t.row(row...)
	}
	return t
}

const (
	maxFailureRows   = 40
	maxFailureReason = 160
)

func (c *Comparison) failures() *table {
	t := newTable([]string{"model/surface", "task", "repeat", "why"})

	dropped := 0
	for _, cell := range c.Cells {
		for _, a := range cell.Attempts {
			if a.Passed && a.Error == "" {
				continue
			}
			if len(t.rows) == maxFailureRows {
				dropped++
				continue
			}
			t.row(cell.Key.String(), a.TaskID, fmt.Sprintf("r%d", a.Repeat), failureReason(a))
		}
	}
	if dropped > 0 {
		c.Notes = append(c.Notes, fmt.Sprintf("%d further failures are not listed above; the transcripts have all of them", dropped))
	}
	return t
}

func failureReason(a *Attempt) string {
	var parts []string
	for _, check := range a.FailedChecks {
		parts = append(parts, check.String())
	}
	if a.Error != "" {
		parts = append(parts, a.Error)
	}
	if len(parts) == 0 {
		return "no checks ran"
	}
	return clipRunes(strings.Join(parts, "; "), maxFailureReason)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func percent(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

func ratio(v float64) string { return fmt.Sprintf("%.2f", v) }

// A surface that reports no cost would otherwise read as free rather than as
// unmeasured.
func cost(usd float64) string {
	if usd == 0 {
		return "—"
	}
	return fmt.Sprintf("$%.2f", usd)
}

func arrow(points float64) string {
	switch {
	case points > 0:
		return "▲"
	case points < 0:
		return "▼"
	}
	return "▪"
}

// Columns are padded so the report reads as a table in a terminal as well as
// rendering as one in markdown.
type table struct {
	header []string
	rows   [][]string
}

func newTable(header []string) *table { return &table{header: header} }

func (t *table) row(cells ...string) { t.rows = append(t.rows, cells) }

func (t *table) write(b *strings.Builder, heading string) {
	if len(t.rows) == 0 {
		return
	}

	widths := make([]int, len(t.header))
	for i, h := range t.header {
		widths[i] = width(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			widths[i] = max(widths[i], width(cell))
		}
	}

	fmt.Fprintf(b, "\n### %s\n\n", heading)
	writeRow(b, t.header, widths)
	rule := make([]string, len(widths))
	for i, w := range widths {
		rule[i] = strings.Repeat("-", w)
	}
	writeRow(b, rule, widths)
	for _, row := range t.rows {
		writeRow(b, row, widths)
	}
}

func writeRow(b *strings.Builder, cells []string, widths []int) {
	b.WriteString("|")
	for i, cell := range cells {
		fmt.Fprintf(b, " %s%s |", cell, strings.Repeat(" ", widths[i]-width(cell)))
	}
	b.WriteString("\n")
}

// The arrows and dashes are multi-byte, and padding on len() would misalign
// every column after one.
func width(s string) int { return len([]rune(s)) }
