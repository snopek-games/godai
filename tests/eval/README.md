# Evals

Measures how well a model drives Godot through Godai: whether it completes a
task, and which tools it reaches for on the way.

```sh
go run ./cmd/godai-eval run --solution
go run ./cmd/godai-eval run --model sonnet --repeats 3 --out results-sonnet.json
go run ./cmd/godai-eval run --model haiku --ids add-player-sprite --keep-work
go run ./cmd/godai-eval run --model sonnet --surface cli --out results-cli.json
go run ./cmd/godai-eval run --model sonnet --surface editor --out results-editor.json
go run ./cmd/godai-eval run --model sonnet --surface none --out results-baseline.json
go run ./cmd/godai-eval compare results-*.json
go run ./cmd/godai-eval matrix --models sonnet,haiku --surfaces mcp,cli,none
```

- `run` measures one model on one surface and writes a results file
- `compare` builds a set of tables comparing those results
- `matrix` does both, running every model against every surface

The harness builds `./cmd/godai` from the working tree, so an eval measures the
code in front of you rather than whatever `godai` is on `PATH`. Pass `--godai`
to test a released binary instead.

> [!CAUTION]
> The harness runs on Linux and macOS. It deliberately doesn't build for Windows,
> because it relies on Unix process group. On Windows, run it in Docker (see below).

## Flags

| Flag | Effect |
| --- | --- |
| `--model` | Anything `claude --model` takes: `opus`, `sonnet`, `haiku`, or a full id |
| `--surface` | `mcp`, `cli`, `editor`, or `none` for the baseline. See "Surfaces" |
| `--repeats` | Repeats per task; the gap between the pass rate and `pass_all_repeats` is the flakiness |
| `--ids` / `--tags` | Run a subset |
| `--concurrency` | How many attempts to run at once |
| `--full-tools` | Restore the tools a surface holds back. See "Surfaces" |
| `--solution` | Drive the tasks with their `solution.sh` instead of a model. See "Checking the oracles" |
| `--pristine` | Run no agent and expect every task to fail. See "Checking the verifiers" |
| `--out` | Where the results JSON file will be written |
| `--transcripts` | Where the conversations go; defaults to `<out>-transcripts`, empty writes none |
| `--verbose` | Narrate each conversation to stderr as it happens |
| `--keep-work` | Keep the scratch project to poke at after a failure |
| `--bare` | See "Reproducibility" below |
| `--godot` | Godot binary; defaults to whatever `godai engine which` reports |

## Docker

`tests/eval/docker/` packages the harness and everything it needs — `godai`,
`godai-eval`, Godot and Claude Code — into one image:

```sh
./tests/eval/docker/build.sh                 # build (or rebuild) the image
./tests/eval/docker/run.sh run --solution
./tests/eval/docker/run.sh run --model haiku --bare --out results.json
```

`run.sh` hands its arguments to `godai-eval` with the repo mounted at `/work`,
so tasks are read from — and results written to — your working tree. The
binaries come from the image (`GODAI` and `GODOT` point at the baked-in
`godai` and Godot, so nothing is built at runtime): after
changing Go code, rebuild the image. The Godot and Claude Code versions are
pinned; `build.sh --build-arg GODOT_VERSION=...` overrides them.

The container has no OAuth login, so model runs need credentials from the host
environment or a `.env` in the repo root: either `ANTHROPIC_API_KEY` with
`--bare`, or `CLAUDE_CODE_OAUTH_TOKEN` to bill your Claude subscription.

Run `claude setup-token` on the host to get a subscription token.
Subscription runs can't be `--bare`, but the container's home directory is
empty, so no host hooks, plugins or memory reach them anyway. The `editor`
surface still needs `ANTHROPIC_API_KEY` either way.

### Windows

On Windows, run `build.bat` and `run.bat` (instead of the `*.sh` versions).
Otherwise, everything should work the same!

## What one attempt does

1. Copy `tasks/<id>/fixture/` into a scratch directory and init a git repo
   there, to track what the agent changes.
2. Open a **headless, auto-approving** editor on it. This also installs and
   enables the addon, which edits `project.godot`.
3. Commit the git baseline — after step 2, so the addon's own changes don't
   look like the agent touching a protected path.
