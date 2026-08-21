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

func TestHelpHidesSnakeCaseAliases(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "editor-tool", "get_node_properties", "--help"})
	is.NoErr(err)

	is.True(strings.Contains(out, "--node-paths"))
	is.True(strings.Contains(out, "--node-path"))
	is.True(!strings.Contains(out, "node_paths"))
}

func TestHelpMarksRepeatableFlagsWithoutBrackets(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "editor-tool", "get_node_properties", "--help"})
	is.NoErr(err)

	is.True(strings.Contains(out, "(repeatable)"))
	is.True(!strings.Contains(out, "[ --")) // no [ --flag ] repeat brackets
}

func TestEditorToolHelpGroupsByToolsetAndHidesAliases(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "editor-tool", "--help"})
	is.NoErr(err)

	is.True(strings.Contains(out, "Scene:"))
	is.True(strings.Contains(out, "Script:"))
	is.True(strings.Contains(out, "add_node"))
	is.True(!strings.Contains(out, "add-node"))
}

func TestUnknownCommandSuggestsTheNearestName(t *testing.T) {
	is := is.New(t)

	_, err := runCLI(t, []string{"godai", "projct"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), `did you mean "project"?`))

	_, err = runCLI(t, []string{"godai", "editor-tool", "ad_node"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), `did you mean "add_node"?`))
}

func TestUnknownFlagSuggestsTheNearestName(t *testing.T) {
	is := is.New(t)

	_, err := runCLI(t, []string{"godai", "editor-tool", "read_script", "--file-pth", "res://x.gd"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "did you mean --file-path?"))
}

func TestKebabCaseToolNamesAreAccepted(t *testing.T) {
	is := is.New(t)

	// Resolving the alias gets as far as the tool's own required-argument
	// check, rather than failing as an unknown command.
	_, err := runCLI(t, []string{"godai", "editor-tool", "read-script"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "file_path"))
}

func TestRootHelpGroupsGlobalOptions(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--help"})
	is.NoErr(err)

	for _, header := range []string{"Choosing a Godot version:", "Choosing a project:", "Connecting to the editor:", "Output:"} {
		is.True(strings.Contains(out, header))
	}
}

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
	is.True(strings.Contains(out, "--headless")) // the command's own flags remain
	is.True(strings.Contains(out, globalOptionsNote))
}

func TestSubcommandHelpPointsAtGlobalOptionsInstead(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "editor", "--help"})
	is.NoErr(err)
	is.True(!strings.Contains(out, "GLOBAL OPTIONS:"))
	is.True(strings.Contains(out, globalOptionsNote))
}
