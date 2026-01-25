Godai - AI agent (LLM) integration with the Godot Engine
========================================================

Godai aims to integrate an AI agent (LLM) with the Godot editor, so that you can use natural
language to ask the AI to perform various operations on your project.

Modes of Operation
------------------

Godai is capable of operating in two modes: API mode and MCP mode.

### API mode

In API mode, you can type your prompts into the "AI" panel in the bottom dock of the Godot editor,
and it will connect to a remote API (currently, the Claude/Anthropic API).

In order to use this mode, you need to have setup an API key (which will likely involve entering
credit card information and paying some amount of money) and configuring it in Godot's editor
settings.

#### Pro's

- The chat box is right in the Godot editor
- More control over the AI, which we can use to get somewhat better responses
- Works anywhere Godot works, including with the Godot editor on Android and the Web

#### Con's

- More setup
- Costs money (although, usually very little)
- Only works with the APIs supported by Godai (currently, only Claude/Anthropic - I'd like to add more
  in the future)
- No integration with tools outside of Godot

### MCP mode

In MCP mode, you type your prompts into an MCP client (like Claude Desktop, Cursor, etc) and it will
send commands to the Godot editor.

In order to use this mode, you'll need to download Godai's MCP server and configure your MCP client,
which usually involves editing a JSON configuration file.

#### Pro's

- It can launch the Godot editor, create new projects, and interact with multiple Godot editors at once
- Integration with other tools outside of Godot (probably via their own MCP's)
- Works with any AI that has MCP support
- Potentially no cost (see below)

#### Con's

- Requires using an external tool outside of Godot (the MCP client)
- Less control over the AI, potentially leading to somewhat worse responses
- Only works on desktop platforms (ie Windows, Linux and MacOS)

#### Free?

At the moment, some MCP clients (like Claude Desktop) have a free tier, which can allow you to use Godai
at no cost to you. That is, until the bubble bursts, the investor money dries up, and these AI companies
can no longer operate at a loss :-)

Setting up addon for API mode
-----------------------------

1. Download the Godot addon
2. Extract it into your project (such that `addons/godai` contains the `plugin.cfg` file)
3. Go to **Project** -> **Project Settings...**, switch to the **Plugins** tab, enable "Godai" by
   checking the checkbox
5. Go to **Editor** -> **Editor Settings...**, find the **Godai** section and enter your
   **Anthropic API Key**

To get an **Anthropic API Key**, you need to create a developer account on the
[Claude Console](https://console.anthropic.com/login?returnTo=%2F%3F), and once your logged in:

1. Expand the left sidebar
2. Click **Manage** -> **API keys**
3. Click **+ Create Key**
4. Provide a name (perhaps "godai") and then click **Add**
5. Copy the provided API key (NOTE: this will _NOT_ be saved in the console for you, so you must copy
   it somewhere right away)

However, Godai won't be able to actually use the API key, until you've bought some credits. To do that,
click **Billing** and then **Buy credits**.

The pricing is "per million tokens" where each word or punctuation that the AI processes is approximately
1-3 tokens. So, if you're only testing it out, you won't pay much at all, however, with heavy usage it can
begin to add up. But since credits must be purchased in advance, you are in control of the maximum that
can be spent.

Setting up the MCP
------------------

### Claude Desktop Extension

If your MCP client is Claude Desktop, you can install the MCP as an "extension" from the .MCPB file,
which includes builds for all platforms (Windows, Linux and MacOS) and will automatically configure
Claude Desktop, so you don't need to mess around with JSON files or anything.

1. Download the .MCPB from the [latest release](https://gitlab.com/snopek-games/godai/-/releases)
   of Godai
2. Depending on your operating system, you may be able to double-click the .MCPB file, which will
   open Claude and prompt you to install the extension

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

```js
{
  "mcpServers": {
    "godai": {
      "command": "/path/to/godai-mcp",
      "args": [
        "--godot-path", "/path/to/godot4",
        "--project-path", "/path/to/my/godot/projects"
      ]
    }
  }
}
```

If you're on Linux, and the MCP is having trouble launching Godot, you may need to get the value of
the `$DISPLAY` environment variable on your system (by typing `echo $DISPLAY` in a terminal window)
and provide that with the `--x11-display` option, for example:

```js
{
  "mcpServers": {
    "godai": {
      "command": "/path/to/godai-mcp",
      "args": [
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

I also have fears about the long-term affects of LLMs on society, especially with regard to children
and students. Using AI too much can be a crutch that prevents real learning. Blindly following AI can
lead to an enormous amount of wasted time when it makes stuff up. And you know what, doing the thing
yourself is rewarding and fun - what happens if we all forget that due to our natural human laziness?

So, while Godai may somewhat useful, ultimately, I work on it because it's a cool technology that's
fun to play with. I absolutely enjoy making Godai, much more than actually using it. And, hey, it's
Open Source, so you're free to have the same fun with me :-)

License
-------

Copyright 2025 David Snopek.

Licensed under the [MIT License](LICENSE.txt).
