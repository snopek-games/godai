package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

func newPrompter(cmd *cli.Command) core.Prompter {
	if cmd.Bool("no-input") || !isInteractive() {
		return core.NoPrompter{}
	}
	return &ttyPrompter{}
}

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

type ttyPrompter struct{}

func (p *ttyPrompter) Prompt(ctx context.Context, message string, schema map[string]any) (map[string]any, error) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("prompt schema has no properties")
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", message)

	required := requiredFields(schema)
	reader := bufio.NewReader(os.Stdin)
	answers := map[string]any{}

	for _, name := range sortedKeys(properties) {
		field, _ := properties[name].(map[string]any)
		value, err := p.ask(reader, name, field, required[name])
		if err != nil {
			return nil, err
		}
		if value != nil {
			answers[name] = value
		}
	}

	return answers, nil
}

func (p *ttyPrompter) ask(reader *bufio.Reader, name string, field map[string]any, required bool) (any, error) {
	fieldType, _ := field["type"].(string)
	description, _ := field["description"].(string)

	defaultValue := field["default"]
	if text, ok := defaultValue.(string); ok && text == "" {
		// An empty default would otherwise make pressing Enter *answer* the
		// question with nothing, rather than leave it unanswered.
		defaultValue = nil
	}

	if description != "" {
		fmt.Fprintf(os.Stderr, "\n  %s\n", description)
	}

	for {
		fmt.Fprintf(os.Stderr, "  %s%s: ", name, promptSuffix(fieldType, defaultValue, field["enum"]))

		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return nil, core.ErrPromptDeclined
		}
		answer := strings.TrimSpace(line)

		if answer == "" {
			if defaultValue != nil {
				return defaultValue, nil
			}
			if required {
				fmt.Fprintln(os.Stderr, "  (required)")
				continue
			}
			return nil, nil
		}

		if fieldType == "boolean" {
			switch strings.ToLower(answer) {
			case "y", "yes", "true":
				return true, nil
			case "n", "no", "false":
				return false, nil
			default:
				fmt.Fprintln(os.Stderr, "  (please answer y or n)")
				continue
			}
		}

		if allowed := enumValues(field["enum"]); len(allowed) > 0 && !contains(allowed, answer) {
			fmt.Fprintf(os.Stderr, "  (must be one of: %s)\n", strings.Join(allowed, ", "))
			continue
		}

		return answer, nil
	}
}

func promptSuffix(fieldType string, defaultValue, enum any) string {
	if fieldType == "boolean" {
		if b, ok := defaultValue.(bool); ok && b {
			return " [Y/n]"
		}
		return " [y/N]"
	}
	if allowed := enumValues(enum); len(allowed) > 0 {
		return " (" + strings.Join(allowed, "/") + ")"
	}
	if defaultValue != nil {
		return fmt.Sprintf(" [%v]", defaultValue)
	}
	return ""
}

func enumValues(enum any) []string {
	raw, ok := enum.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, fmt.Sprintf("%v", v))
	}
	return out
}

func requiredFields(schema map[string]any) map[string]bool {
	out := map[string]bool{}
	raw, ok := schema["required"].([]any)
	if !ok {
		return out
	}
	for _, v := range raw {
		if name, ok := v.(string); ok {
			out[name] = true
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
