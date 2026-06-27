# Functional tests: addon MCP server

These tests connect to a real Godot editor over the MCP HTTP transport and
exercise the addon's MCP server end-to-end, including the tools in
`addons/godai/tools/default/*_tools.gd`.

## Running locally

```sh
go test ./tests/functional/addon/
```

By default, the harness:

1. Creates a temporary Godot project, copies `addons/godai` into it, and adds
   some fixture scenes/scripts.
2. Launches `godot --editor --headless` against that project, using
   environment variables to force the HTTP transport onto a free port (see
   below).
3. Waits for the MCP server to respond to `initialize`, then runs the tests.
4. Shuts the editor down and deletes the temporary project (it is kept if the
   tests fail, and the path is printed).

The tests are skipped in `-short` mode, or when no Godot binary can be found.

## Environment variables

Harness configuration:

| Variable | Effect |
| --- | --- |
| `GODOT` | Path to (or name of) the Godot binary to launch. Otherwise `godot`, then `godot4`, from `PATH`. |
| `GODAI_TEST_PORT` | Connect to an editor that is already listening on this port instead of launching a headless one. Tests that need to inspect the project directory are skipped in this mode. |
| `GODAI_TEST_VERBOSE` | Stream the editor's log output to stderr (it always goes to `editor.log` in the temp project). |
| `GODAI_TEST_KEEP` | Keep the temporary project directory even when the tests pass. |

The addon itself supports these overrides of its editor settings (there is no
editor-settings equivalent of `override.cfg`, so the harness uses these to
configure the headless editor):

| Variable | Overrides | Values |
| --- | --- | --- |
| `GODAI_MCP_TRANSPORT` | `godai/mcp_transport` | `websocket`, `http`, or the integer enum value |
| `GODAI_MCP_BASE_PORT` | `godai/mcp_base_port` | port number |
| `GODAI_MCP_PORT_COUNT` | `godai/mcp_port_count` | number of ports to try |

## Testing against a running editor

To debug a test against the editor you already have open (with the HTTP
transport enabled in the Godai editor settings):

```sh
GODAI_TEST_PORT=12120 go test -v ./tests/functional/addon/
```

Beware: the tests create scenes and resources inside whatever project that
editor has open, and they will fail on a second run against the same project,
since the files they create will already exist.

## Note on shared editor state

All tests share one editor instance, so they cannot run in parallel. Any test
that needs particular editor state is expected to set that state up itself —
e.g. `TestNoSceneOpen` closes all open scenes first, and tests that operate on
nodes create their own scene. Tests that create files use file names unique to
that test, since the project sticks around for the whole run. Keep that in
mind when adding new tests.
