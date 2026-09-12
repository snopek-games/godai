package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/selfupdate"

	"github.com/urfave/cli/v3"
)

const releasesURL = "https://gitlab.com/snopek-games/godai/-/releases"

type mcpClientDef struct {
	name  string
	label string
	cli   string
}

var mcpClientDefs = []mcpClientDef{
	{name: "claude-code", label: "Claude Code", cli: "claude"},
	{name: "claude-desktop", label: "Claude Desktop"},
	{name: "codex", label: "Codex", cli: "codex"},
	{name: "cursor", label: "Cursor"},
	{name: "gemini", label: "Gemini CLI", cli: "gemini"},
	{name: "vscode", label: "VS Code", cli: "code"},
	{name: "other", label: "your MCP client"},
}

type mcpSetupPlan struct {
	Client     string   `json:"client"`
	Scope      string   `json:"scope"`
	Register   []string `json:"register_command,omitempty"`
	Registered bool     `json:"registered,omitempty"`
	ConfigPath string   `json:"config_path,omitempty"`
	Config     any      `json:"config,omitempty"`
	ConfigTOML string   `json:"config_toml,omitempty"`
	Deeplink   string   `json:"deeplink,omitempty"`
	Extension  string   `json:"extension_url,omitempty"`
}

var mcpScopeNames = []string{"local", "project", "user"}

func mcpSetupCommand() *cli.Command {
	return &cli.Command{
		Name:      "setup",
		Usage:     "connect Godai to an MCP client",
		ArgsUsage: "[client]",
		Description: "Builds the MCP server registration for one of these clients: " + clientNameList() + ".\n\n" +
			"Clients with their own CLI (claude, codex, gemini, code) get a registration command, which is shown first and only run with your approval (--yes skips the question and runs it immediately; with --json it registers silently and reports \"registered\": true in the plan). The others get the exact configuration to paste; no file is ever changed directly.\n\n" +
			"--scope picks who gets the registration: user (all your projects, the default), project (shared with everyone working on the project), or local (this project, just for you). Asking for a scope the client can't do is an error.\n\n" +
			"Options for the server itself (like --global, --root or --toolsets) are baked into the registration when given, for example: godai mcp --toolsets=all setup claude-code",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "scope",
				Value: "user",
				Usage: "registration scope: user (all your projects), project (shared with the project), or local (this project, just for you)",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "skip all prompts: run the registration command right away, with the default scope unless --scope is given",
			},
		},
		ShellComplete: func(_ context.Context, cmd *cli.Command) {
			for _, def := range mcpClientDefs {
				fmt.Fprintln(cmd.Root().Writer, def.name)
			}
		},
		Action: runMCPSetup,
	}
}

func runMCPSetup(ctx context.Context, cmd *cli.Command) error {
	if err := atMostOneArg(cmd, "client"); err != nil {
		return err
	}
	if err := checkDisplayFlags(cmd); err != nil {
		return err
	}

	out := printer(cmd)
	yes := cmd.Bool("yes")
	interactive := !yes && !cmd.Bool("no-input") && !out.JSON && isInteractive()

	prompted := false
	name := cmd.Args().First()
	if name == "" {
		if !interactive {
			return newUsageError("which MCP client? use `godai mcp setup <CLIENT>` with one of: %s", clientNameList())
		}
		chosen, err := promptForClient(ctx)
		if err != nil {
			return err
		}
		name = chosen
		prompted = true
	}

	def, ok := clientDef(name)
	if !ok {
		return newUsageError("unknown MCP client %q; expected one of: %s", name, clientNameList())
	}

	projectDir, err := os.Getwd()
	if err != nil {
		projectDir = "."
	}

	scope := cmd.String("scope")
	supported := supportedScopes(def)
	switch {
	case cmd.IsSet("scope"):
		if !contains(mcpScopeNames, scope) {
			return newUsageError("unknown scope %q; expected one of: %s", scope, strings.Join(mcpScopeNames, ", "))
		}
		if !contains(supported, scope) {
			return newUsageError("%s only supports --scope %s", def.label, strings.Join(supported, " or "))
		}
	case interactive && len(supported) > 1:
		scope, err = promptForScope(ctx, supported, projectDir)
		if err != nil {
			return err
		}
		prompted = true
	}

	serverCmd, serverArgs, err := serverInvocation(cmd, def)
	if err != nil {
		return err
	}

	plan := buildSetupPlan(def, scope, projectDir, serverCmd, serverArgs)

	if len(plan.Register) > 0 {
		if yes {
			if _, err := exec.LookPath(def.cli); err != nil {
				if err := out.Value(plan, func(io.Writer) error {
					printSetupInstructions(out, def, plan)
					return nil
				}); err != nil {
					return err
				}
				return fmt.Errorf("nothing was registered: the %q command isn't on your PATH; install %s, then run the command above", def.cli, def.label)
			}
			if out.JSON {
				if err := registerQuietly(ctx, def, plan.Register); err != nil {
					return err
				}
				plan.Registered = true
			} else {
				return registerNow(ctx, out, def, plan.Register)
			}
		} else if interactive {
			if _, err := exec.LookPath(def.cli); err == nil {
				return offerToRegister(ctx, out, def, plan.Register)
			}
		}
	}

	if prompted {
		out.Printf("\n")
	}
	if err := out.Value(plan, func(io.Writer) error {
		printSetupInstructions(out, def, plan)
		return nil
	}); err != nil {
		return err
	}

	if !out.JSON {
		printSetupNotes(out, def)
	}
	return nil
}

