package output

import (
	"io"
	"os"

	"golang.org/x/term"
)

const (
	Red      = "31"
	Green    = "32"
	Yellow   = "33"
	Cyan     = "36"
	Bold     = "1"
	Dim      = "2"
	BoldCyan = Bold + ";" + Cyan
)

// Paint wraps text in an ANSI style when enabled, or returns it unchanged.
func Paint(enabled bool, code, text string) string {
	if !enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
