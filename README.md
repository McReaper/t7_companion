<div align="center">
    <img src="docs/media/logo.svg" alt="T7 Companion logo" width="160" height="160"/>
    <h1>T7 Companion</h1>
    <h3><em>Black Ops 3 modding knowledge, grounded skills, and a headless build pipeline — for your AI agent.</em></h3>
</div>

<p align="center">
    <a href="https://github.com/McReaper/t7_companion/releases/latest"><img src="https://img.shields.io/github/v/release/McReaper/t7_companion?sort=semver" alt="Latest release"/></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="License: MIT"/></a>
    <img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25"/>
    <img src="https://img.shields.io/badge/runs-offline-brightgreen" alt="Runs offline"/>
</p>

<p align="center">
    <a href="#-install">Install</a> ·
    <a href="#-what-your-agent-gets">Features</a> ·
    <a href="#%EF%B8%8F-cli">CLI</a> ·
    <a href="#-updating">Updating</a> ·
    <a href="#-contributing">Contributing</a> ·
    <a href="#-license">License</a>
</p>

T7 Companion turns a general-purpose coding agent into a Black Ops 3 modding partner. It gives the agent a **local, offline knowledge base** of the BO3 modding community — wikis, forums, Discord, decompiled Treyarch scripts, tutorials, mod-tools schemas — plus **skills** that encode the method and the silent-failure traps of each craft, and a **build tool** that compiles your map without the Launcher. It all runs on your machine: one pure-Go binary, `t7kb`, serving a bundled SQLite index over MCP.

> [!TIP]
> 🔊 Turn on audio for the demo.

https://github.com/user-attachments/assets/0d533897-fd35-4f0d-9774-948546c0af6b

```mermaid
flowchart LR
    S["Skills<br/>(Claude Code plugin)"] -. "how each craft works,<br/>where it fails silently" .-> A([Your AI agent])
    A <-->|MCP| T["t7kb<br/>one local binary"]
    T --> K[("Knowledge base<br/>search · get")]
    T --> G["Your GDTs<br/>find · read · check · edit"]
    T --> B["BO3 mod tools<br/>compile · light · link · run"]
```

## 📥 Install

### Claude Code

Install the plugin, then let it set everything up — it downloads `t7kb` and the database and registers the MCP server:

```
/plugin marketplace add McReaper/t7_companion
/plugin install t7kb@t7-reapy
/reload-plugins
/t7kb:setup
```

> [!NOTE]
> `/t7kb:setup` is itself one of the plugin's skills, so it only exists after `/reload-plugins` (or a new session). Setup also offers to drop an `AGENTS.md` primer at your BO3 mod-tools root and records where your install lives, so the agent can check claims against Treyarch's own files.

### Any other MCP client (Codex, OpenCode, Cursor, Copilot…)

Run the installer — or paste this README to your agent and let it do it:

```bash
curl -fsSL https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.sh | bash
```
```powershell
irm https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.ps1 | iex
```

Then register `t7kb mcp` as a stdio server — **[docs/clients.md](docs/clients.md)** has copy-paste config for each client. The skills are a Claude Code feature; on other agents, drop [`templates/AGENTS.md`](templates/AGENTS.md) at your BO3 mod-tools root for the same core guidance.

> [!NOTE]
> The installers put the binary, the embedding model and the database in one folder (`~/.t7kb`, or `%LOCALAPPDATA%\t7kb` on Windows). The database downloads as a ~0.9 GB archive and unpacks itself (~3.5 GB) on first run. Both installers are idempotent; `--force` / `-Force` reinstalls. To do it by hand, extract a release archive and `t7kb.db.zip` into the same folder.

## ✨ What your agent gets

### A knowledge base it searches before answering

Two MCP tools: `search` (hybrid keyword + semantic retrieval) and `get` (a full document by id). Every hit carries its **source, URL and a reliability score**, so the agent can weigh a Treyarch script above a Discord guess, cite what it used, and surface disagreements instead of blending them.

### Skills that know where BO3 modding fails silently

The Claude Code plugin ships skills for scripting, debugging, Radiant mapping, compiling, assets and porting, animation and retargeting, zombies AI, moving platforms, HUD/LUI, FX, atmosphere and audio, localization, and cross-referencing other Call of Duty titles. They aren't documentation — the knowledge base is — they're the method for each craft and the traps that cost an afternoon: the callback that never fires in zombies, the wallbuy that shows *Cost: 0*, the reverb that never switches rooms. Each claim is grounded in t7kb or checked against the mod-tools install itself, and the agent is told to verify shipped tokens the same way before asserting them.

### A build it can run and read

The MCP `build` tool drives the mod-tools pipeline headlessly — `gdtdb` → `cod2map64` (compile) → `radiant_modtools` (light) → `linker_modtools` (link), optionally launching the game — so the agent can compile the map it just edited and read the result:

