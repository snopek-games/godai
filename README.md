Godai - AI agent (LLM) integration with the Godot Engine
========================================================

Godai aims to integrate an AI agent (LLM) with the Godot editor, so that you can use natural
language to ask the AI to perform various operations on your project.

Works with Godot 4.6 or later.

Features
--------

Through Godai, the AI can drive the Godot editor on your behalf. It can:

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

Most changes are made through the editor's own undo/redo system, so you can undo what the AI does
just like any other editor action.

> [!CAUTION]
> The AI can create, modify, and delete files in your project, and it occasionally does something
> unexpected. Use version control (or keep a backup), and review its changes before relying on them.

Modes of Operation
------------------

Godai is capable of operating in two modes: **API mode** and **MCP mode**.

### API mode

In API mode, you can type your prompts into the "AI" panel in the bottom dock of the Godot editor,
and it will connect to a remote API (currently, the Anthropic API).

In order to use this mode, you need to have an API key (which will likely involve entering
credit card information and paying some amount of money) and configuring it in Godot's editor
settings.

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

Quick Start: MCP mode
---------------------

### Claude Code

If you have Node installed (with `npx` available), then you can run:

```bash
claude mcp add godai -- npx -y @snopek-games/godai-mcp
```
Then restart Claude Code if it was already running. And that's it!

<details>
<summary><strong>Without Node and <code>npx</code></strong></summary>

If you don't have (or don't want to use) Node/`npx`, you can download a standalone binary for your
platform from the [latest release](https://gitlab.com/snopek-games/godai/-/releases) of Godai,
then give the full path to that instead:

```bash
claude mcp add godai -- /path/to/godai-mcp
```
</details>

In order to specify the path to Godot:

```bash
claude mcp add godai -- npx -y @snopek-games/godai-mcp --godot-path /path/to/godot4
```

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
        "-y", "@snopek-games/godai-mcp",
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
then give the full path to that instead as the `"command"`, and drop the `"-y", "@snopek-games/godai-mcp"`
argument.

So, for example:

```jsonc
{
  "mcpServers": {
    "godai": {
      "command": "/path/to/godai-mcp",
      "args": [
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
        "-y", "@snopek-games/godai-mcp",
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

### Updating the MCP server

If you're using a standalone binary, it can update itself:

```bash
godai-mcp self-update --check   # is there a newer release?
godai-mcp self-update           # install it
godai-mcp self-update --rollback  # go back to the version the last update replaced
```

Only stable releases are offered; betas and release candidates are skipped. The previous version is
kept next to the executable (as `godai-mcp....old`), which is what `--rollback` restores.

> [!NOTE]
> If you installed via `npx`/npm, use `npm install -g @snopek-games/godai-mcp@latest` instead - npm
> replaces the executable on its next install, so a self-update wouldn't stick. Claude Desktop
> extensions (.MCPB) are updated by installing the new .MCPB file.

The Godai addon inside your projects doesn't need updating separately: the MCP server carries a copy
of the matching addon, and reinstalls it in your project when the versions don't match.

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
  to the AI agent. It's used in both the API and MCP mode.

- **Command-line MCP server written in Go:** this is what Claude Code (or other MCP client) interacts
  with when using MCP mode. It connects to the addon running in the Godot editor using WebSockets.
  This is only used in MCP mode.

This two part design allows the MCP server to launch the Godot editor, and interact with multiple Godot
editor instances for different projects.

It also maintains the connection to the MCP client if the editor restarts. This is important, because
most MCP clients will only connect to your MCP servers at startup, and will give up on an MCP server if
the connection is broken (until the MCP client is restarted).

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
