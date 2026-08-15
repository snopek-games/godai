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

const globalOptionsNote = "Global options also apply; run 'godai --help' to list them."

func TestRootHelpListsGlobalOptions(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--help"})
	is.NoErr(err)
	is.True(strings.Contains(out, "GLOBAL OPTIONS:"))
	is.True(strings.Contains(out, "--godot-version"))
}

func TestCommandHelpPointsAtGlobalOptionsInstead(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "mcp", "--help"})
	is.NoErr(err)
	is.True(!strings.Contains(out, "GLOBAL OPTIONS:"))
	is.True(!strings.Contains(out, "--godot-version"))
	is.True(strings.Contains(out, "--headless"))
	is.True(strings.Contains(out, globalOptionsNote))
}

func TestSubcommandHelpPointsAtGlobalOptionsInstead(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "editor", "--help"})
	is.NoErr(err)
	is.True(!strings.Contains(out, "GLOBAL OPTIONS:"))
	is.True(strings.Contains(out, globalOptionsNote))
}
