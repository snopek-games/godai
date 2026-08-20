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

	type parsedProperty struct {
		spec   Spec
		schema propertySchema
	}

	properties := make([]parsedProperty, 0, len(names))
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

		properties = append(properties, parsedProperty{spec, property})
	}

	flags := make([]cli.Flag, 0, len(properties))
	specs := make([]Spec, 0, len(properties))

	// Aliases come in a second pass so every property has claimed its flag before any singular is checked against them.
	for _, p := range properties {
		aliases := aliasesFor(p.spec)
		if singular := singularAlias(p.spec); singular != "" {
			if other, taken := claimed[singular]; taken {
				return nil, nil, fmt.Errorf("property %q needs --%s as its singular alias, but %q already claims it; rename one of them", p.spec.Property, singular, other)
			}
			claimed[singular] = p.spec.Property
			aliases = append(aliases, singular)
		}

		flags = append(flags, newFlag(p.spec, p.schema, aliases))
		specs = append(specs, p.spec)
	}

	return flags, specs, nil
}

// Repeatable flags also answer to the singular of their name, since each occurrence sets exactly one value.
func singularAlias(spec Spec) string {
	if spec.Kind != KindStringList && spec.Kind != KindStringMap {
		return ""
	}
	switch {
	case strings.HasSuffix(spec.Flag, "ies"):
		return strings.TrimSuffix(spec.Flag, "ies") + "y"
	case len(spec.Flag) > 1 && strings.HasSuffix(spec.Flag, "s") && !strings.HasSuffix(spec.Flag, "ss"):
		return strings.TrimSuffix(spec.Flag, "s")
	}
	return ""
}

type propertySchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	Default     any      `json:"default"`
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

func newFlag(spec Spec, property propertySchema, aliases []string) cli.Flag {
	// The schemas mark up code in backticks, which urfave/cli would take as
	// the flag's value placeholder ("--nodes MyMesh:mesh").
	usage := strings.ReplaceAll(property.Description, "`", "")
	if len(spec.Enum) > 0 {
		usage = appendHint(usage, "one of: "+strings.Join(spec.Enum, ", "))
	}

	switch spec.Kind {
	case KindStringMap:
		usage = appendHint(usage, "KEY=VALUE")
	case KindJSON:
		usage = appendHint(usage, "JSON")
	}

	if property.Default != nil {
		usage = appendHint(usage, "default: "+defaultHint(property.Default))
	}
	if spec.Required {
		usage = appendHint(usage, "required")
	}

	switch spec.Kind {
	case KindBool:
		return &cli.BoolFlag{Name: spec.Flag, Aliases: aliases, Usage: usage}
	// Collect omits unset flags, so the editor's own default applies: printing
	// Go's zero value would contradict the schema's description of the default.
	case KindInt:
		return &cli.IntFlag{Name: spec.Flag, Aliases: aliases, Usage: usage, HideDefault: true}
	case KindNumber:
		return &cli.FloatFlag{Name: spec.Flag, Aliases: aliases, Usage: usage, HideDefault: true}
	case KindStringList, KindStringMap:
		// A slice rather than cli.StringMapFlag: the map flag splits values on
		// commas, which mangles Godot variant syntax like "Vector2(1, 2)".
		return &cli.StringSliceFlag{Name: spec.Flag, Aliases: aliases, Usage: usage}
	default:
		return &cli.StringFlag{Name: spec.Flag, Aliases: aliases, Usage: usage}
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

func defaultHint(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
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
