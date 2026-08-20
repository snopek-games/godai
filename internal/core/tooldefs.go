package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"gitlab.com/snopek-games/godai"
)

var inputSchemaEmpty json.RawMessage = json.RawMessage(`{"type":"object","properties":{}}`)

type ToolDescription []string

type ToolDefinition struct {
	Title                  string          `json:"title"`
	Description            ToolDescription `json:"description"`
	InputSchema            json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema           json.RawMessage `json:"outputSchema,omitempty"`
	DoNotForward           bool            `json:"doNotForward,omitempty"`
	Annotations            map[string]any  `json:"annotations"`
	Toolsets               []string        `json:"toolsets,omitempty"`
	CLIPositionalArguments []string        `json:"cliPositionalArguments,omitempty"`
	CLIResPathArguments    []string        `json:"cliResPathArguments,omitempty"`
}

func (d *ToolDescription) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*d = arr
		return nil
	}

	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*d = []string{one}
		return nil
	}

	return fmt.Errorf("ToolDescription: cannot unmarshal %s", string(data))
}

func (d *ToolDefinition) GetDescription() string {
	return strings.Join(d.Description, "\n")
}

func (d *ToolDefinition) GetInputSchema() json.RawMessage {
	if len(d.InputSchema) == 0 {
		return inputSchemaEmpty
	}
	return d.InputSchema
}

func (d *ToolDefinition) GetOutputSchema() json.RawMessage {
	return d.OutputSchema
}

func LoadToolDefinitions(b []byte) (map[string]*ToolDefinition, error) {
	var data struct {
		Schema string                     `json:"$schema"`
		Tools  map[string]*ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unable to parse tool definitions: %w", err)
	}

	return data.Tools, nil
}

var RemoteToolDefinitions = sync.OnceValue(func() map[string]*ToolDefinition {
	b, err := godai.AddonFS.ReadFile("addons/godai/tools/default/default_tools.json")
	if err != nil {
		panic(fmt.Errorf("unable to read default_tools.json from Godot addon: %w", err))
	}
	tools, err := LoadToolDefinitions(b)
	if err != nil {
		panic(err)
	}
	return tools
})

type ToolListing struct {
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Annotations  map[string]any  `json:"annotations"`
}

const projectPathDescription = "The path to the Godot project. It must already be open in the Godot editor."

func RemoteToolListing() []ToolListing {
	defs := RemoteToolDefinitions()

	list := make([]ToolListing, 0, len(defs))
	for name, def := range defs {
		if def.DoNotForward {
			continue
		}

		inputSchema, err := withProjectPath(def.GetInputSchema())
		if err != nil {
			slog.Error("skipping tool with an unusable inputSchema", "toolName", name, "error", err)
			continue
		}

		list = append(list, ToolListing{
			Name:         name,
			Title:        def.Title,
			Description:  def.GetDescription(),
			InputSchema:  inputSchema,
			OutputSchema: def.GetOutputSchema(),
			Annotations:  BuildAnnotations(def.Title, def.Annotations),
		})
	}

	return list
}

func withProjectPath(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("inputSchema has no properties")
	}
	properties["project_path"] = map[string]any{
		"type":        "string",
		"description": projectPathDescription,
	}

	return json.Marshal(&schema)
}

func BuildAnnotations(title string, ann map[string]any) map[string]any {
	// 'title' is also set on the annotations for backwards compatibility with old MCP clients.
	out := map[string]any{"title": title}

	readOnlyHint, _ := ann["readOnlyHint"].(bool)
	out["readOnlyHint"] = readOnlyHint

	if openWorldHint, ok := ann["openWorldHint"]; ok {
		out["openWorldHint"] = openWorldHint
	} else {
		out["openWorldHint"] = true
	}

	if !readOnlyHint {
		if destructiveHint, ok := ann["destructiveHint"]; ok {
			out["destructiveHint"] = destructiveHint
		} else {
			out["destructiveHint"] = true
		}

		if idempotentHint, ok := ann["idempotentHint"]; ok {
			out["idempotentHint"] = idempotentHint
		} else {
			out["idempotentHint"] = false
		}
	}

	return out
}
