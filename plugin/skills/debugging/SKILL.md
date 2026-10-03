---
name: debugging
description: How to diagnose Black Ops 3 modding problems — make errors visible (developer/logfile, devblocks, debug macros, the S.R.E. call stack), find and grep the game's `console_mp.log` (and `crashes.log`) and tell whether it is from the latest run, get real line numbers, tell compile vs linker vs unresolved-external vs runtime apart, and drive the dvar/devgui toolkit. Use when a map won't build, won't load, crashes, hangs on a black screen, or a script misbehaves at runtime — `unexpected $end, expecting TOKEN_SEMICOLON`, `Could not find scriptparsetree`, `Error linking script`, `Server Disconnected - Clientfield Mismatch`, a call stack tagged `missing line information`, an `assert fail:` value or an assert that never fires, `spawn prevented by ai_disableSpawn or g_spawnai` — and when reading any BO3 log. Distinct from t7kb:compiling (running the build and judging its verdict) and t7kb:scripting (writing the fix).
---

# Debugging BO3 mods

Most "it just fails" reports are a **visibility** problem — the fix is to make the engine tell you what's wrong, then work the error from its stage. Look the exact message up in **t7kb** (`t7kb:search` the string, `t7kb:get` the top hits — there's an error-list reference plus the Discord/forum causes tidy docs omit); this skill is the method and the toolkit around it.

## Tooling: catch it before you build

The fastest debugging is not reaching the game. Script in **VS Code with the GSCode extension** (Blakintosh's language server) — its inline diagnostics flag undefined functions, bad calls, and syntax errors as you type, killing a whole class of compile/link errors before a build. Don't skip this.

## Make errors visible first

In **Launcher → dvars** (or `+set …` on the command line), set:

- **`developer 2`** — verbose script-error detail (`1` is the lighter dev mode).
- **`logfile 1`** — async write (faster). Use **`logfile 2`** when chasing a hard crash: it syncs every line, so the tail survives the crash instead of being lost.

Reproduce, then read the **S.R.E. (script runtime error)** — the console prints the error plus a **call stack** naming the file for each frame. Cheats/dev need the map launched via `devmap` or a loaded mod (`sv_cheats`).

- **`assert`, `assertmsg` and `/# … #/` dev blocks need `scr_mod_enable_devblock 1`** — not `developer`. `GSC_Language.pdf`: *"Assert – Only tested when devblocks are enabled"*; the Launcher's tooltip: *"Developer blocks are executed in mods"* — and on a **usermap** it doesn't take (maintainer-verified). So on a usermap, `assert`s and `/# #/` blocks effectively never run: an assert that never fires says nothing about whether its code path ran — instrument with `IPrintLnBold` or a `#define`-gated print instead, or build as a mod.
- **These dvars reach a run only if something passes them.** The Launcher's Dvars dialog applies to *its* Run; a raw `BlackOps3.exe +devmap` line passes none of them, so it has no `logfile` and writes no new log. `t7kb:build`'s run stage passes them when asked: `launcher_dvars: true` reuses the ones saved in the Launcher's dialog, and `dvars: {"developer": "2", "logfile": "2"}` sets them directly (CLI `--launcher-dvars`, `--dvar logfile=2`). For a debug run, ask for `developer 2` and `logfile 2` (plus `scr_mod_enable_devblock 1` if you need asserts). By hand, it's `+set developer 2 +set logfile 2` before `+set fs_game <map> +devmap <map>`.
- **Check the Launcher's persisted dvars when AI never spawns.** `ai_disableSpawn` is one of them and easy to leave on; the log then floods with `SpawnFromSpawner( <unnamed> ) spawn prevented by ai_disableSpawn or g_spawnai.`

Turn this on **before** theorizing: get the real message and call stack first.

## Read `console_mp.log` yourself

`logfile` writes the **whole console to a file on disk** — so read and grep it directly instead of asking the user to copy the console or screenshot an error. Always do this before asking them to re-describe a symptom.

It lands at the **`fs_game` root**: a **mod** run → `mods/<modname>/console_mp.log`; a **usermap** (no `fs_game`) → the **game root** `<bo3_root>/console_mp.log` (*not* `usermaps/<map>/`). Don't assume which — a mod run whose `fs_game` folder is missing falls back to the root, and stale copies from earlier sessions sit in both places. **Glob `<bo3_root>/console_mp.log` plus `<bo3_root>/mods/*/console_mp.log` and take the newest by mtime.**

**The file may hold several sessions, or be from a previous run.** Both make you read a past run as the current one:

- **Several sessions in one file.** A fresh game launch rewrites the log (one `logfile opened on <date>` header at the top), but every map reload inside the same game process (`devmap` again, `map_restart`) appends another session below it — so the first match you grep is the *oldest*, very likely from before the fix you are testing. (verified on real logs) `log_append` makes it accumulate across launches too.
- **A file from a previous process.** If the run you just did never opened a log — `logfile` wasn't set, which is the default for a **headless** launch that passes no dvars (above) — the file on disk is the last run's, untouched. Compare the `logfile opened on` header and the file's mtime with when you launched before reading anything.

Always cut to the last session first:

```bash
start=$(grep -n "Game Initialization" console_mp.log | tail -1 | cut -d: -f1)
tail -n +$start console_mp.log | grep -c "your error"
```

The tell that you're reading the wrong window: **an occurrence count that grows run over run** while the first hit's timestamp never changes. Count per session; a cumulative count is history.

Lines are prefixed with an engine timestamp + subsystem tag (`[<ms>][<SUBSYSTEM>]`). What to grep:

- **`script error`** — the S.R.E. block, tagged `SCRIPTERROR`: `******* script error *******`, the message, then `******* Call stack *******` and one `file 'scripts/…'` line per frame (`- missing line information` on a usermap — see below).
- **`Could not find`** — a missing asset/material/rawfile, tagged `DB`: not in the `.zone`, or never converted.
- **`Error:`** — the catch-all first pass on a "it just fails" report.

Siblings at the **game root**, both worth checking when there's no S.R.E.: **`crashes.log`** (a hard crash's module list + addresses) and **`console.log`** (the non-`_mp` frontend/LUI log).

