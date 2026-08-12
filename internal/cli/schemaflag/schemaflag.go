package schemaflag

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/urfave/cli/v3"
)

type Kind int

const (
	KindString Kind = iota
	KindBool
	KindInt
	KindNumber
	KindStringList
	KindStringMap
	KindJSON
)

type Spec struct {
	Property string
	Flag     string
	Kind     Kind
	Required bool
	Enum     []string
}

const ProjectPathProperty = "project_path"

func Build(schema json.RawMessage) ([]cli.Flag, []Spec, error) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &parsed); err != nil {
			return nil, nil, err
		}
	}

	required := map[string]bool{}
	for _, name := range parsed.Required {
		required[name] = true
	}

	names := make([]string, 0, len(parsed.Properties))
	for name := range parsed.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	flags := make([]cli.Flag, 0, len(names))
	specs := make([]Spec, 0, len(names))
	claimed := map[string]string{}

	for _, name := range names {
		if name == ProjectPathProperty {
			continue
		}

		var property propertySchema
		if err := json.Unmarshal(parsed.Properties[name], &property); err != nil {
			return nil, nil, fmt.Errorf("property %q: %w", name, err)
		}

		spec := Spec{
			Property: name,
			Flag:     flagName(name),
			Kind:     kindOf(property),
			Required: required[name],
			Enum:     property.Enum,
		}

		if other, taken := claimed[spec.Flag]; taken {
			return nil, nil, fmt.Errorf("properties %q and %q both map to --%s", other, name, spec.Flag)
		}
		claimed[spec.Flag] = name

		flags = append(flags, newFlag(spec, property))
		specs = append(specs, spec)
	}

	return flags, specs, nil
}

type propertySchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	Items       *struct {
		Type string `json:"type"`
	} `json:"items"`
	AdditionalProperties *struct {
		Type string `json:"type"`
	} `json:"additionalProperties"`
}

func kindOf(p propertySchema) Kind {
	switch p.Type {
	case "string":
		return KindString
	case "boolean":
		return KindBool
	case "integer":
		return KindInt
	case "number":
		return KindNumber
	case "array":
		if p.Items != nil && p.Items.Type == "string" {
			return KindStringList
		}
	case "object":
		if p.AdditionalProperties != nil && p.AdditionalProperties.Type == "string" {
			return KindStringMap
		}
	}
	return KindJSON
}

func flagName(property string) string {
	return strings.ReplaceAll(property, "_", "-")
}

func aliasesFor(spec Spec) []string {
	if spec.Flag == spec.Property {
		return nil
	}
	return []string{spec.Property}
}

func newFlag(spec Spec, property propertySchema) cli.Flag {
	// The schemas mark up code in backticks, which urfave/cli would take as
	// the flag's value placeholder ("--nodes MyMesh:mesh").
	usage := strings.ReplaceAll(property.Description, "`", "")
	if len(spec.Enum) > 0 {
		usage = appendHint(usage, "one of: "+strings.Join(spec.Enum, ", "))
	}

	switch spec.Kind {
	case KindStringList:
		usage = appendHint(usage, "repeatable")
	case KindStringMap:
		usage = appendHint(usage, "KEY=VALUE, repeatable")
	case KindJSON:
		usage = appendHint(usage, "JSON")
	}

	if spec.Required {
		usage = appendHint(usage, "required")
	}

	switch spec.Kind {
	case KindBool:
		return &cli.BoolFlag{Name: spec.Flag, Aliases: aliasesFor(spec), Usage: usage}
	// Collect omits unset flags, so the editor's own default applies: printing
	// Go's zero value would contradict the schema's description of the default.
	case KindInt:
		return &cli.IntFlag{Name: spec.Flag, Aliases: aliasesFor(spec), Usage: usage, HideDefault: true}
	case KindNumber:
		return &cli.FloatFlag{Name: spec.Flag, Aliases: aliasesFor(spec), Usage: usage, HideDefault: true}
	case KindStringList, KindStringMap:
		// A slice rather than cli.StringMapFlag: the map flag splits values on
		// commas, which mangles Godot variant syntax like "Vector2(1, 2)".
		return &cli.StringSliceFlag{Name: spec.Flag, Aliases: aliasesFor(spec), Usage: usage}
	default:
		return &cli.StringFlag{Name: spec.Flag, Aliases: aliasesFor(spec), Usage: usage}
	}
}

func CheckRequired(specs []Spec, args core.Args) error {
	var missing []string
	for _, spec := range specs {
		if spec.Required {
			if _, ok := args[spec.Property]; !ok {
				missing = append(missing, spec.Property)
			}
		}
	}

	switch len(missing) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("missing required argument: %s (set it with --%s)", missing[0], flagName(missing[0]))
	default:
		return fmt.Errorf("missing required arguments: %s", strings.Join(missing, ", "))
	}
}

func appendHint(usage, hint string) string {
	if usage == "" {
		return "(" + hint + ")"
	}
	return usage + " (" + hint + ")"
}

func Collect(cmd *cli.Command, specs []Spec) (core.Args, error) {
	args := core.Args{}

	for _, spec := range specs {
		if !cmd.IsSet(spec.Flag) {
			continue
		}

		raw, err := rawValue(cmd, spec)
		if err != nil {
			return nil, err
		}
		args.SetRaw(spec.Property, raw)
	}

	return args, nil
}

func rawValue(cmd *cli.Command, spec Spec) (json.RawMessage, error) {
	switch spec.Kind {
	case KindBool:
		return json.RawMessage(strconv.FormatBool(cmd.Bool(spec.Flag))), nil

	case KindInt:
		return json.RawMessage(strconv.FormatInt(int64(cmd.Int(spec.Flag)), 10)), nil

	case KindNumber:
		return json.RawMessage(strconv.FormatFloat(cmd.Float(spec.Flag), 'g', -1, 64)), nil

	case KindStringList:
		return json.Marshal(cmd.StringSlice(spec.Flag))

	case KindStringMap:
		pairs := map[string]string{}
		for _, entry := range cmd.StringSlice(spec.Flag) {
			key, value, ok := strings.Cut(entry, "=")
			if !ok {
				return nil, fmt.Errorf("--%s expects KEY=VALUE, got %q", spec.Flag, entry)
			}
			pairs[key] = value
		}
		return json.Marshal(pairs)

	case KindJSON:
		value := cmd.String(spec.Flag)
		if !json.Valid([]byte(value)) {
			return nil, fmt.Errorf("--%s must be valid JSON", spec.Flag)
		}
		return json.RawMessage(value), nil

	default:
		value := cmd.String(spec.Flag)
		if len(spec.Enum) > 0 && !contains(spec.Enum, value) {
			return nil, fmt.Errorf("--%s must be one of: %s", spec.Flag, strings.Join(spec.Enum, ", "))
		}
		return json.Marshal(value)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
