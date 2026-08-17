Godai - Godot Automation (and AI agent integration)
===================================================

Godai provides Godot automation, including driving the Godot editor.

The `godai` command lets you control Godot from the terminal or CI, and `godai mcp`
exposes those same operations to an AI agent (LLM) over the Model Context Protocol (MCP),
so you can ask in natural language as well.

Works with Godot 4.6 or later.

Features
--------

Through Godai, you can:

- **Manage Godot itself:** download, run and switch between versions of the engine, and pin
  a project to a specific version.
- **Manage projects:** list available projects, open them in new editor instances, and read or change
  project settings.
- **Edit scenes:** create, open, and save scenes, inspect the scene tree, and instantiate saved
  scenes as child nodes.
- **Edit nodes:** add and remove nodes, read and set their properties, manage their groups, and
  connect or disconnect signals.
- **Work with scripts:** create, read, write, attach, and detach GDScript files (with safe,
  conflict-aware writes that respect unsaved changes in the editor).
- **Work with resources:** create `.tres`/`.res` resources, open them in the inspector, and read or
  change their properties.
- **Manage assets:** inspect and change import settings, and reimport assets.
- **Run and debug:** run the project (or the current scene), stop it, and read recent log output
  from the editor.
- **Configure the editor:** read and change editor settings, and restart the editor when needed.
- **Run arbitrary editor scripts:** for anything not covered by the tools above, execute GDScript
  directly in the editor.

Most changes are made through the editor's own undo/redo system, so you can undo what Godai does
just like any other editor action.

> [!CAUTION]
> The AI can create, modify, and delete files in your project, and it occasionally does something
> unexpected. Use version control (or keep a backup), and review its changes before relying on them.

Modes of Operation
------------------

Godai is capable of operating in three modes: **CLI mode**, **API mode** and **MCP mode**.

### CLI mode

In CLI mode, you run `godai` yourself from a terminal or a script. Start by getting a Godot to
run things with:

```bash
godai engine search                      # which versions are there?
godai engine install 4.5                 # download one; the first becomes the default
godai engine use 4.4.1                   # change the default later
```

Then find a project and get an editor running on it:

```bash
godai project list                       # what projects are there?
godai project open ~/games/platformer    # open one in the editor (if not already running)
godai editor list                        # which ones are open right now?
```

Tools that change the project ask for approval in the editor before they run. Add `--auto-approve`
to `godai project open` to launch an editor that runs them without asking. This can be helpful for
`--headless` editors and CI, where nobody can answer the dialog.

Everything we can do in the editor is a "tool", which takes its own set of options:

```bash
godai editor-tool --help
godai editor-tool get_project_settings --help
godai editor-tool get_project_settings -p ~/games/platformer --names application/config/name
```

If you run `godai` within the Godot project, you can drop the `-p ~/games/platformer`.

Let's create and edit a new scene (use `open_scene` to edit a pre-existing scene):

```bash
cd ~/games/platformer

# Create a new scene (the root node is named "Test" after the scene filename).
godai editor-tool create_scene \
  --file-path res://test.tscn \
  --root-node-type Node3D

# Add a MeshInstance3D called "Sphere" with a SphereMesh.
godai editor-tool add_node \
  --parent-path . \
  --node-type MeshInstance3D \
  --properties name=Sphere \
  --properties 'mesh=Object(SphereMesh)'
```

