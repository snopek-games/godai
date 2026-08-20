package schemaflag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
	"github.com/urfave/cli/v3"
)

func TestBuildEveryRemoteTool(t *testing.T) {
	is := is.New(t)

	defs := core.RemoteToolDefinitions()
	is.True(len(defs) > 0)

	for name, def := range defs {
		flags, specs, err := Build(def.GetInputSchema())
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		is.Equal(len(flags), len(specs))

		seen := map[string]bool{}
		for _, spec := range specs {
			if seen[spec.Flag] {
				t.Errorf("%s: duplicate flag --%s", name, spec.Flag)
			}
			seen[spec.Flag] = true

			if strings.Contains(spec.Flag, "_") {
				t.Errorf("%s: flag --%s should be kebab-case", name, spec.Flag)
			}
			if spec.Property == ProjectPathProperty {
				t.Errorf("%s: project_path should not get a generated flag", name)
			}
		}
	}
}

func TestBuildRejectsCollidingProperties(t *testing.T) {
	is := is.New(t)

	_, _, err := Build(json.RawMessage(
		`{"type":"object","properties":{"a_b":{"type":"string"},"a-b":{"type":"string"}}}`))
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "--a-b"))
}

func TestCheckRequired(t *testing.T) {
	is := is.New(t)

	schema := json.RawMessage(
		`{"type":"object","properties":{"a_b":{"type":"string"},"c":{"type":"string"}},"required":["a_b"]}`)

	flags, specs, err := Build(schema)
	is.NoErr(err)

	for _, flag := range flags {
		if required, ok := flag.(cli.RequiredFlag); ok {
			is.True(!required.IsRequired())
		}
	}

	is.NoErr(CheckRequired(specs, core.Args{"a_b": json.RawMessage(`"x"`)}))

	err = CheckRequired(specs, core.Args{"c": json.RawMessage(`"x"`)})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "a_b"))
	is.True(strings.Contains(err.Error(), "--a-b"))
}

