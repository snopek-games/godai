package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
	"github.com/urfave/cli/v3"
)

func TestEditorToolCommandBuildsEveryTool(t *testing.T) {
	is := is.New(t)

	toolCommand := editorToolCommand("")

	forwardable := 0
	for _, def := range core.RemoteToolDefinitions() {
		if !def.DoNotForward {
			forwardable++
		}
	}

	is.True(forwardable > 0)
	is.Equal(len(toolCommand.Commands), forwardable)
}

func TestToolCommandsDisableSliceFlagSeparator(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		if !command.DisableSliceFlagSeparator {
			t.Errorf("tool command %q would split repeated flag values on commas", command.Name)
		}
	}
}

func TestToolCommandsHaveProjectPathFlag(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		if !hasFlag(command, "project-path") {
			t.Errorf("tool command %q is missing --project-path", command.Name)
		}
	}
}

func TestToolCommandsHaveEscapeHatches(t *testing.T) {
	for _, command := range editorToolCommand("").Commands {
		for _, name := range []string{"arg", "args-json", "arg-file", "stdin-json"} {
			if !hasFlag(command, name) {
				t.Errorf("tool command %q is missing --%s", command.Name, name)
			}
		}
	}
}

func TestEditorToolCommandRoutesLifecycleTools(t *testing.T) {
	is := is.New(t)

	is.True(len(editorLifecycleTools) > 0)

	toolCommand := editorToolCommand("")
	for name := range editorLifecycleTools {
		def, ok := core.RemoteToolDefinitions()[name]
		is.True(ok)
		is.True(!def.DoNotForward)

		is.True(toolCommand.Command(name) != nil)
	}
}

func TestEditorToolCommandSkipsNonForwardedTools(t *testing.T) {
	is := is.New(t)

	toolCommand := editorToolCommand("")
	for name, def := range core.RemoteToolDefinitions() {
		if def.DoNotForward {
			is.True(toolCommand.Command(name) == nil)
		}
	}
}

func TestRequiredToolArgumentsAcceptTheEscapeHatches(t *testing.T) {
	is := is.New(t)

	base := []string{"godai", "editor-tool", "set_project_settings", "-p", filepath.Join(t.TempDir(), "nope")}

	err := runQuietly(t, base)
	is.Equal(ExitCodeFor(err), ExitUsage)
	is.True(strings.Contains(err.Error(), "settings"))

	for _, args := range [][]string{
		{"--settings", "application/config/name=\"demo\""},
		{"--args-json", `{"settings":{"application/config/name":"\"demo\""}}`},
		{"--arg", "settings={}"},
	} {
		if code := ExitCodeFor(runQuietly(t, append(base, args...))); code == ExitUsage {
			t.Errorf("%v: rejected as a usage error", args)
		}
	}
}

func hasFlag(command *cli.Command, name string) bool {
	for _, flag := range command.Flags {
		for _, candidate := range flag.Names() {
			if candidate == name {
				return true
			}
		}
	}
	return false
}

func TestRootCommandSet(t *testing.T) {
	is := is.New(t)

	root := Root()
	for _, name := range []string{"mcp", "project", "config", "editor-tool", "editor", "self-update"} {
		if root.Command(name) == nil {
			t.Errorf("root is missing the %q command", name)
		}
	}

	project := root.Command("project")
	for _, name := range []string{"list", "open"} {
		if project.Command(name) == nil {
			t.Errorf("`godai project` is missing the %q command", name)
		}
	}

	editor := root.Command("editor")
	for _, name := range []string{"list", "restart", "close"} {
		if editor.Command(name) == nil {
			t.Errorf("`godai editor` is missing the %q command", name)
		}
	}

	config := root.Command("config")
	if config.Command("init") == nil {
		t.Error("`godai config` is missing the \"init\" command")
	}
	for _, name := range []string{"set", "unset"} {
		if !hasFlag(config, name) {
			t.Errorf("`godai config` is missing --%s", name)
		}
	}

	is.True(strings.Contains(root.Usage, "Godot"))
}
