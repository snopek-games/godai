package core

import (
	"encoding/json"
	"strings"

	"gitlab.com/snopek-games/godai/internal/jsonrpc"
)

// Args holds tool arguments as the exact JSON bytes they arrived as.
//
// Keeping them as bytes is what makes forwarding lossless: decoding into a
// map[string]any would turn every number into a float64, and re-encoding that
// silently rounds anything past 2^53.
type Args map[string]json.RawMessage

func ParseArgs(raw json.RawMessage) (Args, error) {
	if len(raw) == 0 {
		return Args{}, nil
	}

	var args Args
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, NewUserError("unable to parse tool arguments", err, nil)
	}
	if args == nil {
		args = Args{}
	}

	return args, nil
}

func SplitProjectPath(raw json.RawMessage) (string, Args, error) {
	if len(raw) == 0 {
		return "", nil, NewUserError("unable to parse tool arguments", nil, nil)
	}

	args, err := ParseArgs(raw)
	if err != nil {
		return "", nil, err
	}

	projectPath, ok, err := args.String("project_path")
	if err != nil {
		return "", nil, NewUserError("project_path argument must be a string", err, nil)
	}
	if !ok {
		return "", nil, NewUserError("project_path argument is required", nil, nil)
	}

	args.Delete("project_path")

	return projectPath, args, nil
}

func (a Args) String(name string) (string, bool, error) {
	raw, ok := a[name]
	if !ok {
		return "", false, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, err
	}

	return value, true, nil
}

func (a Args) Bool(name string) (bool, bool, error) {
	raw, ok := a[name]
	if !ok {
		return false, false, nil
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, true, err
	}

	return value, true, nil
}

func (a Args) Set(name string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	a[name] = json.RawMessage(b)
	return nil
}

func (a Args) SetRaw(name string, raw json.RawMessage) {
	a[name] = raw
}

func (a Args) Delete(name string) {
	delete(a, name)
}

func (a Args) Raw() (json.RawMessage, error) {
	if a == nil {
		return json.RawMessage("{}"), nil
	}
	b, err := json.Marshal(map[string]json.RawMessage(a))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolResult struct {
	Raw               json.RawMessage `json:"-"`
	Unparsed          bool            `json:"-"`
	Content           []TextContent   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

func (r *ToolResult) Text() string {
	parts := make([]string, 0, len(r.Content))
	for _, c := range r.Content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// The text content of a failed tool call is the whole structured result
// serialized, so the message worth showing has to come out of the structure.
func (r *ToolResult) ErrorMessage() string {
	var structured struct {
		Error string `json:"error"`
	}
	if len(r.StructuredContent) > 0 {
		if err := json.Unmarshal(r.StructuredContent, &structured); err == nil && structured.Error != "" {
			return structured.Error
		}
	}
	return r.Text()
}

type EditorRPCError struct {
	RPC *jsonrpc.Error
}

func (e *EditorRPCError) Error() string {
	if e.RPC == nil {
		return "unknown error from the Godot editor"
	}
	return e.RPC.Message
}
