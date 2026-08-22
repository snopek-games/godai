Godai - Godot automation (and AI agent integration)
===================================================

<img src="./assets/godai-logo.svg" align="left" width="200" />

**Godai provides Godot automation, including driving the Godot editor.**

The `godai` command lets you control Godot from the terminal or CI, and `godai mcp`
exposes those same operations to an AI agent over the Model Context Protocol (MCP),
so you can connect it to tools like Claude Code, Codex, etc.

Godai is also available as a Godot addon: it adds a chat panel inside the Godot editor
where you can collaborate with an AI agent on your project using natural language.

<br clear="left" />

> [!IMPORTANT]
> The canonical source of Godai is its [GitLab project](https://gitlab.com/snopek-games/godai).
> There is a [read-only mirror on GitHub](https://github.com/snopek-games/godai) for convenience.
> You're welcome to submit PRs on GitHub, but they may get less attention.

Features
--------

![Godai CLI demo](./assets/cli-demo.gif)

Through Godai, you or an AI agent can:

- **Manage Godot versions:** download, run and switch between versions of the engine, and pin
  a project to a specific version.
- **Manage projects:** list available projects, open them in the Godot editor, and read or change
  project settings.
- **Edit scenes:** create, open, and save scenes, inspect the scene tree, and instantiate saved
  scenes as child nodes.
- **Edit nodes:** add and remove nodes, read and set their properties, manage their groups, and
  connect or disconnect signals.
- **Work with scripts:** create, read, write, attach, and detach GDScript files, with safe,
  conflict-aware writes that respect unsaved changes in the editor.
- **Work with resources:** create `.tres`/`.res` resources, open them in the inspector, and read or
  change their properties.
- **Manage assets:** inspect and change import settings, and reimport assets.
- **Run and debug:** run the project (or the current scene), stop it, and read recent log output
  from the editor.
- **Configure the editor:** read and change editor settings, and restart the editor when needed.
- **Run arbitrary editor scripts:** for anything not covered by the tools above, execute GDScript
  directly in the editor.

> [!NOTE]
> Most changes are made through the editor's own undo/redo system, so you can undo what Godai does
> just like any other editor action.

> [!CAUTION]
> AI agents can create, modify, and delete files in your project, and can occasionally do something
> unexpected. Use version control (or keep a backup), and review their changes before relying on them.

Documentation
-------------

Godai's documentation is available at [godai.sh/docs](https://godai.sh/docs).

Additionally, the command-line reference documentation can be viewed with `godai --help`.

See ["Configuring your MCP client"](https://godai.sh/docs/mcp) for how to connect Godai with
Claude Code, Codex, etc, or run `godai mcp setup`.

Quick Install
-------------

The recommended way to install Godai is via the install script.

On Linux or macOS:

```bash
curl -fsSL https://godai.sh/install | bash
```

Or, on Windows (PowerShell):

```powershell
irm https://godai.sh/install.ps1 | iex
```

Or, you can use a package manager:

<!-- brew -->
<details>
<summary>brew</summary>

```bash
brew tap snopek-games/godai
brew install godai
```
</details>

<!-- npm -->
<details>
<summary>npm</summary>

```bash
npm install -g @snopek-games/godai@latest
```
</details>

<!-- go install -->
<details>
<summary>go install</summary>

```bash
go install gitlab.com/snopek-games/godai/cmd/godai@latest
```
</details>

After Godai is installed, it can help you configure your MCP client via:

```bash
godai mcp setup
```

See the [full "Installation" documentation](https://godai.sh/docs/install) for more information.

"What is this AI trash?!"
-------------------------

It seems like most people are either in love with LLMs (and think _everything_ should be done
with AI!) or hate LLMs and refuse to touch them.

Personally, I have complex and mixed feelings that fall somewhere in the middle.

There are serious open questions with regard to copyright and LLMs, so using an LLM to write
code that you intend to distribute is extremely risky. They also tend to produce pretty mediocre
code in the best case, and insecure, buggy weirdness in the worst case. I would personally never
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