func TestKinds(t *testing.T) {
	is := is.New(t)

	_, specs, err := Build(json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":     {"type": "string"},
			"enabled":  {"type": "boolean"},
			"count":    {"type": "integer"},
			"ratio":    {"type": "number"},
			"groups":   {"type": "array", "items": {"type": "string"}},
			"props":    {"type": "object", "additionalProperties": {"type": "string"}},
			"nodes":    {"type": "array", "items": {"type": "object"}}
		}
	}`))
	is.NoErr(err)

	kinds := map[string]Kind{}
	for _, spec := range specs {
		kinds[spec.Property] = spec.Kind
	}

	is.Equal(kinds["name"], KindString)
	is.Equal(kinds["enabled"], KindBool)
	is.Equal(kinds["count"], KindInt)
	is.Equal(kinds["ratio"], KindNumber)
	is.Equal(kinds["groups"], KindStringList)
	is.Equal(kinds["props"], KindStringMap)
	is.Equal(kinds["nodes"], KindJSON)
}

func collect(t *testing.T, schema string, argv ...string) core.Args {
	t.Helper()
	is := is.New(t)

	flags, specs, err := Build(json.RawMessage(schema))
	is.NoErr(err)

	var got core.Args
	cmd := &cli.Command{
		Name:  "tool",
		Flags: flags,
		// Mirrors internal/cli; without it "Vector2(1, 2)" is split on its comma.
		DisableSliceFlagSeparator: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			got, err = Collect(cmd, specs)
			return err
		},
	}

	is.NoErr(cmd.Run(context.Background(), append([]string{"tool"}, argv...)))
	return got
}

func TestCollectPreservesLargeIntegers(t *testing.T) {
	is := is.New(t)

	const schema = `{"type":"object","properties":{"payload":{"type":"array","items":{"type":"object"}}}}`
	args := collect(t, schema, "--payload", `{"n":9007199254740993}`)

	raw, err := args.Raw()
	is.NoErr(err)
	is.True(strings.Contains(string(raw), "9007199254740993"))
}

func TestCollectOmitsUnsetFlags(t *testing.T) {
	is := is.New(t)

	const schema = `{"type":"object","properties":{"count":{"type":"integer"},"name":{"type":"string"}}}`

	args := collect(t, schema)
	is.Equal(len(args), 0)

	args = collect(t, schema, "--count", "5")
	is.Equal(len(args), 1)
	is.Equal(string(args["count"]), "5")
}

func TestCollectKinds(t *testing.T) {
	is := is.New(t)

	const schema = `{
		"type": "object",
		"properties": {
			"node_path": {"type": "string"},
			"enabled":   {"type": "boolean"},
			"groups":    {"type": "array", "items": {"type": "string"}},
			"props":     {"type": "object", "additionalProperties": {"type": "string"}}
		}
	}`

	args := collect(t, schema,
		"--node-path", "/root/Player",
		"--enabled",
		"--groups", "a", "--groups", "b",
		"--props", "position=Vector2(1, 2)")

	is.Equal(string(args["node_path"]), `"/root/Player"`)
	is.Equal(string(args["enabled"]), "true")
	is.Equal(string(args["groups"]), `["a","b"]`)

	is.Equal(string(args["props"]), `{"position":"Vector2(1, 2)"}`)
}

func TestCollectAcceptsSnakeCaseAlias(t *testing.T) {
	is := is.New(t)

	args := collect(t, `{"type":"object","properties":{"node_path":{"type":"string"}}}`,
		"--node_path", "/root/Player")
	is.Equal(string(args["node_path"]), `"/root/Player"`)
}

func TestCollectAcceptsSingularAliasForRepeatableFlags(t *testing.T) {
	is := is.New(t)

	const schema = `{
		"type": "object",
		"properties": {
			"node_paths": {"type": "array", "items": {"type": "string"}},
			"properties": {"type": "object", "additionalProperties": {"type": "string"}}
		}
	}`

	args := collect(t, schema,
		"--node-path", "Player", "--node-path", "Enemy",
		"--property", "speed=1.0")

	is.Equal(string(args["node_paths"]), `["Player","Enemy"]`)
	is.Equal(string(args["properties"]), `{"speed":"1.0"}`)
}

func TestSingularAliasSkipsNonRepeatableFlags(t *testing.T) {
	is := is.New(t)

	const schema = `{
		"type": "object",
		"properties": {
			"status":  {"type": "string"},
			"filters": {"type": "array", "items": {"type": "string"}}
		}
	}`

	flags, _, err := Build(json.RawMessage(schema))
	is.NoErr(err)

	aliases := map[string]bool{}
	for _, flag := range flags {
		for _, name := range flag.Names() {
			aliases[name] = true
		}
	}

	is.True(aliases["filter"])
	is.True(!aliases["statu"])
}

// A naming collision is a schema problem to fix, not something to quietly work around.
func TestSingularAliasCollisionIsAnError(t *testing.T) {
	is := is.New(t)

	_, _, err := Build(json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":  {"type": "string"},
			"names": {"type": "array", "items": {"type": "string"}}
		}
	}`))
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "--name"))
	is.True(strings.Contains(err.Error(), "singular"))
}

func TestSchemaDefaultShowsInUsage(t *testing.T) {
	is := is.New(t)

	flags, _, err := Build(json.RawMessage(
		`{"type":"object","properties":{"parent_path":{"type":"string","default":"."}}}`))
	is.NoErr(err)

	doc, ok := flags[0].(cli.DocGenerationFlag)
	is.True(ok)
	is.True(strings.Contains(doc.GetUsage(), "(default: .)"))
}

func TestCollectRejectsInvalidJSON(t *testing.T) {
	is := is.New(t)

	flags, specs, err := Build(json.RawMessage(
		`{"type":"object","properties":{"nodes":{"type":"array","items":{"type":"object"}}}}`))
	is.NoErr(err)

	cmd := &cli.Command{
		Name:                      "tool",
		Flags:                     flags,
		DisableSliceFlagSeparator: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			_, err := Collect(cmd, specs)
			return err
		},
	}

	err = cmd.Run(context.Background(), []string{"tool", "--nodes", "not json"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "--nodes"))
}
