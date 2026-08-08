---
name: bo3-debugging
description: How to diagnose Black Ops 3 modding problems — make errors visible (developer/logfile, debug macros, the S.R.E. call stack), find and grep the game's `console_mp.log` (and `crashes.log`), get real line numbers, tell compile vs linker vs unresolved-external vs runtime apart, and drive the interactive dvar/devgui toolkit. Use when a map won't build or compile, won't load, crashes, or a script misbehaves at runtime, and when reading a script error, console output, `console_mp.log`, or any BO3 log file.
---

# Debugging BO3 mods

Most "it just fails" reports are a **visibility** problem — the fix is to make the engine tell you what's wrong, then work the error from its stage. Look the exact message up in **t7kb** (`search` the string, `get` the top hits — there's an error-list reference plus the Discord/forum causes tidy docs omit); this skill is the method and the toolkit around it.

## Tooling: catch it before you build

The fastest debugging is not reaching the game. Script in **VS Code with the GSCode extension** (Blakintosh's language server) — its inline diagnostics flag undefined functions, bad calls, and syntax errors as you type, killing a whole class of compile/link errors before a build. Don't skip this.

## Make errors visible first

In **Launcher → dvars** (or `+set …` on the command line), set:

- **`developer 2`** — verbose script-error detail (`1` is the lighter dev mode).
- **`logfile 1`** — async write (faster). Use **`logfile 2`** when chasing a hard crash: it syncs every line, so the tail survives the crash instead of being lost.

Reproduce, then read the **S.R.E. (script runtime error)** — the console prints the error plus a **call stack** naming the file for each frame. Cheats/dev need the map launched via `devmap` or a loaded mod (`sv_cheats`).

Turn this on **before** theorizing — guessing at a hidden error just burns build cycles; get the real message and call stack first.

## Read `console_mp.log` yourself

`logfile` writes the **whole console to a file on disk** — so read and grep it directly instead of asking the user to copy the console or screenshot an error. Always do this before asking them to re-describe a symptom.

It lands at the **`fs_game` root**: a **mod** run → `mods/<modname>/console_mp.log`; a **usermap** (no `fs_game`) → the **game root** `<bo3_root>/console_mp.log` (*not* `usermaps/<map>/`). Don't assume which — a mod run whose `fs_game` folder is missing falls back to the root, and stale copies from earlier sessions sit in both places. **Glob `<bo3_root>/console_mp.log` plus `<bo3_root>/mods/*/console_mp.log` and take the newest by mtime.**

**Do not assume the file is one session.** It is frequently **appended** across runs, so the first match you grep for is the *oldest* occurrence — very likely a run from before the fix you are testing. Diagnosing several rounds in a row against the same stale block is the single easiest way to burn an afternoon "fixing" things that were never broken.

Always cut to the last session first:

```bash
start=$(grep -n "Game Initialization" console_mp.log | tail -1 | cut -d: -f1)
tail -n +$start console_mp.log | grep -c "your error"
```

The tell that you've been reading the wrong window: **an occurrence count that grows run over run** (2 → 4 → 10 → 14) while the timestamps of the first hit never change. A per-session count is what matters; a cumulative one means you're re-reading history.

Lines are prefixed with an engine timestamp + subsystem tag (`[<ms>][<SUBSYSTEM>]`). What to grep:

- **`script error`** — the S.R.E. block, tagged `SCRIPTERROR`: `******* script error *******`, the message, then `******* Call stack *******` and one `file 'scripts/…'` line per frame (`- missing line information` on a usermap — see below).
- **`Could not find`** — a missing asset/material/rawfile, tagged `DB`: not in the `.zone`, or never converted.
- **`Error:`** — the catch-all first pass on a "it just fails" report.

Siblings at the **game root**, both worth checking when there's no S.R.E.: **`crashes.log`** (a hard crash's module list + addresses) and **`console.log`** (the non-`_mp` frontend/LUI log).

## Getting real line numbers

`missing line information` means the frame lives in a **shipped FastFile** — those carry no debug info. It is not a property of usermaps as such: a script **zoned by your own map** does report `file '…' line N` in a usermap build. So the question is never "usermap or mod", it is "is this frame my script or Treyarch's".

When the whole stack is stock (`_zm_behavior.gsc` twice and nothing else), you have two ways to get lines:

- **Build/run as a mod** — carries per-line debug info for stock frames too.
- **Take the stock script over into your map** (next section). Your copy is your script, so it reports lines — and you can instrument it, which is usually worth more than the line number alone.

## Overriding a stock script from a *usermap* — the assetlist CSV

The received wisdom that "a usermap can't override stock scripts, only a mod can" is **incomplete**. Dropping your copy at the same path under `usermaps/<map>/scripts/…` and zoning it is not enough — the stock one is still pulled in by the patch asset list and wins. The missing step:

**Comment the stock entry out of `zone_source/all/assetlist/zm_patch.csv`.**

```
//scriptparsetree,scripts/zm/_zm_behavior.gsc
```

That file lists every stock script the zm patch zone contributes; commenting a line removes it from the build, and your zoned copy takes its place. Shipped installs already ship several lines commented this way (`_zm_ai_dogs`, `_zm_pack_a_punch`, `_zm_weapons`), which is the confirmation the mechanism is intended. Back the CSV up first — it is a shared, install-wide file, so the change affects every map you build until you undo it.

## Instrument rather than theorise

When an error's call stack is stock and unhelpful, the fastest route to the answer is almost never more reasoning about which branch "must" be at fault. Take the file over (above) and **mark every candidate site**, then let the log say which one runs:

```gsc
IPrintLnBold("^3SG#7 L547");
self SetGoal( goalPos );
```

One run, and the last tag before the error is the line. This costs ten minutes and ends the guessing; a chain of plausible-but-unverified hypotheses costs hours and, worse, produces "fixes" to things that were never broken — two of which can silently cancel each other out and make a correct fix look like a failure.

**Read the assert text.** Treyarch's asserts routinely carry the failing value: `assert fail: bus_window` names the exact string that didn't resolve, which is the whole diagnosis. If an assert *should* be firing and isn't in your log, suspect you are reading the wrong session (above) before concluding the code path wasn't reached.

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
