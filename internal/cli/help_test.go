package cli

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestWrapText(t *testing.T) {
	is := is.New(t)

	wrapped := wrapText("one two three four five six seven\n\nand a second paragraph", 3, 20)
	is.Equal(wrapped, strings.Join([]string{
		"one two three",
		"   four five six",
		"   seven",
		"",
		"   and a second",
		"   paragraph",
	}, "\n"))
}

func TestWrapTextHangsBulletContinuations(t *testing.T) {
	is := is.New(t)

	wrapped := wrapText("- one two three four five six", 3, 20)
	is.Equal(wrapped, strings.Join([]string{
		"- one two three",
		"     four five six",
	}, "\n"))
}

func TestWrapTextLeavesShortLinesAlone(t *testing.T) {
	is := is.New(t)

	is.Equal(wrapText("short enough", 3, 20), "short enough")
}

// Flag lines are tabwriter columns, so wrapping one would break the alignment
// of every flag in the block.
func TestWrapTextSkipsFlagLines(t *testing.T) {
	is := is.New(t)

	line := "--some-flag\tsomething much longer than the width we wrap at"
	is.Equal(wrapText(line, flagLineOffset, 20), line)
}
