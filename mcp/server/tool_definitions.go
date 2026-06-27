package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"gitlab.com/snopek-games/godai"
	"strings"
	"sync"
)

//go:embed local_tools.json
var toolsFS embed.FS

var inputSchemaEmpty json.RawMessage = json.RawMessage(`{"type":"object","properties":{}}`)

type ToolDescription []string

type ToolDefinition struct {
	Title        string          `json:"title"`
	Description  ToolDescription `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	DoNotForward bool            `json:"doNotForward,omitempty"`
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

var GetDefaultRemoteToolDefinitions = sync.OnceValue(func() map[string]*ToolDefinition {
	tools, err := loadDefaultRemoteTools()
	if err != nil {
		panic(err)
	}
	return tools
})

var GetLocalToolDefinitions = sync.OnceValue(func() map[string]*ToolDefinition {
	tools, err := loadLocalTools()
	if err != nil {
		panic(err)
	}
	return tools
})

func loadToolsJSON(b []byte) (map[string]*ToolDefinition, error) {
	var data struct {
		Schema string                     `json:"$schema"`
		Tools  map[string]*ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unable to parse default_tools.json: %w", err)
	}

	return data.Tools, nil
}

func loadDefaultRemoteTools() (map[string]*ToolDefinition, error) {
	b, err := godai.AddonFS.ReadFile("addons/godai/tools/default/default_tools.json")
	if err != nil {
		return nil, fmt.Errorf("unable to read default_tools.json from Godot addon: %w", err)
	}
	return loadToolsJSON(b)
}

func loadLocalTools() (map[string]*ToolDefinition, error) {
	b, err := toolsFS.ReadFile("local_tools.json")
	if err != nil {
		return nil, fmt.Errorf("unable to read local_tools.json: %w", err)
	}
	return loadToolsJSON(b)
}
