# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Godai automates Godot:

- a GDScript editor addon (`addons/godai`) that runs an MCP server inside the editor
  and provides an in-editor chat panel
- a Go CLI (`godai`) that talks to the addon over WebSockets, provides stdio MCP (`godai mcp`),
  and other commands for managing Godot engine versions, projects, editors, etc

The repo root is a Godot project (`project.godot`) so the addon can be tested directly.
Module: `gitlab.com/snopek-games/godai`. Hosted on GitLab; CI is `.gitlab-ci.yml`.

## Commands

```bash
go build ./...                              # build everything
go test -tags selfupdate -short ./...       # unit tests (-short skips functional tests)
go test -run TestName ./internal/cli/       # single test
go test ./tests/functional/mcp/             # functional: Go MCP server over stdio
go test ./tests/functional/cli/             # functional: CLI end-to-end
go test ./tests/functional/addon/           # functional: real editor + addon over MCP HTTP
scripts/go-coverage.sh                      # merged unit + functional coverage report
go run ./cmd/godai                          # run the CLI from the working tree
./local-mcp.sh                              # run the MCP server from the working tree

# GDScript (GUT) tests for the addon (run --import first if .godot/ is stale):
godot --headless --import
godot --headless -s addons/gut/gut_cmdln.gd -gdir=res://tests/gut -gexit

# Evals (see tests/eval/README.md):
go run ./cmd/godai-eval run --solution      # oracle check
go run ./cmd/godai-eval run --model haiku --repeats 3 --out results.json
```

- Functional tests need a Godot binary: `GODOT` env var, else `godot`/`godot4` on `PATH`;
  they skip (not fail) without one
- Functional test suites share one editor/server process, so their tests cannot run in
  parallel and must create uniquely-named files / set up their own editor state.
  Shared helpers live in `tests/functional/internal/harness`.
- Useful test env vars: `GODAI_TEST_VERBOSE=1` (stream editor/server logs),
  `GODAI_TEST_KEEP=1` (keep temp dirs on success), `GODAI_TEST_PORT` (addon suite:
  attach to an already-running editor)

## Architecture

- Editor tools ("remote tools") are defined in `addons/godai/tools/default/default_tools.json`:
  they are implemented in the addon (`addons/godai/tools/default/*_tools.gd`) and the CLI
	**forwards** requests to them from `godai editor-tool ...` subcommands or `godai mcp`
- Local tools are defined in `internal/mcp/local_tools.json` (same format) but
  only implemented in the Go MCP (`internal/mcp/`)
- Adding a forwarded tool = edit `default_tools.json` + implement it in the matching
  `*_tools.gd`; the `godai editor-tool` subcommands, their flags, and the MCP tool lists
  are all derived from the JSON at runtime — no Go changes needed
- The version in `addons/godai/plugin.cfg` is the single source of truth: bumping it
  means editing that file, nothing else

## Conventions

- **Default to no comments.** Prefer descriptive names and simple/obvious code.
  `is.True(...) // human-readable failure note` comments are OK in Go tests
- Long Go strings stay on one line; don't break them with string concatenation unless a
  segment ends in `\n`
- **IMPORTANT: Keep README.md light.** User-facing documentation belongs on
  https://godai.sh (source: gitlab.com/snopek-games/godai-website, usually checked
  out at `../godai-website/`), not in the README
