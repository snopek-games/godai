Godai
=====

A CLI for automating the [Godot Engine](https://godotengine.org/) editor -
creating and editing scenes, writing scripts, running projects, and more - plus
an MCP server so AI agents (like Claude) can do the same things.

This package is a small launcher that runs the prebuilt `godai` binary for your
platform, which is installed automatically as an optional dependency.

Usage
-----

As a CLI:

```
npx -y @snopek-games/godai project list
npx -y @snopek-games/godai editor-tool --help
```

As an MCP server, with Claude Code:

```
claude mcp add godai -- npx -y @snopek-games/godai mcp
```

Or in any MCP client that uses a JSON configuration file (Claude Desktop,
Cursor, etc):

```json
{
  "mcpServers": {
    "godai": {
      "command": "npx",
      "args": ["-y", "@snopek-games/godai", "mcp"]
    }
  }
}
```

Run `npx -y @snopek-games/godai --help` to see the available commands, and
`npx -y @snopek-games/godai <command> --help` for any one of them.

Upgrading from `@snopek-games/godai-mcp`
----------------------------------------

This package replaces `@snopek-games/godai-mcp`. The MCP server is now the
`mcp` subcommand rather than what the bare command does, so an existing
configuration needs `"mcp"` added to its arguments.

Supported platforms
-------------------

- Linux x86_64 and arm64
- Windows x86_64 and arm64
- macOS arm64 (Apple Silicon)

Prebuilt binaries can also be downloaded directly from the
[releases page](https://gitlab.com/snopek-games/godai/-/releases) if you'd
rather not use npm.

License
-------

MIT
