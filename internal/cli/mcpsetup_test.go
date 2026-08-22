package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	is.True(len(plan.Register) >= 9) // prefix, binary, and server args
	is.Equal(plan.Register[:7], []string{"claude", "mcp", "add", "--scope", "user", "godai", "--"})
	is.Equal(plan.Register[len(plan.Register)-1], "mcp")
}

func TestMCPSetupScopeIsBakedIntoTheRegistration(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "--scope", "project", "claude-code"})
	is.NoErr(err)

	var plan struct {
		Scope    string   `json:"scope"`
		Register []string `json:"register_command"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(plan.Scope, "project")
	is.Equal(plan.Register[:6], []string{"claude", "mcp", "add", "--scope", "project", "godai"})
}

func TestMCPSetupUnknownScopeIsAUsageError(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "setup", "--scope", "bogus", "claude-code"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
}

func TestMCPSetupUnsupportedScopeIsAUsageError(t *testing.T) {
	is := is.New(t)

	err := runQuietly(t, []string{"godai", "mcp", "setup", "--scope", "local", "codex"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
	is.True(strings.Contains(err.Error(), "user or project")) // says which scopes would work

	err = runQuietly(t, []string{"godai", "mcp", "setup", "--scope", "project", "claude-desktop"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
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

func TestMCPSetupCursorProjectScopeJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "--scope", "project", "cursor"})
	is.NoErr(err)

	var plan struct {
		ConfigPath string `json:"config_path"`
		Deeplink   string `json:"deeplink"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.True(strings.HasSuffix(plan.ConfigPath, filepath.Join(".cursor", "mcp.json")))
	is.True(filepath.IsAbs(plan.ConfigPath)) // reiterates which project directory
	is.Equal(plan.Deeplink, "")              // the deeplink always installs globally
}

func TestMCPSetupVSCodeProjectScopeJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "--scope", "project", "vscode"})
	is.NoErr(err)

	var plan struct {
		Register   []string `json:"register_command"`
		ConfigPath string   `json:"config_path"`
		Config     struct {
			Servers struct {
				Godai struct {
					Command string `json:"command"`
				} `json:"godai"`
			} `json:"servers"`
		} `json:"config"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(len(plan.Register), 0)
	is.True(strings.HasSuffix(plan.ConfigPath, filepath.Join(".vscode", "mcp.json")))
	is.True(plan.Config.Servers.Godai.Command != "") // workspace mcp.json uses "servers", not "mcpServers"
}

func TestMCPSetupCodexProjectScopeJSON(t *testing.T) {
	is := is.New(t)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "--scope", "project", "codex"})
	is.NoErr(err)

	var plan struct {
		Register   []string `json:"register_command"`
		ConfigPath string   `json:"config_path"`
		ConfigTOML string   `json:"config_toml"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.Equal(len(plan.Register), 0)
	is.True(strings.HasSuffix(plan.ConfigPath, filepath.Join(".codex", "config.toml")))
	is.True(strings.HasPrefix(plan.ConfigTOML, "[mcp_servers.godai]\ncommand = \""))
	is.True(strings.Contains(plan.ConfigTOML, "\"mcp\""))
}

func TestMCPSetupYesNeedsAClient(t *testing.T) {
	err := runQuietly(t, []string{"godai", "mcp", "setup", "--yes"})
	if code := ExitCodeFor(err); code != ExitUsage {
		t.Fatalf("got exit code %d, want %d (%v)", code, ExitUsage, err)
	}
}

func fakeClientCLI(t *testing.T, name string, exitCode int) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := fmt.Sprintf("#!/bin/sh\necho added-by-fake\nexit %d\n", exitCode)
	if runtime.GOOS == "windows" {
		path += ".bat"
		script = fmt.Sprintf("@echo added-by-fake\r\n@exit /b %d\r\n", exitCode)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestMCPSetupYesWithJSONRegistersAndReportsIt(t *testing.T) {
	is := is.New(t)

	fakeClientCLI(t, "claude", 0)

	out, err := runCLI(t, []string{"godai", "--json", "mcp", "setup", "--yes", "claude-code"})
	is.NoErr(err)
	is.True(!strings.Contains(out, "added-by-fake")) // the client CLI's output must not pollute the JSON

	var plan struct {
		Registered bool     `json:"registered"`
		Register   []string `json:"register_command"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.True(plan.Registered)
	is.True(len(plan.Register) > 0)
}

func TestMCPSetupYesWithJSONReportsARegistrationFailure(t *testing.T) {
	is := is.New(t)

	fakeClientCLI(t, "claude", 3)

	err := runQuietly(t, []string{"godai", "--json", "mcp", "setup", "--yes", "claude-code"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "added-by-fake")) // the captured output explains the failure
}

func TestMCPSetupYesFailsWhenTheClientCLIIsMissing(t *testing.T) {
	is := is.New(t)

	t.Setenv("PATH", t.TempDir())

	out, err := runCLI(t, []string{"godai", "mcp", "setup", "--yes", "claude-code"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "on your PATH"))
	is.True(strings.Contains(out, "claude mcp add --scope user godai --")) // the command is still shown for running later
	if code := ExitCodeFor(err); code == ExitUsage {
		t.Fatalf("got a usage error, want a plain failure (%v)", err)
	}

	out, err = runCLI(t, []string{"godai", "--json", "mcp", "setup", "--yes", "claude-code"})
	is.True(err != nil)

	var plan struct {
		Registered bool     `json:"registered"`
		Register   []string `json:"register_command"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &plan))
	is.True(!plan.Registered)
	is.True(len(plan.Register) > 0)
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
	is.True(strings.Contains(out, "claude mcp add --scope user godai --"))
	is.True(strings.Contains(out, "restart Claude Code"))
}

func TestGeminiRegisterProtectsDashedArguments(t *testing.T) {
	is := is.New(t)

	is.Equal(geminiRegister("user", "npx", []string{"-y", "@snopek-games/godai", "mcp"}),
		[]string{"gemini", "mcp", "add", "-s", "user", "godai", "npx", "--", "-y", "@snopek-games/godai", "mcp"})
	is.Equal(geminiRegister("project", "/bin/godai", []string{"mcp", "--global"}),
		[]string{"gemini", "mcp", "add", "-s", "project", "godai", "/bin/godai", "mcp", "--", "--global"})
	is.Equal(geminiRegister("user", "/bin/godai", []string{"mcp"}),
		[]string{"gemini", "mcp", "add", "-s", "user", "godai", "/bin/godai", "mcp"})
}

func TestShellJoinQuotesOnlyWhatNeedsIt(t *testing.T) {
	is := is.New(t)

	is.Equal(shellJoin([]string{"claude", "mcp", "add"}), "claude mcp add")
	is.Equal(shellJoin([]string{"/my path/godai", "mcp"}), "'/my path/godai' mcp")
	is.Equal(shellQuote("it's"), `'it'\''s'`)
}
