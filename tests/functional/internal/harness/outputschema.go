package harness

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type outputSchemaSet struct {
	schemas map[string]*jsonschema.Schema
}

func compileOutputSchemas(defs []ToolDef) (*outputSchemaSet, error) {
	set := &outputSchemaSet{schemas: make(map[string]*jsonschema.Schema, len(defs))}

	for _, def := range defs {
		if def.OutputSchema == nil {
			continue
		}

		b, err := json.Marshal(def.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %s: re-encoding outputSchema: %w", def.Name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("tool %s: outputSchema: %w", def.Name, err)
		}

		// '$ref: "#"' resolves against the resource URL, so each tool needs its own.
		url := "https://godai.test/tools/" + def.Name + "/outputSchema.json"
		c := jsonschema.NewCompiler()
		if err := c.AddResource(url, doc); err != nil {
			return nil, fmt.Errorf("tool %s: outputSchema: %w", def.Name, err)
		}
		schema, err := c.Compile(url)
		if err != nil {
			return nil, fmt.Errorf("tool %s: outputSchema: %w", def.Name, err)
		}

		set.schemas[def.Name] = schema
	}

	return set, nil
}

func (s *outputSchemaSet) validate(toolName string, result *ToolCallResult) error {
	schema, ok := s.schemas[toolName]
	if !ok {
		return nil
	}

	if len(result.StructuredContent) == 0 {
		// A failure can be reported as text alone, but a successful call has to
		// return the structured result it promised.
		if result.IsError {
			return nil
		}
		return fmt.Errorf("tool %s declares an outputSchema but returned no structuredContent", toolName)
	}

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(result.StructuredContent))
	if err != nil {
		return fmt.Errorf("tool %s: unparsable structuredContent %q: %w", toolName, result.StructuredContent, err)
	}
	if err := schema.Validate(inst); err != nil {
		return fmt.Errorf("tool %s: structuredContent doesn't match its outputSchema:\n%w", toolName, err)
	}

	return nil
}
