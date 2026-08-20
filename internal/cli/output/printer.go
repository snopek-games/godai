package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type Printer struct {
	Out      io.Writer
	Err      io.Writer
	JSON     bool
	ColorOut bool
	ColorErr bool
}

func NewPrinter(out, errOut io.Writer, asJSON bool) *Printer {
	return &Printer{
		Out:      out,
		Err:      errOut,
		JSON:     asJSON,
		ColorOut: !asJSON && colorEnabled(out),
		ColorErr: colorEnabled(errOut),
	}
}

// Paint styles text for the human output stream; the text comes back unchanged
// when color is off (not a terminal, NO_COLOR, or JSON output).
func (p *Printer) Paint(code, text string) string {
	return Paint(p.ColorOut, code, text)
}

func (p *Printer) Value(v any, human func(w io.Writer) error) error {
	if p.JSON {
		return p.writeJSON(v)
	}
	return human(p.Out)
}

func (p *Printer) writeJSON(v any) error {
	encoder := json.NewEncoder(p.Out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func (p *Printer) Table(headers []string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}

	w := tabwriter.NewWriter(p.Out, 0, 0, 2, ' ', 0)
	if len(headers) > 0 {
		fmt.Fprintln(w, strings.Join(headers, "\t"))
	}
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

func (p *Printer) Note(format string, args ...any) {
	fmt.Fprintf(p.Err, Paint(p.ColorErr, Cyan, "note:")+" "+format+"\n", args...)
}

func (p *Printer) Warn(format string, args ...any) {
	fmt.Fprintf(p.Err, Paint(p.ColorErr, Yellow, "warning:")+" "+format+"\n", args...)
}

func (p *Printer) Error(format string, args ...any) {
	fmt.Fprintf(p.Err, Paint(p.ColorErr, Red, "error:")+" "+format+"\n", args...)
}

func (p *Printer) Printf(format string, args ...any) {
	if p.JSON {
		return
	}
	fmt.Fprintf(p.Out, format, args...)
}
