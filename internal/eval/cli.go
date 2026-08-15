package eval

import (
	"fmt"
	"os"
	"path/filepath"
)

// The MCP handshake hands the model godai's instructions; nothing does that for a CLI.
const cliInstructions = `The godai CLI is on your PATH. It drives a running Godot editor: the scene tree, node properties, scripts, resources and project settings. Prefer it over editing .tscn/.tres/.gd files on disk - the editor owns that state and direct file edits can be clobbered or rejected.

Start with ` + "`godai --help`" + ` and ` + "`godai editor-tool --help`" + `, which list what it can do.`

// The shim supplies the flags a developer's config would, so the agent types
// the command a developer would type.
func cliSurface(cfg Config, work *Workspace) (surface, error) {
	shim, err := work.GodaiShim(work.GodaiArgs()...)
	if err != nil {
		return surface{}, fmt.Errorf("install godai shim: %w", err)
	}

	return surface{
		args: append(cliArgs(cfg.FullTools), "--append-system-prompt", cliInstructions),
		env:  []string{"PATH=" + filepath.Dir(shim) + string(os.PathListSeparator) + os.Getenv("PATH")},
	}, nil
}

func cliArgs(fullTools bool) []string {
	tools := "Bash,Read,Glob,Grep"
	if fullTools {
		tools = "Bash,Read,Write,Edit,Glob,Grep"
	}
	return []string{
		"--mcp-config", `{"mcpServers":{}}`,
		"--strict-mcp-config",
		"--allowedTools", tools,
	}
}

// An installed godai earlier on PATH answers against its own config and engine,
// and otherwise looks like a normal run.
func checkShimAnswered(work *Workspace, godaiCalls int) error {
	if godaiCalls == 0 {
		return nil
	}
	calls, err := work.ShimCalls()
	if err != nil {
		return fmt.Errorf("read shim log: %w", err)
	}
	if len(calls) == 0 {
		return fmt.Errorf("%d godai calls, none of them through the shim", godaiCalls)
	}
	return nil
}