## Getting real line numbers

`missing line information` means the frame lives in a **shipped FastFile** — those carry no debug info. It is not a property of usermaps as such: a script **zoned by your own map** does report `file '…' line N` in a usermap build. So the question is never "usermap or mod", it is "is this frame my script or Treyarch's".

**An error inside an `#insert`ed `.gsh` is reported at the `#insert` line** of the including file (`GSC_Language.pdf`), so the number points at the include, not at the bug — open the `.gsh`.

When the whole stack is stock (`_zm_behavior.gsc` twice and nothing else), you have two ways to get lines:

- **Build/run as a mod** — carries per-line debug info for stock frames too.
- **Take the stock script over into your map** (next section). Your copy is your script, so it reports lines — and you can instrument it, which is usually worth more than the line number alone.

## An edit to a stock asset has no effect: the linker packs the stock one

When your copy of a stock script, anim, model, FX or rawfile is zoned and built but the game still runs the original, an upstream zone's assetlist contributes it and the linker skips yours. **t7kb:compiling** owns the override (commenting the assetlist CSV line) and the `.ff`-size test that proves which one ships.

## Instrument rather than theorise

When an error's call stack is stock and unhelpful, don't reason about which branch "must" be at fault. Take the file over (above) and **mark every candidate site**, then let the log say which one runs:

```gsc
IPrintLnBold("^3SG#7 L547");
self SetGoal( goalPos );
```

One run, and the last tag before the error is the line. Unverified hypotheses produce "fixes" to things that were never broken, and two such fixes can silently cancel each other out and make a correct fix look like a failure.

**Read the assert text.** Treyarch's asserts routinely carry the failing value: `assert fail: bus_window` names the exact string that didn't resolve, which is the whole diagnosis. If an assert *should* be firing and isn't in your log, check `scr_mod_enable_devblock` and whether you are reading the wrong session (both above) before concluding the code path wasn't reached.

