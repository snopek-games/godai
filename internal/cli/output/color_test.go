package output

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestPaint(t *testing.T) {
	is := is.New(t)

	is.Equal(Paint(true, Cyan, "hi"), "\x1b[36mhi\x1b[0m")
	is.Equal(Paint(true, BoldCyan, "hi"), "\x1b[1;36mhi\x1b[0m")
	is.Equal(Paint(false, Cyan, "hi"), "hi")
}

func TestPrinterColorIsOffForNonTerminals(t *testing.T) {
	is := is.New(t)

	var out, errOut strings.Builder
	p := NewPrinter(&out, &errOut, false)
	is.Equal(p.ColorOut, false)
	is.Equal(p.ColorErr, false)

	p.Note("plain %s", "text")
	is.Equal(errOut.String(), "note: plain text\n")
	is.Equal(p.Paint(Cyan, "hi"), "hi")
}