4. Run the agent in the scratch project with the task's `instruction.md`, with
   Godai reachable only through `--surface` and nothing else.
5. Close the editor with `--skip-save`.
6. Copy `tasks/<id>/verify/` in and run it headlessly.

Two of these steps look like oversights but are deliberate:

- **The editor closes without saving.** If a task asks the agent to save its
  work, the harness must not save it on the way out, or the check is meaningless.
- **The verifier is copied in after the agent is done**, so the agent never
  sees the checks it will be graded against.

## Reading a failure

Every attempt writes its whole conversation to `<out>-transcripts/`. This is on
by default because failures usually don't reproduce: by the time you know you
want the transcript, re-running gives you a different conversation.

```
results-transcripts/
  add-player-sprite-r1.md      # the conversation, rendered to read
  add-player-sprite-r1.jsonl   # what Claude Code emitted, untouched
```

Oracle runs (`--solution`) get a `.md` too, listing the calls `solution.sh` made.

`--verbose` prints the same conversation to stderr as it happens, one line per
tool call, for watching live instead of reading afterwards. With `--concurrency`
above 1 the attempts interleave, so each line carries its task and repeat number.

## Surfaces

Everything else — the fixture, the opened editor, the addon, the verifier — is
identical across surfaces, so `--surface` changes exactly one thing between two
runs of the same task: how the model reaches Godai.

| Surface | Whose agent | The model gets |
| --- | --- | --- |
| `mcp` | Claude Code | Godai as an MCP server, and `Read`, `Glob`, `Grep` |
| `cli` | Claude Code | `Bash` with the `godai` CLI on `PATH`, and `Read`, `Glob`, `Grep` |
| `editor` | Godai's own | the addon's tools, from the chat panel in the Godot editor |
| `none` | Claude Code | `Read`, `Write`, `Edit`, `Glob`, `Grep`, and no way to reach Godai |

### mcp

The default toolset has no shell and no file-editing tools, so the MCP is the
only way to change the project: a failing task means the MCP failed, not that
the model routed around it. `Bash` is denied explicitly rather than just left
off the allowlist, because `dontAsk` mode waves read-only commands through
otherwise, teaching the model that shelling out half-works.

`--full-tools` adds `Bash`, `Write` and `Edit`, making the toolset the `none`
baseline's plus the MCP. That's the honest shape for comparing against `none`:
the cells differ only in whether the MCP is available, and the model is free to
ignore it — `godai_calls` tells you whether it did.

### cli

An MCP server tells the model about its tools and instructions during the
handshake. A CLI has no equivalent, so this surface appends a short system
prompt saying `godai` is on `PATH` and pointing at `godai --help`.

Reading that help is counted as `help_calls`, separate from the tool-selection
metrics. The `mcp` and `editor` surfaces get their tool documentation for free,
so charging the `cli` surface for reading it would skew `first_action_correct`
and `precision`.

The shell is always on — it's how the model reaches `godai` — so here
`--full-tools` adds `Write` and `Edit`, giving the cell the `none` toolset plus
the CLI for the same comparison shape as `mcp`.

### editor

The only surface that doesn't use Claude Code: it runs Godai's own agent — the
one behind the chat panel — which calls the Messages API itself.

The harness drives the panel with environment variables:

| Variable | |
| --- | --- |
| `GODAI_EVAL_PROMPT_FILE` | The prompt, as a file. Writing it is what starts the run |
| `GODAI_EVAL_STREAM_FILE` | Where the conversation is written, one event per line |
| `GODAI_AUTO_APPROVE_TOOLS` | Without this a headless editor denies every tool that needs approval |
| `GODAI_API_KEY` / `GODAI_API_MODEL` | Override the editor settings (`GODAI_API_PROVIDER` and `GODAI_API_URL` exist too) |

The stream file uses the same event format as Claude Code's
`--output-format stream-json`, so the harness scores, renders and narrates a
panel run with the same code it uses for a Claude Code run.

This surface needs `ANTHROPIC_API_KEY` whatever `--bare` says, because the
addon calls the API itself.

### none

`--surface none` runs the same task with no Godai at all. It also removes the
shell, because `godai` is probably on your `PATH`, and a baseline that can call
it isn't a baseline. The model is left with `Read`, `Write`, `Edit`, `Glob` and
`Grep` — it has to write the `.tscn` and the `.gd` by hand.