Property values are strings in Godot variant syntax, and `Object(...)` builds a resource to embed.
`set_node_properties` takes an object keyed by node path, so a single call can change as many nodes
as you like (and it becomes a single action in the editor's undo history):

```bash
# Make the sphere green by setting the material.
godai editor-tool set_node_properties \
  --action "Make the sphere green" \
  --nodes '{"Sphere": {"material_override": "Object(StandardMaterial3D,\"albedo_color\":Color(0, 1, 0, 1))"}}'

# Resize the sphere (a colon in the property name reaches inside the resource).
godai editor-tool set_node_properties \
  --action "Enlarge the sphere" \
  --nodes '{"Sphere": {"mesh:radius": "1.0", "mesh:height": "2.0"}}'
```

Scripts work the same way, and an exported variable is just another property once the script is
attached:

```bash
godai editor-tool create_script \
  --file-path res://spin.gd \
  --content 'extends MeshInstance3D

@export var speed := 1.0

func _process(delta: float) -> void:
    rotate_y(speed * delta)
'

godai editor-tool attach_script \
  --node-path Sphere \
  --script-path res://spin.gd

godai editor-tool set_node_properties \
  --action "Slow the spin" \
  --nodes '{"Sphere": {"speed": "0.25"}}'
```

Reading properties back uses those same colon paths. Only properties that differ from their
defaults are shown (unless `--include-defaults` is used):

```bash
godai editor-tool get_node_properties \
  --node-paths Sphere \
  --node-paths Sphere:mesh \
  --node-paths Sphere:material_override
```

Outputs:

```json
{
  "nodes": {
    "Sphere": {
      "material_override": "Object(StandardMaterial3D)",
      "mesh": "Object(SphereMesh)",
      "name": "Sphere",
      "script": "Resource(\"res://spin.gd\")",
      "speed": "0.25"
    },
    "Sphere:material_override": {
      "albedo_color": "Color(0, 1, 0, 1)"
    },
    "Sphere:mesh": {
      "height": "2.0",
      "radius": "1.0"
    }
  }
}
```

To see the whole scene at once:

```bash
godai editor-tool get_current_scene_tree
```

Outputs:

```json
{
  "children": [
    {
      "name": "Sphere",
      "path": "Sphere",
      "script": "res://spin.gd",
      "type": "MeshInstance3D"
    }
  ],
  "name": "Test",
  "path": ".",
  "type": "Node3D"
}
```

So far this has only changed the editor's copy of the scene, exactly as if you'd done it by hand,
so each step can be undone with Ctrl+Z. Saving the scene is its own tool:

```bash
godai editor-tool save_scene
```

To change a script that already exists, read it first: `write_script` refuses to overwrite a
script you haven't read, or one that has changed since you read it, so you can't clobber edits you
haven't seen.

```bash
godai editor-tool read_script --file-path res://spin.gd
godai editor-tool write_script --file-path res://spin.gd --content '...'
```

Two more worth knowing: `get_log_messages` returns the editor's recent output (where a broken
script reports itself), and `execute_editor_script` runs GDScript inside the editor, for anything
the other tools don't cover.

### API mode

In API mode, you can type your prompts into the "Godai" panel in the bottom dock of the Godot editor,
and it will connect to a remote LLM API (currently, the Anthropic API).

In order to use this mode, you need to enter an API key in Godot's editor settings. This will likely
involve entering credit card information to the provider (for example, Anthropic), and paying some
amount of money.

The main advantage of this mode is that it requires only the Godot editor, and it'll work anywhere
that the Godot editor does, including on Android, the Web, or standalone XR devices.

### MCP mode

In MCP mode, you type your prompts into an MCP client (like Claude Code, Cursor, etc) and it will
send commands to the Godot editor.