func promptForClient(ctx context.Context) (string, error) {
	names := make([]any, 0, len(mcpClientDefs))
	detected := ""
	for _, def := range mcpClientDefs {
		names = append(names, def.name)
		if detected == "" && def.cli != "" {
			if _, err := exec.LookPath(def.cli); err == nil {
				detected = def.name
			}
		}
	}

	property := map[string]any{
		"type":        "string",
		"description": "Which MCP client do you want to connect Godai to?",
		"enum":        names,
	}
	if detected != "" {
		property["default"] = detected
	}

	answers, err := (&ttyPrompter{}).Prompt(ctx, "Let's connect Godai to your MCP client!", map[string]any{
		"type":       "object",
		"required":   []any{"client"},
		"properties": map[string]any{"client": property},
	})
	if err != nil {
		return "", err
	}

	name, _ := answers["client"].(string)
	return name, nil
}

func supportedScopes(def mcpClientDef) []string {
	switch def.name {
	case "claude-code":
		return []string{"user", "project", "local"}
	case "codex", "cursor", "gemini", "vscode":
		return []string{"user", "project"}
	default:
		return []string{"user"}
	}
}

func promptForScope(ctx context.Context, supported []string, projectDir string) (string, error) {
	explanations := map[string]string{
		"local":   "local: this project only, just for you",
		"project": "project: everyone working on this project, via a file you commit",
		"user":    "user: all your projects",
	}

	lines := make([]string, 0, len(supported)+2)
	lines = append(lines, "Who should get this registration?")
	values := make([]any, 0, len(supported))
	for _, scope := range supported {
		lines = append(lines, "  "+explanations[scope])
		values = append(values, scope)
	}
	lines = append(lines, fmt.Sprintf("(\"this project\" = %s)", projectDir))

	answers, err := (&ttyPrompter{}).Prompt(ctx, "Godai can be registered at more than one scope.", map[string]any{
		"type":     "object",
		"required": []any{"scope"},
		"properties": map[string]any{
			"scope": map[string]any{
				"type":        "string",
				"description": strings.Join(lines, "\n  ") + "\n",
				"enum":        values,
				"default":     "user",
			},
		},
	})
	if err != nil {
		return "", err
	}

	scope, _ := answers["scope"].(string)
	return scope, nil
}

func serverInvocation(cmd *cli.Command, def mcpClientDef) (string, []string, error) {
	toolsets := cmd.StringSlice("toolsets")
	if _, err := core.ExpandToolsets(toolsets); err != nil {
		return "", nil, newUsageError("%v", err)
	}

	roots := cmd.StringSlice("root")
	if cmd.Bool("global") && len(roots) > 0 {
		return "", nil, newUsageError("--global and --root are mutually exclusive")
	}

	command, args := godaiCommand()
	args = append(args, "mcp")

	if def.name == "claude-desktop" || cmd.Bool("global") {
		args = append(args, "--global")
	} else {
		for _, root := range roots {
			args = append(args, "--root", root)
		}
	}
	if len(toolsets) > 0 {
		args = append(args, "--toolsets="+strings.Join(toolsets, ","))
	}
	if cmd.Bool("headless") {
		args = append(args, "--headless")
	}
	if cmd.Bool("offscreen") {
		args = append(args, "--offscreen")
	}
	if cmd.IsSet("offscreen-size") {
		args = append(args, "--offscreen-size", cmd.String("offscreen-size"))
	}
	if cmd.Bool("auto-approve") {
		args = append(args, "--auto-approve")
	}
	if cmd.IsSet("project-base-path") {
		args = append(args, "--project-base-path", cmd.String("project-base-path"))
	}
	if cmd.IsSet("godot-path") {
		args = append(args, "--godot-path", cmd.String("godot-path"))
	}
	if def.name == "claude-desktop" && runtime.GOOS == "linux" {
		args = append(args, "--x11-display", cmd.String("x11-display"))
	}

	return command, args, nil
}