`--full-tools` gives the shell back. Use it where `godai` isn't installed, like
CI or a container. It makes a stronger baseline, and a fairer one if what you
want to compare against is a developer's ordinary Claude Code setup. The run
warns if it finds a `godai` on `PATH`.

`pass_rate` is the number that matters here. The action stats only confirm the
run really was a baseline: `godai_calls` should be 0 and `bypass_rate` 1.0.

## Checking the oracles

`--solution` runs the same attempt with `solution.sh` in place of the model:
same fixture, same headless editor, same verifier, no API key and no cost. It
exercises everything a task can get wrong except the instruction itself, so
when a task starts failing on every model, run this first. Being free, it also
runs on every CI pipeline.

It exits non-zero as soon as one task fails, because an oracle is a test rather
than a measurement.

The script reaches `godai` through a shim that records every call, so an oracle
run is scored on tool selection like an agent run is. `precision` and `recall`
below 1.0 mean the task's `expected_actions` and its oracle have drifted apart —
one of the two is wrong.

## Checking the verifiers

`--pristine` is the oracle check's inverse: the same fixture, headless editor
and verifiers, but no agent at all. Every task is expected to *fail* — a task
that passes untouched has verifiers that check nothing, so an agent that does
nothing would score on it. Like `--solution` it is free, exits non-zero on the
first offending task, and runs on every CI pipeline.

## Reproducibility

`claude --bare` skips hooks, plugins, `CLAUDE.md` and auto-memory, which is
what you want for a comparable number. It also never reads OAuth or the
keychain, so it needs `ANTHROPIC_API_KEY`. CI passes `--bare`; locally it is
off by default.

Without `--bare`, Claude Code runs with your normal setup and login. The
harness strips `ANTHROPIC_API_KEY` from Claude Code's environment for these
runs, so they bill your subscription rather than the key. It also warns you:
with your hooks, plugins and `CLAUDE.md` in play, the results are useful for
iterating locally but not comparable across machines.

A `.env` in the directory you run from is loaded at startup, so the key can
live there instead of in your shell — it's gitignored.

## Reading the output

`pass_rate` is the headline number: passes over attempts, so with `--repeats`
every repeat weighs the same. `pass_all_repeats` is stricter — the fraction of
*tasks* whose repeats all passed. The gap between the two is the flakiness: a
model that usually manages a task lifts `pass_rate`, but only a model that
reliably manages it moves `pass_all_repeats`. A run where the two are close is
one whose passes you can trust to repeat.

The tool-selection metrics react to smaller changes than either pass rate, so
they are the ones to watch when tuning tool names and descriptions:

| Field | Means |
| --- | --- |
| `first_action_correct` | Was the first Godai call one of the expected ones? The cleanest signal for tool descriptions, since every later call is influenced by what earlier calls returned |
| `recall` | Of the task's `expected_actions`, the fraction the agent used. Low recall means it skipped steps the task was built to elicit; the skipped ones are listed in `missing_actions` |
| `precision` | Of the distinct Godai actions the agent used, the fraction that were expected. Low precision means it wandered through tools the task didn't call for; the extras are listed in `unexpected_actions` |
| `bypass_rate` | Fraction of project-mutating actions that went around Godai. High means plain file editing is easier to reach for than the tools |
| `redundant_calls` | Calls repeated with identical arguments |
| `help_calls` | `godai --help` and friends on the `cli` surface: how much reading it took to get started |
| `error_rate` | Right tool, wrong arguments |

`open_godot_project` is scored as if it were always expected: the harness opens
the editor before the agent starts, and an agent that makes sure of that first
shouldn't score worse than one that assumes it. It counts for neither
`first_action_correct` nor `precision` — except in a task that lists it in
`expected_actions`, where it's scored like any other tool.

Two situations fail an attempt outright instead of scoring it: the Godai MCP
server didn't load, or a `godai` other than the harness's answered on the `cli`
surface. Without this, Claude Code would just solve the task by editing files,
and a broken setup would look like a pass.

## Comparing runs

`godai-eval compare` puts results beside each other in a table. Name the files,
or a directory to read every results file in it.

```sh
go run ./cmd/godai-eval compare results-*.json
go run ./cmd/godai-eval compare results/
go run ./cmd/godai-eval compare --baseline main/ results/
```