This only works when running on your desktop or laptop, but it can allow you to take advantage of
other tools or MCP servers (for example,
[using Blender via an MCP server](https://www.blender.org/lab/mcp-server/)) from the same chat
session.

#### Free?

At the moment, some MCP clients (like Claude Code/Desktop) have a free tier, which can allow you
to use Godai at no cost to you. That is, until the bubble bursts, the investor money dries up,
and these AI companies can no longer operate at a loss :-)

Quick Start: CLI mode
---------------------

Install the `godai` binary from the [latest release](https://gitlab.com/snopek-games/godai/-/releases),
run it with `npx -y @snopek-games/godai`, or install it with [Homebrew](https://brew.sh/):

```bash
brew tap snopek-games/godai
brew install godai
```

Run `godai` with no arguments to see the commands, and `godai <command> --help` for any of them.

| Command | What it does |
| ------- | ------------ |
| `godai engine search [filter] [--all]` | Search the Godot versions available to install |
| `godai engine install <version> [--with-templates]` | Download and install a version of Godot, making the first one the default |
| `godai engine list` | List the versions that are installed |
| `godai engine use <version>` | Make a version the default |
| `godai engine which [version]` | Print the path to a version, or to the one that would be used here |
| `godai engine run [version] [-- args...]` | Run Godot, passing anything after `--` straight to it |
| `godai engine remove <version>` | Remove an installed version (`install-templates` and `remove-templates` handle just the templates) |
| `godai engine list-templates [version]` | List the export templates that are installed, or one version's platform by platform |
| `godai engine link <name> <path>` | Give a name to a Godot executable Godai didn't install, recording which version it reports |
| `godai project list` | List the Godot projects available to open |
| `godai project open [path] [--headless] [--auto-approve]` | Open a project in the editor |
| `godai project pin-engine <version>` \| `unpin-engine` | Record in the project which version of Godot it's built with |
| `godai config [setting...]` | Show Godai's own settings, or just the ones you name |
| `godai config --set <setting>=<value>` | Change a setting (`--unset` clears one, `init` sets them up interactively) |
| `godai editor-tool <tool>` | Run one of the tools a running editor provides (`--help` lists them) |
| `godai editor list` | List the projects currently open in an editor |
| `godai editor restart` \| `close` | Control a running editor |
| `godai mcp [--toolsets=...]` | Run the MCP server on stdio, optionally limiting the toolsets it advertises |
| `godai mcp toolsets` | List the toolsets and the tools in each |
| `godai self-update` | Update the binary in place |

Each editor tool is its own subcommand with flags built from that tool's schema, so you can discover
what it takes without reading any JSON:

```bash
godai editor-tool add_node --help
godai editor-tool add_node --node-type Sprite2D --parent-path . --properties position="Vector2(10, 20)"
```

Commands that act on a project work out which one you mean, in this order: the positional `[path]`
argument (on the commands that take one), `--project-path/-p`, the `GODAI_PROJECT_PATH` environment
variable, and then the nearest `project.godot` at or above the working directory.

Which Godot they run is worked out in a similar order: `--godot-path` (or `$GODOT`),
`--godot-version`, the `godot_version` in the project's `.godai.json`, the version `project.godot`
was last saved with, the `godot_version` setting, and then `godot4` or `godot` on `$PATH`.
`godai engine which` prints the answer.

`godai project pin-engine` writes the version to `.godai.json` in the project, next to
`project.godot`. Commit it to your Git repo to ensure you always use the correct version.

Add `--json` to any command to get machine-readable output on stdout (errors go to stderr as JSON
too).

Exit codes distinguish the interesting cases:

- `2`: bad usage
- `3`: not configured
- `4`: no editor connected
- `5`: timed out
- `6`: the editor ran the tool and it failed

> [!NOTE]
> A one-shot command leaves every editor it launched running - shutting one down would mean paying
> for a full project import on the next invocation. Close it with `godai editor close <path>`.

### Shell completion

`godai completion <shell>` prints a tab-completion script for `bash`, `zsh`, `fish`, or `pwsh`
(PowerShell). To enable it:

```bash
# Bash: add to ~/.bashrc
source <(godai completion bash)

# Zsh: add to ~/.zshrc
source <(godai completion zsh)

# Fish: run once
godai completion fish > ~/.config/fish/completions/godai.fish
```

For PowerShell, save the output of `godai completion pwsh` to a `.ps1` file and dot-source it from
your profile.

Quick Start: MCP mode
---------------------

### Claude Code

If you have Node installed (with `npx` available), then you can run:

```bash
claude mcp add godai -- npx -y @snopek-games/godai mcp
```
Then restart Claude Code if it was already running. And that's it!

<details>
<summary><strong>Without Node and <code>npx</code></strong></summary>

If you don't have (or don't want to use) Node/`npx`, you can install with
[Homebrew](https://brew.sh/) (`brew tap snopek-games/godai && brew install godai`) and run:

```bash
claude mcp add godai -- godai mcp
```

Or download a standalone binary for your platform from the
[latest release](https://gitlab.com/snopek-games/godai/-/releases) of Godai,
then give the full path to that instead:

```bash
claude mcp add godai -- /path/to/godai mcp
```
</details>

> [!NOTE]
> You shouldn't have to manually install the Godai addon, it'll get automatically installed if you use
> the MCP server to open your project in the Godot editor. Just ask your AI agent to do it!

### Claude Desktop Extension

If your MCP client is Claude Desktop, you can install the MCP server as an "extension" from the
.MCPB file, which includes builds for all platforms (Windows, Linux, and MacOS) and will automatically
configure Claude Desktop, so you don't need to mess around with JSON files or anything.

1. Download the .MCPB from the [latest release](https://gitlab.com/snopek-games/godai/-/releases)
   of Godai.
2. Depending on your operating system, you may be able to double-click the .MCPB file, which will
   open Claude and prompt you to install the extension.

If this doesn't work (it doesn't for me [on Linux](https://github.com/aaddrick/claude-desktop-debian)),
then you'll need to:

1. Open Claude Desktop
2. Go to **File** -> **Settings...**
3. Switch to the "Extensions" tab and click the "Advanced settings" button
4. Scroll down to the bottom, click the "Install Extension" button, and select the .MCPB file

### Claude Desktop (manual setup)

If you'd prefer to configure Claude Desktop manually, you'll need to edit (or create) the
`claude_desktop_config.json` file. The location of the file depends on your operating system:

| Operating System | Path                                                              |
| ---------------- | ----------------------------------------------------------------- |
| Windows          | `%APPDATA%\Claude\claude_desktop_config.json`                     |
| Linux            | `~/.config/Claude/claude_desktop_config.json`                     |
| MacOS            | `~/Library/Application Support/Claude/claude_desktop_config.json` |

Edit or create that file, and add an entry for Godai, for example:

```jsonc
{
  "mcpServers": {
    "godai": {
      "command": "npx",
      "args": [
        "-y", "@snopek-games/godai", "mcp",
        "--global",
        "--godot-path", "/path/to/godot4",
        "--project-base-path", "/path/to/my/godot/projects"
      ]
    }
  }
}
```

<details>
<summary><strong>Without Node and <code>npx</code></strong></summary>

If you don't have (or don't want to use) Node/`npx`, you can download a standalone binary for your
platform from the [latest release](https://gitlab.com/snopek-games/godai/-/releases) of Godai,
then give the full path to that instead as the `"command"`, and drop the `"-y", "@snopek-games/godai"`
arguments (keeping `"mcp"`).

So, for example:

```jsonc
{
  "mcpServers": {
    "godai": {
      "command": "/path/to/godai",
      "args": [
        "mcp",
        "--global",
        "--godot-path", "/path/to/godot4",
        "--project-base-path", "/path/to/my/godot/projects"
      ]
    }
  }
}
```
</details>

If you're on Linux, and the MCP server is having trouble launching Godot, you may need to get the value
of the `$DISPLAY` environment variable on your system (by typing `echo $DISPLAY` in a terminal window)
and provide that with the `--x11-display` option, for example:

```jsonc
{
  "mcpServers": {
    "godai": {
      "command": "npx",
      "args": [
        "-y", "@snopek-games/godai", "mcp",
        // ... other arguments
        "--x11-display", ":1"
      ]
    }
  }
}
```

See the Claude documentation for more information about
[configuring local MCP servers](https://modelcontextprotocol.io/docs/develop/connect-local-servers).

### Other MCP clients

Most MCP clients are configured using a JSON file similar to the manual Claude Desktop configuration
shown in the previous section.

So, while you'll need to consult the documentation for your chat client, you'll probably be able to
copy and modify the JSON shown above.

Two important notes:

- If your MCP client will run **one instance for your whole computer** (like Claude Desktop does),
  then you'll want to use the `--global` argument.
- Otherwise (like Claude Code, which runs a separate instance per project directory), it will only
  access Godot projects that are in one of its allowed "roots". This will
  use the [MCP Roots feature](https://modelcontextprotocol.io/specification/2025-11-25/client/roots)
  if your client supports it. If not, it'll use the current directory it was spawned from as its
  root. If you want to manually specify the allowed roots, add one or more `--root PATH` arguments.
- The tools for installing and removing Godot versions (the `engine` toolset) aren't advertised by
  default. Add `--toolsets=all` to expose every toolset, or pick just the toolsets you want, like
  `--toolsets=scene,project`. `godai mcp toolsets` lists the toolsets and the tools in each.

Updating the Godai CLI
----------------------

If you're using a standalone binary, it can update itself:

```bash
godai self-update --check     # is there a newer release?
godai self-update             # install it
godai self-update --rollback  # go back to the version the last update replaced
```

Only stable releases are offered; betas and release candidates are skipped. The previous version is
kept next to the executable (as `godai.old`), which is what `--rollback` restores.

`--check` reuses release data fetched within the last day; pass `--no-cache` to ask again right
away. Other commands use the same cached data to mention a newer release when one exists - at most
once a day, on stderr, and only in a terminal. To turn that reminder off, run
`godai config --set update_check=off` or set the `GODAI_NO_UPDATE_CHECK` environment variable.

> [!NOTE]
> If you installed via `npx`/npm, use `npm install -g @snopek-games/godai@latest` instead - npm
> replaces the executable on its next install, so a self-update wouldn't stick. Likewise for
> Homebrew: use `brew update && brew upgrade godai`. Claude Desktop extensions (.MCPB) are
> updated by installing the new .MCPB file. `godai self-update` recognizes these installs and
> points you at the right command instead of updating in place.

The Godai addon inside your projects doesn't need updating separately: the `godai` binary carries a
copy of the matching addon, and reinstalls it in your project when the versions don't match.

An editor that's already running keeps the addon it launched with, though, so after an update the
CLI refuses to drive it (closing it is still allowed). `godai editor restart` fixes that: when it
notices the mismatch, it closes the editor, installs the matching addon, and launches it again.

Quick Start: API mode
---------------------

1. Download the Godai addon from the [latest release](https://gitlab.com/snopek-games/godai/-/releases)
2. Extract it into your project (such that `addons/godai` contains the `plugin.cfg` file)
3. Go to **Project** -> **Project Settings...**, switch to the **Plugins** tab, enable "Godai" by
   checking the checkbox
4. Go to **Editor** -> **Editor Settings...**, find the **Godai** section and enter your
   **Anthropic API Key**

To get an **Anthropic API Key**, you need to create a developer account on the
[Claude Console](https://console.anthropic.com/), and once you're logged in:

1. Expand the left sidebar
2. Click **Manage** -> **API keys**
3. Click **+ Create Key**
4. Provide a name (perhaps "godai") and then click **Add**
5. Copy the provided API key (NOTE: this will _NOT_ be saved in the console for you, so you must copy
   it somewhere right away)

However, Godai won't be able to actually use the API key, until you've bought some credits. To do that,
click **Billing** and then **Buy credits**.

The pricing is "per million tokens" where each word or punctuation mark that the AI processes is roughly
1 token. So, if you're only testing it out, you won't pay much at all, however, with heavy usage it can
begin to add up. But since credits must be purchased in advance, you are in control of the maximum that
can be spent.

Technical Details
-----------------

Godai is made up of two parts:

- **Addon for the Godot editor:** this provides the core functionality of Godai, exposing editor features
  as tools. It's used in all three modes.

- **The `godai` command, written in Go:** this is what you run in CLI mode, and what Claude Code (or
  other MCP client) interacts with in MCP mode. It connects to the addon running in the Godot editor
  using WebSockets. It isn't used in API mode.

This two part design allows `godai` to launch the Godot editor, and interact with multiple Godot
editor instances for different projects.

In MCP mode, it also maintains the connection to the MCP client if the editor restarts. This is
important, because most MCP clients will only connect to your MCP servers at startup, and will give up
on an MCP server if the connection is broken (until the MCP client is restarted).

However, the addon itself does implement a full MCP server! If you go into **Editor Settings** and
change the **Mcp Transport** to **HTTP**, you can connect to it directly from an MCP client.
This is useful when manually testing with the
[MCP Inspector](https://modelcontextprotocol.io/docs/tools/inspector).

"What is this AI trash?!"
-------------------------

It seems like most people are either in love with LLMs (and think _everything_ should be done
with AI!) or hate LLMs and refuse to touch them.

Personally, I have complex and mixed feelings that fall somewhere in the middle.

There are serious open questions with regard to copyright and LLMs, so using an LLM to write
code that you intend to distribute is extremely risky. They also tend to produce pretty mediocre
code in the best case, and insecure, buggy, weirdness in the worst case. I would personally never
use an LLM to write production code. And "vibe coding" - what?

However, I do think LLMs can be a very useful tool. Being able to use an application via natural
human language (especially with speech-to-text) can be a powerful tool for accessibility, and
for beginners first learning to use that application. Also, LLMs are fairly good at producing code
for short, one-off scripts. If the LLM is able to examine and interact with your project without
you needing to explain everything, it can be surprisingly proficient at making changes!

That said, you always need to be careful when using AI. In working on Godai, I've asked it to do the
same handful of operations a hundred times, and it'll do something acceptable 9 out of 10 times. But
that 10th time, it'll do something hilariously weird and unexpected - it's far from perfect.

I also have fears about the long-term effects of LLMs on society, especially with regard to children
and students. Using AI too much can be a crutch that prevents real learning. Blindly following AI can
lead to an enormous amount of wasted time when it makes stuff up. And you know what, doing the thing
yourself is rewarding and fun - what happens if we all forget that due to our natural human laziness?

So, while Godai may be somewhat useful, ultimately, I work on it because it's a cool technology that's
fun to play with. I absolutely enjoy making Godai, much more than actually using it. And, hey, it's
Open Source, so you're free to have the same fun with me :-)

License
-------

Copyright 2025-2026 David Snopek.

Licensed under the [MIT License](LICENSE.txt).