## Diagnose by stage

- **Compile (GSC/CSC)** — a parse error in your script; the compiler names the file (and line). Fix the source. (`unexpected $end, expecting TOKEN_SEMICOLON` = a missing `;`/brace.)
- **Linker / build** — builds, but a reference doesn't resolve: `Error linking script "scripts/…"` / `Could not find scriptparsetree "scripts/…"` = the script (or asset) is **not in the `.zone`**, or the path is wrong. Add it / fix the path.
- **Unresolved external** — a called function the linker can't find: a missing `#using` for its namespace, a typo, or the defining script isn't zoned. A build-time link failure, **not** a runtime bug.
- **Runtime** — builds and loads, then errors mid-game (often only under `developer 2`): a bad `self`/`level` assumption, an undefined value (guard with `isdefined`), or a thread on a dead entity (missing `endon`).
- **Load dies on `Com_ERROR: Server Disconnected - Clientfield Mismatch.`** (`Check host TTY output for client & server clientfield registration details`) — it names no field. Relaunch with `+set com_clientfieldsdebug 1` and find `Clientfield mismatches :` in the console: each line reads `<SIDE> '<pool>' set does not contain field '<name>'`, and **the named side is the one missing the registration**. Causes, most common first: a `#using` in the map's `.gsc` with no matching `#using` of that feature's `.csc` half (the self-registration runs on one side only); the same field registered with different bits/type/version on the two sides; a stale copied `zm_usermap.gsc`/`.csc` after a mod-tools update; a `clientuimodel` only Lua reads, which still needs its CSC `register` (stock `_load.csc` does it with an `undefined` callback). (community, several sources; one wiki copy of the error list inverts the side — trust the console line.)
- **Black screen at load, nothing logged** — usually a loop that can iterate without a `wait`/`waittill`, starving the scheduler (**t7kb:scripting**), or a vehicle-path/animtree load death (**t7kb:moving-platforms**).

## Method

1. Reproduce with real output on (above); capture the **exact** message + call stack.
2. Run `t7kb:search` for the error string and key tokens; `t7kb:get` the top hits — the corpus (Discord/forums) carries causes tidy docs omit.
3. For any Treyarch-shipped token the error names (function, KVP, asset path), confirm the correct form against the raw mod-tools install before "fixing" it.

## Interactive & visual debugging

Once it loads but *misbehaves*, drive it instead of rebuilding. Set dvars from the console (`~`, then `/dvar value`) or a `.cfg`; run **`dvardump`** to discover what's available. High-value tools:

- **devgui** — BO3's built-in zombies dev menu (developer mode): `goto_round`, give money/perks/powerups/weapons, god mode, infinite ammo, force-spawn zombies / make a crawler — reach a broken state without playing through. `ExecDevGui("command")` fires any entry from GSC (keep it behind a dev check).
- **`timescale`** — `<1` slow-mo / `>1` fast-forward to catch timing/ordering bugs.
- **Isolate AI**: `g_spawnai 0` / `ai_disableSpawn` to remove zombies from the equation; `ai_showNavMesh`, `ai_showNavPaths`, `ai_showNavVolume` to see why pathing breaks.
- **Geometry/collision**: `r_showCollision`, `r_showTris`, `g_bDebugRenderBulletMeshes`.
- **Clientfields**: `com_clientFieldsDebug` for the server↔client state you can't otherwise see.
- **In-script**: a `#define`-gated `PRINT_X_DEBUG` macro for map-side debug prints (**t7kb:scripting**); `assert`/`assertmsg` (devblock-gated, above); `IPrintLnBold` for a quick on-screen value; GSC debug-draw built-ins for world-space issues (look the exact names up in t7kb). The full dvar set lives in t7kb — name the symptom and search.

## Don't invent

Error strings, dvar names, and log paths are shipped behaviour — quote the exact text from the log you actually read, and confirm dvars against the Launcher and t7kb before prescribing them. If neither the log, t7kb, nor the raw install supports a cause, say it's a hypothesis to instrument, not a diagnosis.