It prints a model x surface grid of pass rates, a table per theme (outcome,
tool selection, cost, trouble), and then the per-task grid, which is usually
where a difference between two surfaces shows up. The report is markdown on
stdout; `--out` also writes it to a file.

Results for the same model and surface are merged no matter how many files they
came in, and the summary is recomputed across all of them rather than taken from
any one file.

It calls out two problems that the numbers alone can't show:

- a cell that never ran a task the others did, so its rates are computed over
  a different set of tasks
- a task whose `task_rev` changed between the runs, since a changed task is
  indistinguishable from an effect of whatever you were testing

`--baseline` compares against a second set of results, adding a pass-rate
delta and its p-value, and `--fail-on-regression` exits non-zero when a cell
drops by more than the noise. Significance comes from a two-sided z-test on two
proportions: with a handful of tasks and a few repeats, a swing of several
points is often just noise, and gating on the raw delta would fail good
changes. `--alpha` moves the bar, 0.05 by default.

The `editor` surface reports no cost, so it shows `—` rather than `$0.00`.
Compare tokens across surfaces instead.

## Running a whole matrix

`godai-eval matrix` runs one cell per model/surface pair and finishes with the
same report `compare` would produce.

```sh
go run ./cmd/godai-eval matrix --models sonnet,haiku --surfaces mcp,cli,none
go run ./cmd/godai-eval matrix --repeats 3 --out-dir results/nightly
go run ./cmd/godai-eval matrix --baseline results/main --fail-on-regression
```

Each cell lands in `--out-dir` (`results/` by default) as
`<model>-<surface>.json`, with its transcripts beside it, so the directory can
be handed straight back to `compare` afterwards. `--report` also writes the
report itself to a file.

Cells run one after another; `--concurrency` still applies to the attempts
within a cell. To resume an interrupted matrix, rerun it with `--skip-existing`,
which skips the cells already in `--out-dir` — ^C during a cell reports
whatever finished rather than throwing it away.

It takes the same flags as `run` otherwise, except `--solution`, which pins the
model and surface itself. `--full-tools` applies to every Claude Code cell;
`editor` runs godai's own agent and is unaffected.

On CI, prefer one job per cell — a GitLab `parallel:matrix` over `run`, with a
`compare` job collecting the artifacts. `matrix` is meant for a local machine,
where one process can build `godai` and resolve Godot once for the whole run.

## Writing a task

```
tasks/<id>/
  task.json        # metadata: timeouts, expected actions, protected paths
  instruction.md   # the prompt, phrased the way a user would phrase it
  fixture/         # the Godot project the agent starts from
  verify/verify.gd # end-state check, prints GODAI_VERIFY_JSON:{...}
  solution.sh      # oracle: solves it with the godai CLI, no model involved
```

A task may also add `verify/editor_verify.gd`: a GDScript *snippet* (bare
statements, tab-indented, no `extends`) that the harness runs through
`execute_editor_script` in the agent's editor before closing it, for checks
against state that doesn't survive shutdown — open scripts, the inspected
object, the current scene. It prints the same `GODAI_VERIFY_JSON:{...}` line,
and its checks are merged with `verify.gd`'s; both must pass. The harness makes
this call itself, so it never appears in the agent's action metrics and a task
may still forbid `execute_editor_script` for the agent.

- Phrase the instruction like a user would, and don't name the tools. If the
  instruction has to read like a spec before the task is gradeable, the fix
  belongs in the verifier, not the instruction.
- Verify behavior, not source text. Instantiate the node and drive it rather
  than grepping the GDScript, so a different-but-correct implementation passes.
- Emit several checks. `checks_passed / checks_total` has far more resolution
  than pass/fail, which matters when judging a small change to a tool
  description.
- Give every failing check a `detail` that names the value it saw. It reaches
  `failed_checks` in the results JSON, the transcript, and the comparison
  report, and it is all a reader gets without re-running the task.
- `expected_actions` is scored as a set — precision, recall, and whether the
  first Godai call was one of them — never as a sequence. There is usually more
  than one valid order, and grading a specific one just measures your own
  preference.
- Every task needs a `solution.sh`: `--solution` refuses to run without one. An
  oracle that quietly broke looks exactly like a model regression.
