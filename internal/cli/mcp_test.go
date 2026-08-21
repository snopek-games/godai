package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func TestMCPToolsetsListsAllToolsets(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "mcp", "toolsets"})
	is.NoErr(err)

	for _, name := range core.ToolsetNames {
		is.True(strings.Contains(out, name+":\n")) // every toolset gets a heading
	}
	is.True(strings.Contains(out, "  get_current_scene\n"))     // a remote tool is listed
	is.True(strings.Contains(out, "  install_godot_version\n")) // a local tool is listed
	is.True(strings.Contains(out, "default = "))
	is.True(strings.Contains(out, "(everything except engine)"))
}

func TestMCPToolsetsJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "toolsets"})
	is.NoErr(err)

	var result struct {
		Toolsets []struct {
			Name      string   `json:"name"`
			InDefault bool     `json:"in_default"`
			Tools     []string `json:"tools"`
		} `json:"toolsets"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &result))
	is.Equal(len(result.Toolsets), len(core.ToolsetNames))

	byName := map[string]bool{}
	for _, toolset := range result.Toolsets {
		byName[toolset.Name] = true
		is.True(len(toolset.Tools) > 0)                       // no toolset is empty
		is.Equal(toolset.InDefault, toolset.Name != "engine") // only engine is out of the default set
	}
	for _, name := range core.ToolsetNames {
		is.True(byName[name]) // every toolset is in the JSON
	}
}

func TestMCPUnknownToolsetIsAUsageError(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "--toolsets=bogus"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
	if !strings.Contains(err.Error(), `unknown toolset "bogus"`) {
		t.Errorf("error doesn't name the bad toolset: %v", err)
	}
}
