<div align="center">
    <img src="docs/media/logo.svg" alt="T7 Companion logo" width="160" height="160"/>
    <h1>T7 Companion</h1>
    <h3><em>Black Ops 3 modding knowledge, grounded skills, and a headless build pipeline — for your AI agent.</em></h3>
</div>

<p align="center">
    <a href="https://github.com/McReaper/t7_companion/releases/latest"><img src="https://img.shields.io/github/v/release/McReaper/t7_companion?sort=semver" alt="Latest release"/></a>
    <a href="https://github.com/McReaper/t7_companion/releases"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/McReaper/t7_companion/badges/installs.json" alt="Installs"/></a>
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

T7 Companion turns a general-purpose coding agent into a Black Ops 3 modding partner: a **local knowledge base** of the BO3 modding community, **skills** that know where each craft fails silently, and **tools** to check your GDTs and build your map without the Launcher. One binary, `t7kb`, served over MCP — everything runs offline on your machine.

> [!TIP]
> 🔊 Turn on audio for the demo.

https://github.com/user-attachments/assets/0d533897-fd35-4f0d-9774-948546c0af6b

```mermaid
flowchart LR
    U(["You"]) -->|"ask, in your own words"| A["Your AI agent<br/>Claude Code, Codex, Cursor…"]
    A <-->|"follows the skills,<br/>calls t7kb"| TC
    TC -->|"reads, edits, builds"| MT
    subgraph TC["T7 Companion"]
        direction TB
        S["Skills (Claude Code)<br/>how each craft works,<br/>where it fails silently"]
        T["t7kb<br/>runs on your machine"]
        K[("Knowledge base<br/>what the BO3 modding<br/>community knows")]
        T -->|"searches"| K
    end
    subgraph MT["Your BO3 mod tools"]
        direction TB
        G["GDTs<br/>APE's assets"]
        M["Map build<br/>compile, light"]
        L["Linker<br/>fastfiles, shaders"]
        P["Black Ops III<br/>run your map"]
        G ~~~ M ~~~ L ~~~ P
    end
```

## 📥 Install

### Claude Code

```
/plugin marketplace add McReaper/t7_companion
/plugin install t7kb@t7-reapy
/reload-plugins
/t7kb:setup-or-update
```

Pick the **User** scope: a project-scope install doesn't load in `usermaps/<map>`.

`/t7kb:setup-or-update` downloads `t7kb` and its database, registers the MCP server, records your BO3 mod-tools root, and offers git tracking for it and plugin auto-update.

### Other MCP clients (Codex, OpenCode, Cursor, Copilot…)

```bash
curl -fsSL https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.sh | bash
```
```powershell
irm https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.ps1 | iex
```

Then register `t7kb mcp` as a stdio server ([per-client config](docs/clients.md)), or paste this README to your agent and let it do that.

The database: a ~0.9 GB download, unpacked to ~3.8 GB on first run.

## ✨ What your agent gets

- **A knowledge base it searches before answering** — wikis, forums, Discord, Treyarch's scripts, tutorials and the mod-tools schemas. Every hit carries its source, URL and reliability, so the agent can cite it and weigh a Treyarch script above a Discord guess.
- **Skills for each craft** — scripting, mapping, compiling, assets, animation, zombies AI, HUD, FX, atmosphere…: the method and its silent traps (the callback that never fires, the wallbuy that shows *Cost: 0*). Claude Code only.
- **GDT tools** — find, read, check and edit APE's GDTs against the install's own schema, as a dry run unless told to write. `gdt_check` catches broken references, missing files and duplicates before the linker does.
- **What the build packed** — why an asset is in your fastfile, what each zone line weighs, and which of your stock overrides won't ship.
- **A headless build** — create, compile, light, link and launch your map, with a short report of every linker error and warning instead of hundreds of lines: *"Relink zm_mymap"*. Windows only.
- **Shader decompilation** — the game's compiled shaders back to HLSL, to start a custom shader from a stock one.

## ⌨️ CLI

Every tool is also a command: `t7kb` alone opens an interactive search, and [the command list](docs/cli.md) has the rest.

## 🔄 Updating

**Turn on auto-update** for the `t7-reapy` marketplace (`/plugin` → Marketplaces; `/t7kb:setup-or-update` offers it): Claude Code then keeps the skills current, and at the next session start t7kb says when the binary is behind. `t7kb update-check` says whether either is behind, with the commands to update each. To refresh the binary, run `/t7kb:setup-or-update` in Claude Code, or re-run the installer with `--force` / `-Force`.

## 🤝 Contributing

Found a trap a skill doesn't mention, or one that's wrong? Tell your agent in Claude Code: the `contribute` skill writes the fix and, with your go-ahead, opens a pull request. By hand, start from [the authoring guide](plugin/skills/contribute/SKILL.md).

## 📄 License

> [!IMPORTANT]
> Code is **MIT** — see [`LICENSE`](LICENSE). `t7kb.db` bundles knowledge from the BO3 modding community; every entry carries its `source` and `url` for attribution — see [`docs/NOTICE.md`](docs/NOTICE.md).
