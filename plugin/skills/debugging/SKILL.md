---
name: bo3-debugging
description: How to diagnose Black Ops 3 modding problems — make errors visible (developer/logfile, debug macros, the S.R.E. call stack), get real line numbers, tell compile vs linker vs unresolved-external vs runtime apart, and drive the interactive dvar/devgui toolkit. Use when a map won't build or compile, won't load, crashes, or a script misbehaves at runtime, and when reading a script error, console output, or log file.
---

# Debugging BO3 mods

Most "it just fails" reports are a **visibility** problem — the fix is to make the engine tell you what's wrong, then work the error from its stage. Look the exact message up in **t7kb** (`search` the string, `get` the top hits — there's an error-list reference plus the Discord/forum causes tidy docs omit); this skill is the method and the toolkit around it.

## Tooling: catch it before you build

The fastest debugging is not reaching the game. Script in **VS Code with the GSCode extension** (Blakintosh's language server) — its inline diagnostics flag undefined functions, bad calls, and syntax errors as you type, killing a whole class of compile/link errors before a build. Don't skip this.

## Make errors visible first

In **Launcher → dvars** (or `+set …` on the command line), set:

- **`developer 2`** — verbose script-error detail (`1` is the lighter dev mode).
- **`logfile 1`** — async write (faster). Use **`logfile 2`** when chasing a hard crash: it syncs every line, so the tail survives the crash instead of being lost.

Reproduce, then read the **S.R.E. (script runtime error)** in the console — it prints the error plus a **call stack** naming the file for each frame. The full log is `console_mp.log`, written at the **`fs_game` root**: for a **usermap** that's the **game root** (`…/Call of Duty Black Ops III/console_mp.log`, *not* the `usermaps/<map>` folder); for a **mod** it's `mods/<modname>/console_mp.log`. Cheats/dev need the map launched via `devmap` or a loaded mod (`sv_cheats`).

Turn this on **before** theorizing — guessing at a hidden error just burns build cycles; get the real message and call stack first.

## Getting real line numbers (the usermap trap)

As a **usermap**, the call stack shows the file but reads `missing line information` for every frame — no line numbers, even with `developer 2`. To get real `file '…' line N`, **build/run the script as a mod** (a mod build carries the per-line debug info a usermap FastFile doesn't). Also: **stock/base-game frames never show lines** — the shipped FastFiles have no debug info, so trace from the last frame that's in *your own* code.

## Diagnose by stage

- **Compile (GSC/CSC)** — a parse error in your script; the compiler names the file (and line). Fix the source. (`unexpected $end, expecting TOKEN_SEMICOLON` = a missing `;`/brace.)
- **Linker / build** — builds, but a reference doesn't resolve: `Error linking script "scripts/…"` / `Could not find scriptparsetree "scripts/…"` = the script (or asset) is **not in the `.zone`**, or the path is wrong. Add it / fix the path.
- **Unresolved external** — a called function the linker can't find: a missing `#using` for its namespace, a typo, or the defining script isn't zoned. A build-time link failure, **not** a runtime bug.
- **Runtime** — builds and loads, then errors mid-game (often only under `developer 2`): a bad `self`/`level` assumption, an undefined value (guard with `isdefined`), or a thread on a dead entity (missing `endon`).

## Method

1. Reproduce with real output on (above); capture the **exact** message + call stack.
2. `search` t7kb for the error string and key tokens; `get` the top hits — the corpus (Discord/forums) carries causes tidy docs omit.
3. For any Treyarch-shipped token the error names (function, KVP, asset path), confirm the correct form against the raw mod-tools install before "fixing" it.

## Interactive & visual debugging

Once it loads but *misbehaves*, drive it instead of rebuilding. Set dvars from the console (`~`, then `/dvar value`) or a `.cfg`; run **`dvardump`** to discover what's available. High-value tools:

- **devgui** — BO3's built-in zombies dev menu (developer mode): `goto_round`, give money/perks/powerups/weapons, god mode, infinite ammo, force-spawn zombies / make a crawler — reach a broken state without playing through. `ExecDevGui("command")` fires any entry from GSC (keep it behind a dev check).
- **`timescale`** — `<1` slow-mo / `>1` fast-forward to catch timing/ordering bugs.
- **Isolate AI**: `g_spawnai 0` / `ai_disableSpawn` to remove zombies from the equation; `ai_showNavMesh`, `ai_showNavPaths`, `ai_showNavVolume` to see why pathing breaks.
- **Geometry/collision**: `r_showCollision`, `r_showTris`, `g_bDebugRenderBulletMeshes`.
- **Clientfields**: `com_clientFieldsDebug` for the server↔client state you can't otherwise see.
- **In-script**: a `#define`-gated `PRINT_X_DEBUG` macro for map-side debug prints (see the scripting skill); `assert`/`assertmsg`; `IPrintLnBold` for a quick on-screen value; GSC debug-draw built-ins for world-space issues (look the exact names up in t7kb). The full dvar set lives in t7kb — name the symptom and search.