> *"Relink zm_mymap"* · *"Full compile and light of zm_mymap, then tell me what failed"*

It returns a short per-stage report (status, duration, first actionable error) instead of each tool's hundreds of lines, and handles the traps the tools don't announce: `cod2map64` silently skips the navmesh unless it runs from `bin/`, the light bake detaches and has to be waited on, and a linker warning exits non-zero on a build that actually succeeded.

| Parameter | What it does |
|---|---|
| `name` | Map or mod name, e.g. `zm_mymap` (required) |
| `stages` | `compile,light,link,run` — default `compile,light,link`; `link` alone for a script-only change |
| `mod` | Build `mods/<name>` instead of a usermap |
| `light` | `low` / `medium` / `high` bake quality (default `medium`) |
| `onlyents` | Fast entity-only compile — invalid after brush edits |
| `launcher_dvars` | `run`: start the game with the dvars saved in the Launcher's Dvars dialog |
| `dvars` | `run`: dvars to start the game with, e.g. `{"developer": "2", "logfile": "2"}` to get `console_mp.log` |

> [!NOTE]
> `build` is Windows only (the `gdt_*` tools just read and write text files); both need the BO3 Mod Tools installed (`TA_TOOLS_PATH`/`TA_GAME_PATH`, set by the Launcher on first run). `build` and `gdt_edit` are the only tools on the server that change anything on disk. The `run` stage starts the game on your map the way the Launcher's Run does; Steam must be running.

### GDT editing it can check

APE's asset databases are plain-text GDTs, and the `gdt_*` MCP tools let the agent read and write them using the install's own rules as the schema: the `deffiles/*.awi` scripts that build APE's pages, and the techsets behind each material type.

- **`gdt_find` / `gdt_get`**: find an asset in any GDT the tools index and read its fields, inherited ones included.
- **`gdt_schema`**: what APE declares for an asset type, and for a material, the texture slots its type really reads.
- **`gdt_edit`**: create or change assets, one or a batch, **as a dry run unless told to write**. It refuses stock GDTs and duplicate names, turns a texture into an image asset, and catches out-of-range values, unknown dropdown choices, a surface type left on `<error>`, a material type and category that disagree, or an image with the wrong semantic. It also writes what APE writes when a field changes (a gloss preset's range, an effect image's alpha settings).
- **`gdt_check`**: diagnose a whole GDT before you build: missing or wrong-typed references, exports and textures missing on disk, duplicates, LODs that would never be drawn.
- **`gdt_refs`**: everything that uses an asset, before you rename or delete it.

Then `build` indexes and links it.

## ⌨️ CLI

The same binary works from a terminal:

```
t7kb                         interactive browse: type a query, pick a hit, read it
t7kb search <query>...       hybrid search  (--bm25 keyword-only · -n N results · --scores)
t7kb get <doc_id>            print a full document
t7kb build <name>            compile/light/link  (--stages · --light · --mod · --onlyents · --gdt-rebuild · --json)
t7kb gdt find|get|schema|edit|check|refs   GDT lookup, schema, validated edits, diagnostics  (edit is a dry run unless --write)
t7kb mcp                     run the stdio MCP server
t7kb update-check            is a newer release out?
```

`--db PATH` picks the database; by default `t7kb` uses `$T7KB_DB`, then the `t7kb.db` beside the binary, then `./t7kb.db`.

## 🔄 Updating

**Claude Code:** update the plugin (`/plugin marketplace update t7-reapy`, then update `t7kb` in the `/plugin` menu), then run `/t7kb:setup`: it checks for a newer release and offers to update the binary and database.

**Other clients:** `t7kb update-check` tells you whether a newer release exists (it only runs when you ask and never downloads anything itself); re-run the installer with `--force` / `-Force` to update.

Plugin and binary versions move together, one per release.

## 🤝 Contributing

Found a trap a skill doesn't mention, or a skill that turned out wrong? In Claude Code, just tell the agent — the `contribute` skill picks it up and, with your go-ahead, forks the repo, writes the fix in the house style, validates it and opens a pull request. By hand: every skill lives in [`plugin/skills/`](plugin/skills), `python plugin/skills/contribute/scripts/check_skills.py` validates them (CI runs it on every PR), and [`plugin/evals/`](plugin/evals) holds the trigger tests for `claude plugin eval`. The authoring guide is [`plugin/skills/contribute/SKILL.md`](plugin/skills/contribute/SKILL.md).

## 📄 License

> [!IMPORTANT]
> Code is **MIT** — see [`LICENSE`](LICENSE). `t7kb.db` bundles knowledge from the BO3 modding community; every entry carries its `source` and `url` for attribution — see [`docs/NOTICE.md`](docs/NOTICE.md).
