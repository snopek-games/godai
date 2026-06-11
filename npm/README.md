Godai MCP
=========

An MCP server that lets AI assistants (like Claude) control the
[Godot Engine](https://godotengine.org/) editor: creating and editing scenes,
writing scripts, running projects, and more.

This package is a small launcher that runs the prebuilt `godai-mcp` binary for
your platform, which is installed automatically as an optional dependency.

Usage
-----

With Claude Code:

```
claude mcp add godai -- npx -y @snopek-games/godai-mcp
```

Or in any MCP client that uses a JSON configuration file (Claude Desktop,
Cursor, etc):

```json
{
  "mcpServers": {
    "godai": {
      "command": "npx",
      "args": ["-y", "@snopek-games/godai-mcp"]
    }
  }
}
```

Run `npx -y @snopek-games/godai-mcp --help` to see the available options (path to the Godot
executable, base directory for your projects, etc).

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
