package core

import (
	"fmt"
	"slices"
	"strings"
)

var ToolsetNames = []string{"project", "scene", "resource", "script", "import", "editor", "engine", "config"}

const DefaultToolsetName = "default"

var nonDefaultToolsets = []string{"engine"}

func DefaultToolsets() []string {
	names := make([]string, 0, len(ToolsetNames))
	for _, name := range ToolsetNames {
		if !slices.Contains(nonDefaultToolsets, name) {
			names = append(names, name)
		}
	}
	return names
}

func ExpandToolsets(requested []string) (map[string]bool, error) {
	if len(requested) == 0 {
		requested = []string{DefaultToolsetName}
	}

	enabled := map[string]bool{}
	for _, name := range requested {
		switch {
		case name == DefaultToolsetName:
			for _, defaultName := range DefaultToolsets() {
				enabled[defaultName] = true
			}
		case slices.Contains(ToolsetNames, name):
			enabled[name] = true
		default:
			return nil, fmt.Errorf("unknown toolset %q; the toolsets are %s, %s", name, DefaultToolsetName, strings.Join(ToolsetNames, ", "))
		}
	}

	return enabled, nil
}

func (d *ToolDefinition) InAnyToolset(enabled map[string]bool) bool {
	for _, name := range d.Toolsets {
		if enabled[name] {
			return true
		}
	}
	return false
}
