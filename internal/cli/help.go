package cli

import (
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const (
	defaultHelpWidth = 80
	minHelpWidth     = 40
	maxHelpWidth     = 100
)

// The offset urfave/cli's templates pass for a flag line, which is a row of
// tabwriter columns: wrapping one breaks the alignment of the whole block.
const flagLineOffset = 6

const bulletMarker = "- "

// urfave/cli's templates already wrap every usage and description, but at a
// default width of 10000 characters, so nothing ever wraps.
func wrapHelp() {
	cli.HelpPrinter = func(out io.Writer, templ string, data any) {
		cli.HelpPrinterCustom(out, templ, data, map[string]any{"wrap": wrapHelpText})
	}
}

// Both templates are copies of urfave/cli v3.6.1's, differing only in the
// trailing section: a pointer to the root help instead of every global option.
const commandHelpTemplate = `NAME:
   {{template "helpNameTemplate" .}}

USAGE:
   {{template "usageTemplate" .}}{{if .Category}}

CATEGORY:
   {{.Category}}{{end}}{{if .Description}}

DESCRIPTION:
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

OPTIONS:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

OPTIONS:{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

Global options also apply; run '{{.Root.Name}} --help' to list them.{{end}}
`

const subcommandHelpTemplate = `NAME:
   {{template "helpNameTemplate" .}}

USAGE:
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Category}}

CATEGORY:
   {{.Category}}{{end}}{{if .Description}}

DESCRIPTION:
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}

COMMANDS:{{template "visibleCommandTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

OPTIONS:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

OPTIONS:{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

Global options also apply; run '{{.Root.Name}} --help' to list them.{{end}}
`

func trimHelpGlobals() {
	cli.CommandHelpTemplate = commandHelpTemplate
	cli.SubcommandHelpTemplate = subcommandHelpTemplate
}

func wrapHelpText(text string, offset int) string {
	return wrapText(text, offset, helpWidth())
}

func wrapText(text string, offset, width int) string {
	if offset == flagLineOffset {
		return text
	}

	padding := strings.Repeat(" ", offset)

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		continuation := padding
		if strings.HasPrefix(line, bulletMarker) {
			continuation += strings.Repeat(" ", len(bulletMarker))
		}

		wrapped := wrapHelpLine(line, width-offset, width-len(continuation), continuation)
		if i > 0 && wrapped != "" {
			wrapped = padding + wrapped
		}
		lines[i] = wrapped
	}

	return strings.Join(lines, "\n")
}

func wrapHelpLine(line string, width, continuationWidth int, continuation string) string {
	words := strings.Fields(line)
	if len(line) <= width || len(words) == 0 {
		return line
	}

	wrapped := strings.Builder{}
	wrapped.WriteString(words[0])

	room := width - len(words[0])
	for _, word := range words[1:] {
		if len(word)+1 > room {
			wrapped.WriteString("\n" + continuation + word)
			room = continuationWidth - len(word)
			continue
		}
		wrapped.WriteString(" " + word)
		room -= 1 + len(word)
	}

	return wrapped.String()
}

func helpWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	switch {
	case err != nil || width < minHelpWidth:
		return defaultHelpWidth
	case width > maxHelpWidth:
		return maxHelpWidth
	}
	return width
}
