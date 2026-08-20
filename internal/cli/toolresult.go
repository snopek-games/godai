package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"
)

func printToolResult(out *Printer, toolName string, result *core.ToolResult) error {
	if out.JSON {
		// Fall back to the raw MCP envelope when there's no structured content.
		payload := result.StructuredContent
		if len(payload) == 0 {
			payload = result.Raw
		}
		_, err := out.Out.Write(append([]byte(payload), '\n'))
		return err
	}

	if result.Unparsed {
		out.Printf("%s\n", result.Raw)
		return nil
	}

	if len(result.StructuredContent) > 0 {
		if printStructuredResult(out, toolName, result.StructuredContent) {
			return nil
		}
		pretty := &bytes.Buffer{}
		if err := json.Indent(pretty, result.StructuredContent, "", "  "); err == nil {
			out.Printf("%s\n", pretty.String())
			return nil
		}
	}

	if text := result.Text(); text != "" {
		out.Printf("%s\n", text)
	}
	return nil
}

type resultField struct {
	name  string
	value any
}

// The keys every tool shares don't get printed as data: success is already the
// exit code (and silence), and the message lists go to stderr with the same
// styling messages get everywhere else in the CLI.
func printStructuredResult(out *Printer, toolName string, raw json.RawMessage) bool {
	fields, err := decodeOrderedObject(raw)
	if err != nil {
		return false
	}

	var data []resultField
	var errs, warns, notes, outputLines []string
	for _, f := range fields {
		list, isStrings := stringList(f.value)
		switch {
		case f.name == "success":
		case f.name == "errors" && isStrings:
			errs = list
		case f.name == "warnings" && isStrings:
			warns = list
		case f.name == "notes" && isStrings:
			notes = list
		case f.name == "output" && isStrings:
			outputLines = list
		default:
			data = append(data, f)
		}
	}

	if render, ok := toolRenderers[toolName]; ok && len(data) > 0 {
		render(out, data)
	} else {
		printDataFields(out, data)
	}

	for _, line := range outputLines {
		out.Printf("%s\n", line)
	}
	for _, note := range notes {
		out.Note("%s", note)
	}
	for _, warn := range warns {
		out.Warn("%s", warn)
	}
	for _, e := range errs {
		out.Error("%s", e)
	}

	return true
}

var toolRenderers = map[string]func(*Printer, []resultField){
	"get_current_scene_tree": printSceneTree,
	"read_script":            printScriptContent,
}

func printDataFields(out *Printer, data []resultField) {
	if len(data) == 1 {
		switch value := data[0].value.(type) {
		case []any:
			if lines, ok := stringList(value); ok {
				for _, line := range lines {
					out.Printf("%s\n", line)
				}
				return
			}
		case map[string]any:
			printMap(out, value, "")
			return
		}
	}

	for _, f := range data {
		switch value := f.value.(type) {
		case map[string]any:
			out.Printf("%s:\n", f.name)
			printMap(out, value, "  ")
		case []any:
			out.Printf("%s:\n", f.name)
			for _, item := range value {
				out.Printf("  %s\n", formatScalar(item))
			}
		default:
			out.Printf("%s: %s\n", f.name, formatScalar(value))
		}
	}
}

func printMap(out *Printer, m map[string]any, indent string) {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		switch value := m[name].(type) {
		case map[string]any:
			out.Printf("%s%s:\n", indent, name)
			printMap(out, value, indent+"  ")
		case []any:
			out.Printf("%s%s:\n", indent, name)
			for _, item := range value {
				out.Printf("%s  %s\n", indent, formatScalar(item))
			}
		default:
			out.Printf("%s%s = %s\n", indent, name, formatScalar(value))
		}
	}
}

func printSceneTree(out *Printer, data []resultField) {
	printSceneTreeNode(out, fieldMap(data), "")
}

func printSceneTreeNode(out *Printer, node map[string]any, indent string) {
	name, _ := node["name"].(string)
	line := indent + name
	if nodeType, ok := node["type"].(string); ok && nodeType != "" {
		line += " " + out.Paint(output.Dim, "("+nodeType+")")
	}
	if script, ok := node["script"].(string); ok && script != "" {
		line += " " + out.Paint(output.Cyan, script)
	}
	out.Printf("%s\n", line)

	children, _ := node["children"].([]any)
	for _, child := range children {
		if childNode, ok := child.(map[string]any); ok {
			printSceneTreeNode(out, childNode, indent+"  ")
		}
	}
}

func printScriptContent(out *Printer, data []resultField) {
	fields := fieldMap(data)
	if open, _ := fields["open_in_editor"].(bool); open {
		out.Note("the script is open in the editor, so this is the live buffer")
	}
	if content, ok := fields["content"].(string); ok {
		out.Printf("%s", content)
		if !strings.HasSuffix(content, "\n") {
			out.Printf("\n")
		}
	}
}

func fieldMap(data []resultField) map[string]any {
	m := make(map[string]any, len(data))
	for _, f := range data {
		m[f.name] = f.value
	}
	return m
}

func formatScalar(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case nil:
		return "null"
	case bool:
		return fmt.Sprintf("%v", v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(encoded)
	}
}

func stringList(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	list := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		list = append(list, s)
	}
	return list, true
}

// A plain json.Unmarshal into a map would lose the object's key order, which
// is the order the tool's schema lists its fields in.
func decodeOrderedObject(raw json.RawMessage) ([]resultField, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("not a JSON object")
	}

	var fields []resultField
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("not a JSON object key: %v", keyToken)
		}

		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields = append(fields, resultField{key, value})
	}
	return fields, nil
}
