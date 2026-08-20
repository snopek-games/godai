package cli

import (
	"io"
	"os"
	"strings"

	"gitlab.com/snopek-games/godai/internal/cli/output"

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
		cli.HelpPrinterCustom(out, templ, data, map[string]any{
			"wrap":       wrapHelpText,
			"heading":    helpHeading,
			"subheading": helpSubheading,
		})
	}
}

// Help goes to stdout, so the stderr-oriented useColor() doesn't apply.
func helpColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func helpHeading(text string) string {
	return output.Paint(helpColor(), output.BoldCyan, text)
}

func helpSubheading(text string) string {
	return output.Paint(helpColor(), output.Cyan, text)
}

// The templates are copies of urfave/cli's, differing in the trailing section
// (a pointer to the root help instead of every global option), the colored
// headings, and the COMMANDS list, which groups by category and lists each
// command under its primary name alone, keeping aliases out of the column.
const commandHelpTemplate = `{{heading "NAME:"}}
   {{template "helpNameTemplate" .}}

{{heading "USAGE:"}}
   {{template "usageTemplate" .}}{{if .Category}}

{{heading "CATEGORY:"}}
   {{.Category}}{{end}}{{if .Description}}

{{heading "DESCRIPTION:"}}
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

{{heading "OPTIONS:"}}{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

{{heading "OPTIONS:"}}{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

Global options also apply; run '{{.Root.Name}} --help' to list them.{{end}}
`

const subcommandHelpTemplate = `{{heading "NAME:"}}
   {{template "helpNameTemplate" .}}

{{heading "USAGE:"}}
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Category}}

{{heading "CATEGORY:"}}
   {{.Category}}{{end}}{{if .Description}}

{{heading "DESCRIPTION:"}}
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}

{{heading "COMMANDS:"}}{{range .VisibleCategories}}{{if .Name}}

   {{subheading (printf "%s:" .Name)}}{{range .VisibleCommands}}
     {{index .Names 0}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{range .VisibleCommands}}
   {{index .Names 0}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{end}}{{if .VisibleFlagCategories}}

{{heading "OPTIONS:"}}{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

{{heading "OPTIONS:"}}{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

Global options also apply; run '{{.Root.Name}} --help' to list them.{{end}}
`

const rootHelpTemplate = `{{heading "NAME:"}}
   {{template "helpNameTemplate" .}}

{{heading "USAGE:"}}
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}} {{if .VisibleFlags}}[global options]{{end}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}

{{heading "VERSION:"}}
   {{.Version}}{{end}}{{end}}{{if .Description}}

{{heading "DESCRIPTION:"}}
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}

{{heading "COMMANDS:"}}{{template "visibleCommandTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

{{heading "GLOBAL OPTIONS:"}}{{range .VisibleFlagCategories}}
   {{if .Name}}{{subheading .Name}}

   {{end}}{{$flglen := len .Flags}}{{range $i, $e := .Flags}}{{if eq (subtract $flglen $i) 1}}{{$e}}
{{else}}{{$e}}
   {{end}}{{end}}{{end}}{{else if .VisibleFlags}}

{{heading "GLOBAL OPTIONS:"}}{{template "visibleFlagTemplate" .}}{{end}}
`

func trimHelpGlobals() {
	cli.RootCommandHelpTemplate = rootHelpTemplate
	cli.CommandHelpTemplate = commandHelpTemplate
	cli.SubcommandHelpTemplate = subcommandHelpTemplate
}

var defaultFlagStringer = cli.FlagStringer

func adjustFlagHelp() {
	cli.FlagStringer = func(flag cli.Flag) string {
		if doc, ok := flag.(docFlag); ok {
			return defaultFlagStringer(displayFlag{doc})
		}
		return defaultFlagStringer(flag)
	}
}

// What the default FlagStringer type-asserts a flag into; every flag urfave/cli ships satisfies it.
type docFlag interface {
	cli.Flag
	cli.DocGenerationMultiValueFlag
	cli.RequiredFlag
}

type displayFlag struct{ docFlag }

func (f displayFlag) Names() []string {
	var names []string
	for _, name := range f.docFlag.Names() {
		if !strings.Contains(name, "_") {
			names = append(names, name)
		}
	}
	return names
}

func (f displayFlag) IsMultiValueFlag() bool {
	return false
}

func (f displayFlag) GetUsage() string {
	usage := f.docFlag.GetUsage()
	if f.docFlag.IsMultiValueFlag() {
		return strings.TrimSpace(usage + " (repeatable)")
	}
	return usage
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
