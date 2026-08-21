package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestMCPSetupUnknownClientIsAUsageError(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "setup", "bogus"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "claude-code") {
		t.Errorf("error doesn't list the clients: %v", err)
	}
}

func TestMCPSetupWithoutTerminalNeedsAClient(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "setup"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
}

func TestMCPSetupRejectsExtraArguments(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "setup", "claude-code", "cursor"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
}

func TestMCPSetupClaudeCodeJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "claude-code"})
	is.NoErr(err)

	var plan struct {
		Client   string   `json:"client"`
		Register []string `json:"register_command"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Client, "claude-code")
	is.True(len(plan.Register) >= 7) // prefix, binary, and server args
	is.Equal(plan.Register[:5], []string{"claude", "mcp", "add", "godai", "--"})
	is.Equal(plan.Register[len(plan.Register)-1], "mcp")
}

func TestMCPSetupBakesServerFlagsIntoTheRegistration(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "--global", "mcp", "--toolsets=all", "setup", "codex"})
	is.NoErr(err)

	var plan struct {
		Register []string `json:"register_command"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Register[:5], []string{"codex", "mcp", "add", "godai", "--"})
	is.Equal(plan.Register[len(plan.Register)-2], "--global")
	is.Equal(plan.Register[len(plan.Register)-1], "--toolsets=all")
}

func TestMCPSetupUnknownToolsetIsAUsageError(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "--toolsets=bogus", "setup", "claude-code"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
}

func TestMCPSetupGlobalAndRootAreMutuallyExclusive(t *testing.T) {
	err := runQuietly(t, []string{"godai", "--global", "--root", "/some/path", "mcp", "setup", "claude-code"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
}

func TestMCPSetupClaudeDesktopJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "claude-desktop"})
	is.NoErr(err)

	var plan struct {
		Client     string `json:"client"`
		ConfigPath string `json:"config_path"`
		Config     struct {
			MCPServers struct {
				Godai struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"godai"`
			} `json:"mcpServers"`
		} `json:"config"`
		Extension string `json:"extension_url"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Client, "claude-desktop")
	is.True(strings.Contains(plan.ConfigPath, "claude_desktop_config.json"))
	is.True(plan.Config.MCPServers.Godai.Command != "")
	is.True(contains(plan.Config.MCPServers.Godai.Args, "--global")) // a desktop app has no working directory
	is.Equal(plan.Extension, releasesURL)
}

func TestMCPSetupCursorJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "cursor"})
	is.NoErr(err)

	var plan struct {
		Client     string `json:"client"`
		ConfigPath string `json:"config_path"`
		Deeplink   string `json:"deeplink"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Client, "cursor")
	is.Equal(plan.ConfigPath, "~/.cursor/mcp.json")
	is.True(strings.HasPrefix(plan.Deeplink, "cursor://anysphere.cursor-deeplink/mcp/install?name=godai&config="))
}

func TestMCPSetupVSCodeJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "vscode"})
	is.NoErr(err)

	var plan struct {
		Client   string   `json:"client"`
		Register []string `json:"register_command"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Client, "vscode")
	is.Equal(len(plan.Register), 3)
	is.Equal(plan.Register[:2], []string{"code", "--add-mcp"})

	var entry struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	is.NoErr(json.Unmarshal([]byte(plan.Register[2]), &entry))
	is.Equal(entry.Name, "godai")
	is.True(entry.Command != "")
	is.True(contains(entry.Args, "mcp"))
}

func TestMCPSetupOtherPrintsTheGenericConfig(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "other"})
	is.NoErr(err)

	var plan struct {
		Client   string   `json:"client"`
		Register []string `json:"register_command"`
		Config   struct {
			MCPServers struct {
				Godai struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"godai"`
			} `json:"mcpServers"`
		} `json:"config"`
		ConfigPath string `json:"config_path"`
		Deeplink   string `json:"deeplink"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Client, "other")
	is.Equal(len(plan.Register), 0)
	is.Equal(plan.ConfigPath, "")
	is.Equal(plan.Deeplink, "")
	is.True(plan.Config.MCPServers.Godai.Command != "")
	is.True(contains(plan.Config.MCPServers.Godai.Args, "mcp"))

	human, err := runCLI(t, []string{"godai", "mcp", "setup", "other"})
	is.NoErr(err)
	is.True(strings.Contains(human, `"mcpServers"`))
	is.True(strings.Contains(human, `"godai"`))
}

func TestMCPSetupPrintsTheCommandWithoutATerminal(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "mcp", "setup", "claude-code"})
	is.NoErr(err)
	is.True(strings.Contains(out, "claude mcp add godai --"))
	is.True(strings.Contains(out, "restart Claude Code"))
}

func TestGeminiRegisterProtectsDashedArguments(t *testing.T) {
	is := is.New(t)

	is.Equal(geminiRegister("npx", []string{"-y", "@snopek-games/godai", "mcp"}),
		[]string{"gemini", "mcp", "add", "godai", "npx", "--", "-y", "@snopek-games/godai", "mcp"})
	is.Equal(geminiRegister("/bin/godai", []string{"mcp", "--global"}),
		[]string{"gemini", "mcp", "add", "godai", "/bin/godai", "mcp", "--", "--global"})
	is.Equal(geminiRegister("/bin/godai", []string{"mcp"}),
		[]string{"gemini", "mcp", "add", "godai", "/bin/godai", "mcp"})
}

func TestShellJoinQuotesOnlyWhatNeedsIt(t *testing.T) {
	is := is.New(t)

	is.Equal(shellJoin([]string{"claude", "mcp", "add"}), "claude mcp add")
	is.Equal(shellJoin([]string{"/my path/godai", "mcp"}), "'/my path/godai' mcp")
	is.Equal(shellQuote("it's"), `'it'\''s'`)
}
