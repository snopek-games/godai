package server

import (
	"bytes"
	"encoding/json"
	"path"
	"testing"

	"github.com/matryer/is"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"gitlab.com/snopek-games/godai"
)

const toolsSchemaPath = "addons/godai/tools/default/default_tools.schema.json"

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
		"mcp/server/local_tools.json":                   local,
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