func godaiCommand() (string, []string) {
	exePath, exeErr := selfupdate.ExecutablePath()
	// npm's install path changes with every node or package upgrade, so the
	// registration would break; npx keeps resolving the current install.
	if exeErr == nil && selfupdate.InstallChannel(exePath) == "npm" {
		return "npx", []string{"-y", "@snopek-games/godai"}
	}
	if path, err := exec.LookPath("godai"); err == nil {
		return path, nil
	}
	if exeErr == nil {
		return exePath, nil
	}
	return "godai", nil
}

func buildSetupPlan(def mcpClientDef, scope, projectDir, command string, args []string) mcpSetupPlan {
	plan := mcpSetupPlan{Client: def.name, Scope: scope}

	switch def.name {
	case "claude-code":
		plan.Register = append([]string{"claude", "mcp", "add", "--scope", scope, "godai", "--", command}, args...)
	case "codex":
		if scope == "project" {
			plan.ConfigPath = filepath.Join(projectDir, ".codex", "config.toml")
			plan.ConfigTOML = codexServerTOML(command, args)
		} else {
			plan.Register = append([]string{"codex", "mcp", "add", "godai", "--", command}, args...)
		}
	case "gemini":
		plan.Register = geminiRegister(scope, command, args)
	case "vscode":
		if scope == "project" {
			plan.ConfigPath = filepath.Join(projectDir, ".vscode", "mcp.json")
			plan.Config = map[string]any{
				"servers": map[string]any{
					"godai": mcpServerEntry{Command: command, Args: args},
				},
			}
		} else {
			plan.Register = vscodeRegister(command, args)
		}
	case "other":
		plan.Config = mcpServersConfig(command, args)
	case "claude-desktop":
		plan.ConfigPath = claudeDesktopConfigPath()
		plan.Config = mcpServersConfig(command, args)
		plan.Extension = releasesURL
	case "cursor":
		plan.Config = mcpServersConfig(command, args)
		if scope == "project" {
			plan.ConfigPath = filepath.Join(projectDir, ".cursor", "mcp.json")
		} else {
			plan.ConfigPath = "~/.cursor/mcp.json"
			plan.Deeplink = cursorDeeplink(command, args)
		}
	}

	return plan
}

// gemini takes the command and arguments positionally, with `--` only where
// needed to stop it parsing a dashed server argument as its own option.
func geminiRegister(scope, command string, args []string) []string {
	argv := []string{"gemini", "mcp", "add", "-s", scope, "godai", command}
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			argv = append(argv, "--")
			return append(argv, args[i:]...)
		}
		argv = append(argv, arg)
	}
	return argv
}

type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func vscodeRegister(command string, args []string) []string {
	entry, err := json.Marshal(struct {
		Name string `json:"name"`
		mcpServerEntry
	}{"godai", mcpServerEntry{Command: command, Args: args}})
	if err != nil {
		return nil
	}
	return []string{"code", "--add-mcp", string(entry)}
}

func codexServerTOML(command string, args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, tomlString(arg))
	}
	return fmt.Sprintf("[mcp_servers.godai]\ncommand = %s\nargs = [%s]\n", tomlString(command), strings.Join(quoted, ", "))
}

// JSON string escaping is a subset of TOML basic-string escaping.
func tomlString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func mcpServersConfig(command string, args []string) map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"godai": mcpServerEntry{Command: command, Args: args},
		},
	}
}

func cursorDeeplink(command string, args []string) string {
	config, err := json.Marshal(mcpServerEntry{Command: command, Args: args})
	if err != nil {
		return ""
	}
	return "cursor://anysphere.cursor-deeplink/mcp/install?name=godai&config=" + url.QueryEscape(base64.StdEncoding.EncodeToString(config))
}

func claudeDesktopConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		return `%APPDATA%\Claude\claude_desktop_config.json`
	case "darwin":
		return "~/Library/Application Support/Claude/claude_desktop_config.json"
	default:
		return "~/.config/Claude/claude_desktop_config.json"
	}
}

func offerToRegister(ctx context.Context, out *Printer, def mcpClientDef, argv []string) error {
	message := fmt.Sprintf("This will connect Godai to %s by running:\n\n  %s", def.label, output.Paint(useColor(), output.BoldCyan, shellJoin(argv)))

	answers, err := (&ttyPrompter{}).Prompt(ctx, message, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"run": map[string]any{
				"type":        "boolean",
				"description": "Run this command now?",
				"default":     true,
			},
		},
	})
	if err != nil && !errors.Is(err, core.ErrPromptDeclined) {
		return err
	}

	if run, _ := answers["run"].(bool); err != nil || !run {
		out.Printf("\n")
		out.Printf("Nothing was changed. To connect later, run:\n")
		out.Printf("\n  %s\n", out.Paint(output.BoldCyan, shellJoin(argv)))
		return nil
	}

	return registerNow(ctx, out, def, argv)
}

