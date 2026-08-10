package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type Printer struct {
	Out  io.Writer
	Err  io.Writer
	JSON bool
}

func NewPrinter(out, errOut io.Writer, asJSON bool) *Printer {
	return &Printer{Out: out, Err: errOut, JSON: asJSON}
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
	fmt.Fprintf(p.Err, "note: "+format+"\n", args...)
}

func (p *Printer) Warn(format string, args ...any) {
	fmt.Fprintf(p.Err, "warning: "+format+"\n", args...)
}

func (p *Printer) Printf(format string, args ...any) {
	if p.JSON {
		return
	}
	fmt.Fprintf(p.Out, format, args...)
}
