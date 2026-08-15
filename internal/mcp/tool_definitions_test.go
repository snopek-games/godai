package mcp

import (
	"bytes"
	"encoding/json"
	"path"
	"slices"
	"testing"

	"github.com/matryer/is"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"gitlab.com/snopek-games/godai"
	"gitlab.com/snopek-games/godai/internal/core"
)

const toolsSchemaPath = "addons/godai/tools/default/default_tools.schema.json"

func outputSchemaURL(toolName string) string {
	return "https://godai.test/tools/" + toolName + "/outputSchema.json"
}

func readAddonFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := godai.AddonFS.ReadFile(name)
	if err != nil {
		t.Fatalf("unable to read %s: %v", name, err)
	}
	return b
}

// Both files describe tools in the same shape, so they share a schema. Keyed by
// the path relative to the repository root, which is what '$schema' resolves
// against.
func toolsJSONFiles(t *testing.T) map[string][]byte {
	t.Helper()

	local, err := toolsFS.ReadFile("local_tools.json")
	if err != nil {
		t.Fatalf("unable to read local_tools.json: %v", err)
	}

	return map[string][]byte{
		"addons/godai/tools/default/default_tools.json": readAddonFile(t, "addons/godai/tools/default/default_tools.json"),
		"internal/mcp/local_tools.json":                 local,
	}
}

func TestToolsJSONMatchesSchema(t *testing.T) {
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(readAddonFile(t, toolsSchemaPath)))
	if err != nil {
		t.Fatalf("unable to parse %s: %v", toolsSchemaPath, err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(toolsSchemaPath, schemaDoc); err != nil {
		t.Fatalf("unable to add %s: %v", toolsSchemaPath, err)
	}
	sch, err := c.Compile(toolsSchemaPath)
	if err != nil {
		t.Fatalf("unable to compile %s: %v", toolsSchemaPath, err)
	}

	for name, b := range toolsJSONFiles(t) {
		t.Run(name, func(t *testing.T) {
			inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("unable to parse %s: %v", name, err)
			}
			if err := sch.Validate(inst); err != nil {
				t.Errorf("%s doesn't match the schema:\n%v", name, err)
			}
		})
	}
}

func TestToolOutputSchemasCompile(t *testing.T) {
	for fileName, b := range toolsJSONFiles(t) {
		defs, err := core.LoadToolDefinitions(b)
		if err != nil {
			t.Fatalf("unable to parse %s: %v", fileName, err)
		}

		for name, def := range defs {
			t.Run(path.Join(fileName, name), func(t *testing.T) {
				doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(def.GetOutputSchema()))
				if err != nil {
					t.Fatalf("unable to parse outputSchema: %v", err)
				}

				// '$ref: "#"' resolves against the resource URL, so each tool
				// needs its own.
				url := outputSchemaURL(name)
				c := jsonschema.NewCompiler()
				if err := c.AddResource(url, doc); err != nil {
					t.Fatalf("unable to add outputSchema: %v", err)
				}
				if _, err := c.Compile(url); err != nil {
					t.Errorf("outputSchema doesn't compile: %v", err)
				}
			})
		}
	}
}

func TestToolsetsAreKnown(t *testing.T) {
	toolsPerToolset := map[string]int{}
	for fileName, b := range toolsJSONFiles(t) {
		defs, err := core.LoadToolDefinitions(b)
		if err != nil {
			t.Fatalf("unable to parse %s: %v", fileName, err)
		}

		for name, def := range defs {
			if len(def.Toolsets) == 0 {
				t.Errorf("%s has no toolsets", name)
			}
			for _, toolset := range def.Toolsets {
				if !slices.Contains(core.ToolsetNames, toolset) {
					t.Errorf("%s is in unknown toolset %q", name, toolset)
				}
				toolsPerToolset[toolset]++
			}
		}
	}

	for _, toolset := range core.ToolsetNames {
		if toolsPerToolset[toolset] == 0 {
			t.Errorf("toolset %s has no tools", toolset)
		}
	}
}

// The schema's enum and core.ToolsetNames both list the valid toolsets; this
// keeps them from drifting apart.
func TestToolsetsSchemaEnumMatches(t *testing.T) {
	is := is.New(t)

	var schema struct {
		Properties struct {
			Tools struct {
				AdditionalProperties struct {
					Properties struct {
						Toolsets struct {
							Items struct {
								Enum []string `json:"enum"`
							} `json:"items"`
						} `json:"toolsets"`
					} `json:"properties"`
				} `json:"additionalProperties"`
			} `json:"tools"`
		} `json:"properties"`
	}
	is.NoErr(json.Unmarshal(readAddonFile(t, toolsSchemaPath), &schema))
	is.Equal(schema.Properties.Tools.AdditionalProperties.Properties.Toolsets.Items.Enum, core.ToolsetNames)
}

// The '$schema' key is what points an editor at the schema; a stale relative
// path silently turns off completion and validation while writing tool
// definitions.
func TestToolsJSONSchemaReference(t *testing.T) {
	for name, b := range toolsJSONFiles(t) {
		t.Run(name, func(t *testing.T) {
			is := is.New(t)

			var doc struct {
				Schema string `json:"$schema"`
			}
			is.NoErr(json.Unmarshal(b, &doc))
			is.True(doc.Schema != "") // must declare '$schema'
			is.Equal(path.Join(path.Dir(name), doc.Schema), toolsSchemaPath)
		})
	}
}
