# Functional tests: Go MCP server

These tests exercise the Go MCP server (`./cmd/godai-mcp`) end-to-end, by building the
`godai-mcp` binary and drive it over **stdio** - the transport a real MCP
client (Claude, Cursor, ...) uses.

They cover:

- the local tools in `mcp/server/local_tools.go` (`list_projects`,
  `open_godot_project`, `list_open_projects`, `get_mcp_configuration`,
  `set_mcp_configuration`),
- `--global` mode, where projects are discovered from the configured base path
  and Godot's project manager (`projects.cfg`) instead of a root, and
- a smoke test proving a remote tool call is forwarded across to a real Godot
  editor (`get_current_scene`).

All `XDG_*` directories are isolated under a temp tree — including the one the
server reads `projects.cfg` from — so the tests never touch the developer's
real Godot installation or project list.

They deliberately do **not** re-test the editor's own tools — that's the
[addon suite](../addon)'s job.

## Running locally

```sh
go test ./tests/functional/mcp/
```

The harness:

1. Creates a temporary, **bare** Godot project (no addon) under a temp root.
2. Builds `godai-mcp` and starts it over stdio, pointed at that root with
   `--godot-path` set to a small wrapper script that forces `--headless`.
3. Performs the MCP `initialize` handshake, then runs the tests.
4. The editor is started the way it is in production: `open_godot_project`
   installs and enables the addon, spawns Godot via `--godot-path`, and waits
   for it to connect back over WebSocket. The server discovers it through the
   instance file it writes under `XDG_CACHE_HOME/godai-mcp/instances`.
5. On teardown, any editor still advertising itself is killed (the server
   doesn't own the editors it spawns), the server is stopped, and the temp
   directory is removed (kept on failure, with the path printed).

The tests are skipped in `-short` mode, on Windows (the godot wrapper is a
shell script), or when no Godot binary can be found.

## Environment variables

| Variable | Effect |
| --- | --- |
| `GODOT` | Path to (or name of) the Godot binary. Otherwise `godot`, then `godot4`, from `PATH`. |
| `GODAI_TEST_VERBOSE` | Stream the server and editor logs to stderr (they always go to `server.log` / `editor.log` in the temp directory). |
| `GODAI_TEST_KEEP` | Keep the temporary directory even when the tests pass. |

## Note on shared state

All tests share one server process and one editor, so they cannot run in
parallel. Tests that need the editor connected call `ensureProjectOpen`, which
calls `open_godot_project` — the first call spawns the editor, later calls
short-circuit in the server.
