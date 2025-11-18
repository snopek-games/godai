package editor

import (
	"encoding/json"
	"fmt"
	"godai"
	"strings"
	"sync"
)

var inputSchemaEmpty json.RawMessage = json.RawMessage(`{"type":"object","properties":{}}`)

type ToolDescription []string

type ToolDefinition struct {
	Title        string          `json:"title"`
	Description  ToolDescription `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
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

var GetDefaultTools = sync.OnceValue(func() map[string]ToolDefinition {
	tools, err := loadDefaultTools()
	if err != nil {
		panic(err)
	}
	return tools
})

func loadDefaultTools() (map[string]ToolDefinition, error) {
	b, err := godai.AddonFS.ReadFile("addons/godai/tools/default_tools.json")
	if err != nil {
		return nil, fmt.Errorf("unable to read default_tools.json: %w", err)
	}

	var data struct {
		Schema string                    `json:"$schema"`
		Tools  map[string]ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unable to parse default_tools.json: %w", err)
	}

	return data.Tools, nil
}