// The JSON document must be the only thing on stdout, so the client CLI's
// output is captured instead of inherited.
func registerQuietly(ctx context.Context, def mcpClientDef, argv []string) error {
	register := exec.CommandContext(ctx, argv[0], argv[1:]...)
	combined, err := register.CombinedOutput()
	if err != nil {
		return fmt.Errorf("registering with %s: %w: %s", def.label, err, strings.TrimSpace(string(combined)))
	}
	return nil
}

func registerNow(ctx context.Context, out *Printer, def mcpClientDef, argv []string) error {
	register := exec.CommandContext(ctx, argv[0], argv[1:]...)
	register.Stdin = os.Stdin
	register.Stdout = os.Stdout
	register.Stderr = os.Stderr
	if err := register.Run(); err != nil {
		return fmt.Errorf("registering with %s: %w", def.label, err)
	}

	out.Printf("\nGodai is connected to %s. Restart it if it's already running.\n", def.label)
	printSetupNotes(out, def)
	return nil
}

func printSetupInstructions(out *Printer, def mcpClientDef, plan mcpSetupPlan) {
	switch {
	case len(plan.Register) > 0:
		if _, err := exec.LookPath(def.cli); err != nil {
			out.Printf("The %q command isn't on your PATH, so %s needs to be installed first.\n\n", def.cli, def.label)
		}
		out.Printf("To connect Godai to %s, run:\n", def.label)
		out.Printf("\n  %s\n\n", out.Paint(output.BoldCyan, shellJoin(plan.Register)))
		out.Printf("Then restart %s if it's already running.\n", def.label)
	case def.name == "other":
		out.Printf("Most MCP clients accept configuration similar to this (check the documentation for where to put it):\n")
		out.Printf("\n%s\n", out.Paint(output.Cyan, indentJSON(plan.Config)))
		out.Printf("Then restart the client if it's already running.\n")
	default:
		if plan.Deeplink != "" {
			out.Printf("To connect Godai to %s, open this link (your browser will hand it to %s):\n", def.label, def.label)
			out.Printf("\n  %s\n\n", out.Paint(output.BoldCyan, plan.Deeplink))
			out.Printf("Or merge this into %s:\n", plan.ConfigPath)
		} else {
			out.Printf("Merge this into %s:\n", plan.ConfigPath)
		}
		if plan.ConfigTOML != "" {
			out.Printf("\n%s\n", out.Paint(output.Cyan, indentLines(plan.ConfigTOML)))
		} else {
			out.Printf("\n%s\n", out.Paint(output.Cyan, indentJSON(plan.Config)))
		}
		out.Printf("Then restart %s if it's already running.\n", def.label)
		if def.name == "codex" {
			out.Printf("\nCodex only reads project config from trusted projects; approve this project in Codex if the server doesn't appear.\n")
		}
		if def.name == "claude-desktop" && runtime.GOOS == "linux" {
			out.Printf("\nIf launching Godot fails, check `echo $DISPLAY` in a terminal and adjust the --x11-display argument to match.\n")
		}
	}
}

func printSetupNotes(out *Printer, def mcpClientDef) {
	fmt.Fprintln(out.Err)
	out.Note("not all toolsets are enabled by default; add --toolsets=all to the `godai mcp` arguments to expose everything (`godai mcp toolsets` lists them)")
	switch def.name {
	case "claude-code", "codex", "gemini", "vscode":
		out.Note("each godai instance only touches Godot projects under the client's MCP roots (or its working directory); add --root PATH arguments to allow other locations")
	case "claude-desktop":
		out.Note("--global lets this single instance reach any Godot project; add --project-base-path PATH to say where your projects usually live")
	case "other":
		out.Note("if your client runs one instance for your whole computer (like Claude Desktop), add --global to the arguments; otherwise godai only touches Godot projects under the client's MCP roots, its working directory, and --root PATH arguments")
	}
}

func indentJSON(v any) string {
	data, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return ""
	}
	return "  " + string(data) + "\n"
}

func indentLines(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", "\n  ") + "\n"
}

func shellJoin(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\n\"'`$&|;<>()*?[]#~%{}\\") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func clientDef(name string) (mcpClientDef, bool) {
	for _, def := range mcpClientDefs {
		if def.name == name {
			return def, true
		}
	}
	return mcpClientDef{}, false
}

func clientNameList() string {
	names := make([]string, 0, len(mcpClientDefs))
	for _, def := range mcpClientDefs {
		names = append(names, def.name)
	}
	return strings.Join(names, ", ")
}
