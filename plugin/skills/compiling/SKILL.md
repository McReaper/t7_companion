---
name: compiling
description: How to build a Black Ops 3 map or mod — the Mod Tools Launcher pipeline (Compile, Light/LEDs, Link, Run), each mod-tools binary, the TA_* environment, the `zm_`/`mp_` prefix, converting GDT/assets before linking, usermap-vs-mod builds, overriding a stock asset or script by commenting its line out of `zone_source/all/assetlist/*.csv` (`zm_patch`, `zm_levelcommon`), which stage to re-run, headless builds (`t7kb:build`). Use when building, compiling, linking or lighting a map or mod, driving the binaries by hand, when an edited or replaced stock asset is ignored and the original still ships, a published map is huge or its `.xpak` grows every link, or judging whether a build succeeded — the linker exits non-zero on warnings too (`exit status 1000`, `Found 1 bad bulletmeshes`, `ok` false on a current Fast File), and the verdict lives in `zone_source/all/assetinfo/` (`.errorlog`, `.csv`). Distinct from t7kb:debugging (reading the errors) and t7kb:mapping (the geometry behind compile failures).
---

# Building & compiling BO3 maps and mods

Shipping a map/mod is a **pipeline of separate stages**, each a different tool with its own inputs — the craft is knowing which stage owns which output so you rebuild only what changed and read failures at the right stage. This skill is the workflow and its gotchas; look exact zone syntax, dvars, and binary flags up in **t7kb** (`t7kb:search` then `t7kb:get`) and confirm shipped tokens against the raw mod-tools install. For *reading* a build error see **t7kb:debugging** (compile vs linker vs unresolved-external vs runtime); for geometry that fails the map compile (leaks, triangle budget) see **t7kb:mapping**.

## The Launcher and its environment

Everything runs through the **Mod Tools Launcher** (`bin/modlauncher.exe`, opened from Steam as *Call of Duty: Black Ops III — Mod Tools*). It orchestrates the per-tool binaries in `bin/` and needs three environment variables pointing at the install (set once by `modtools_setenv.bat` / on first launch): **`TA_TOOLS_PATH`** and **`TA_GAME_PATH`** (both the BO3 root) and **`TA_LOCAL_ASSET_CACHE`** (`share/assetconvert/`). If tools misbehave in odd ways, a missing/stale `TA_*` var is a common root cause — check them before anything else.

New maps: **Add Map** from the toolbar, and prefix the name with its mode — **`zm_`** for zombies, **`mp_`** for multiplayer (e.g. `zm_testmap`). The prefix is load-bearing, not cosmetic: the tools derive the map's folders from those two letters (`map_source/zm/`, `share/raw/maps/zm/`), so an un-prefixed name fails at compile. Choose the name once — renaming later means editing every file that references it. Add Map scaffolds the map's zone/GDT/script files so it's buildable.

## The four build stages (the Build Options panel)

The right-hand Build Options are four independent checkboxes run by the **Build** button — tick only the ones whose input changed:

- **Compile** (quality dropdown) — compiles the Radiant `.map` geometry into the level BSP via `cod2map64.exe`: portals/visibility, collision, umbra occlusion. The full pass runs `-navmesh -navvolume`; the fast option is the entity-only `-onlyents` pass (see *Iterate fast*). This is where BSP **leaks** and **`MAX_MAP_TRIANGLES`** surface — see **t7kb:mapping** for the geometry side.
- **Light** (dropdown **Low / Medium / High**) — bakes lighting and writes the **LEDs** (Lighting Export Data) through Radiant. Lighting work is invisible in-game until this runs, and the LEDs must be exported or the map loads unlit. In the GUI this opens Radiant; **it also bakes headlessly** via `radiant_modtools.exe -ledSilent` (see *Headless build* below) — Radiant isn't only a GUI here. Use Low/Medium while iterating, High for a final pass.
- **Link** — runs `linker_modtools.exe`: reads the map/mod's **`.zone`** file(s) and packs every listed asset into the shipped **Fast Files** (`.ff`). An asset that isn't in the `.zone` won't be in the build — that's the classic `Could not find scriptparsetree` / unresolved-external at this stage (**t7kb:debugging** owns diagnosing it). Linking is the step that turns "edited in the tools" into "loadable by the game".
- **Run** — launches the game on the built map/mod. Build stages fail on *their own stdout*; once the game is up, everything it says goes to **`console_mp.log`** instead (with the `logfile` dvar set) — **t7kb:debugging** covers where that file lands and what to grep in it. **The Launcher's Dvars dialog only applies to a Launcher Run**: a headless run (`t7kb:build`'s `run` stage, or step 5 below) gets none of them unless `t7kb:build` is passed `launcher_dvars`, so pass `developer 2` and `logfile 2` yourself when the run is for debugging — otherwise no new log is written and the old one reads as current. **The game stops on a dialog, not in the log**: a fatal error (`Could not find zone 'fr_<map>'`: the map isn't linked in the game's language) or the safe-mode question after a crash opens a message box the game waits on; `t7kb:build`'s run watches the first 90 s and fails on one with its text and the console's last lines (verified on a real run).

A normal first build ticks all four; day-to-day you re-tick only what changed (see *Iterate fast* below).

## Assets must be converted before you link

The linker packs **converted** assets, not raw source — so the asset pipeline has to run first:

- **GDT / APE** — models, materials, images, sounds, and FX are defined in **GDTs**, edited in **APE** (`asseteditor_modtools.exe` — Asset Property Editor). Save the GDT after editing; the **GdtDB** service (`GdtDBTray.exe`) indexes GDTs so the linker can find them (the Launcher runs a `gdtdb /update` pass — watch its output line). An asset edited but not saved/indexed links stale.
- **Model/anim bins** — source models and animations are converted to engine `.bin` via **`export2bin.exe`** / **`exportxbin.exe`** (usually invoked by the export step from Maya/Blender or on GDT convert). A model that shows source-but-not-in-game usually never got binned. Porting/rigging detail lives in **t7kb:assets**.

Order per iteration: **save GDT → (GdtDB indexes) → Compile/Light as needed → Link → Run**.

## Usermap vs mod — different build target, different output

Where the build lands and what can be overridden depends on the target (this mirrors the scripting/entry-file split — see **t7kb:scripting**):

- **Usermap** — built under `usermaps/<map>/`; the map's own zone. Scripts your map zones report real `file '…' line N`; only frames inside Treyarch's shipped scripts show `missing line information`. Overriding a stock script — or any stock asset — works from a usermap too, through the assetlist CSVs rather than by switching to a mod (below).
- **Mod** — built under `mods/<modname>/`; also carries line info for stock frames, which is the one debugging reason to build as a mod. It does **not** override more than a usermap can: the linker skips an upstream-contributed asset for both targets alike.

Which assets go into the Fast File is entirely the **`.zone`** (`zone_source/*.zone`) — adding a script/model/sound means adding its line there, then re-linking. Look the exact `.zone` entry syntax up in t7kb. **A reference to an asset no GDT defines doesn't fail the link**: the linker links on without it and logs nothing, so a material shows without that texture in game. `t7kb:build` lists those after the link ("linked without"), with the asset that named each (verified against the linker's `.deps` on real maps): fix the name in that asset's GDT, or add the missing asset.

## Overriding a stock asset: comment its assetlist line, from a usermap or a mod

"A usermap can't override stock content, only a mod can" is **wrong**, and it isn't only about scripts. **For any asset an inherited `ignore` list names, the linker packs only a reference to the shipped one** (a few dozen bytes, nothing streamed) — your zone inherits those lists through its `>class,…` header — so your copy is silently ignored no matter how correctly you zone it (verified in the linker's report). Building as a mod does not help: same class, same skip.

**Comment the stock entry out of the assetlist CSV that contributes it**, under `zone_source/all/assetlist/`:

```
//scriptparsetree,scripts/zm/_zm_behavior.gsc      zm_patch.csv
//rawfile,animtrees/generic.atr                    core_common.csv
```

Then your zoned copy takes its place. **Any asset type, and the file is whichever `ignore` list names it** — for a usermap (`zm_mod_level`) the `core_*` lists, `zm_common.csv` and `zm_patch.csv`. `zm_levelcommon.csv` is inherited as `ignore_missing_shipped` instead, whose assets are packed in full, so it never needs commenting (verified in the linker's report). **`t7kb:zone_check` finds the line for you**: for a stock script or asset you zoned, before the first link, and after a link for every GDT asset of yours the build replaced with stock. Without MCP, `grep -rn "<asset path>" zone_source/all/assetlist/` and comment the line you actually find. Shipped installs already comment several lines this way (`_zm_ai_dogs`, `_zm_pack_a_punch`, `_zm_weapons`), so the mechanism is intended. These are shared, install-wide files: keep the BO3 root in git (`/t7kb:setup-or-update` offers it) and commit each edit on its own, or at least back the file up, because the edit affects every map built from that tree until reverted.

Two traps make the edit silently do nothing:

- **Edit the BO3 root's `zone_source/all/assetlist/`, never the map's.** `usermaps/<map>/zone_source/all/assetlist/<map>.csv` is the linker's output, rewritten on every link (verified in the install).
- **Comment with `//`**, as every commented line in the shipped lists does; `#` does not comment a line (community).

**Diagnose from the link, not by theory.** `t7kb:zone_explain` on the asset says, from the last link's report, whether only a reference to the shipped asset was packed and which assetlist line caused it. Without MCP, measure: append a few KB of junk to the file, relink, and compare the `.ff` size before and after:

- **delta 0** → the linker is not packing your file at all; you need the CSV line commented (above).
- **delta > 0** → it *is* in the build, and your bug is elsewhere.

The measurement works for any raw asset — animtrees, animtables, behavior trees, scripts.

## Iterate fast — rebuild only what changed

The stages are decoupled on purpose; the slow full build is only for the first pass or a geometry change.

- **Script-only change** (GSC/CSC/Lua) → **Link** alone. No Compile, no Light — scripts aren't in the BSP. Fastest loop.
- **Entity-only change** (moved/added spawners, script_structs, KVPs — no brush edits) → the map compiler's **`-onlyents`** fast path re-exports just entities. It's invalid the moment any *brush* geometry changed (throws a brush-count mismatch) — that forces a **Full Compile**.
- **Geometry change** (brushes/patches) → **Full Compile** (+ Light if it affects lighting) → Link.
- **Lighting-only change** → **Light** → Link; no recompile.
- **Asset edit** (GDT/model/material) → save + let GdtDB index → **Link**. Run `t7kb:gdt_check` on the GDT first: it reports the missing references, missing source files and duplicates the linker would otherwise hand you one per run (**t7kb:assets**).

When a build hangs or fails, isolate by running one stage at a time and read that stage's output — don't re-run the whole pipeline blind. Capture the exact message and take it to **t7kb:debugging**.

## Headless build — the agent can compile for the user

Every stage is a **console binary**; the Launcher GUI only chains them. An agent can drive the whole build with no GUI, in this order of preference: the `t7kb:build` MCP tool, then the `t7kb build` shell subcommand if no MCP server is registered, then the raw mod-tools binaries only if `t7kb` itself isn't installed. All three run the identical pipeline underneath, so drop down a tier only when the one above genuinely isn't available to you — not as a first instinct.

### `t7kb:build` — the MCP tool, use this first

If this skill fired over MCP, the `t7kb` server is very likely already registered — it's the same server that exposes `t7kb:search`/`t7kb:get` — so call the `build` tool directly instead of shelling out. Its parameters (from the tool's own schema): `name` (required, e.g. `"zm_mymap"`), `stages` (comma list `compile,light,link,run`, default `"compile,light,link"`; pass `"link"` alone for a script-only change), `mod` (bool, target is `mods/<name>` instead of a usermap; default `false`), `light` (`low`|`medium`|`high`, default `"medium"`), `onlyents` (bool, fast entity-only compile; default `false`), `language` (default: the Build Language saved in the Launcher's Edit > Options, else `"english"`; the game opens a map only once it's linked in the game's own language), `skip_gdt` (bool, skip the `gdtdb /update` pass; default `false`), `gdt_rebuild` (bool, run `gdtdb /rebuild` instead — the recovery when a GDT change leaves every asset missing; default `false`), `tools_path`/`game_path` (default `$TA_TOOLS_PATH`/`$TA_GAME_PATH`). It returns the same per-stage JSON report as `t7kb build --json` below, with every error and warning the link logged (see the errors-and-warnings section further down).

If the `t7kb` server has `search`/`get` but no `build` tool, its binary predates it (it ships from 1.5.0): tell the user and offer `/t7kb:setup-or-update` to update it, then fall back to the shell tier below meanwhile.

**It runs synchronously and can take minutes (link) to 20–30 minutes (a full compile+light)** — set a long client-side timeout; a long wait is normal, not a hang.

### `t7kb build` — the shell fallback, when no MCP server is registered

No MCP server this session, or you want the CLI's `--json`/`--verbose` output? The same tool ships a `t7kb build` subcommand that runs the same pipeline with the same gotchas handled; its usage and flags are in **`references/cli.md`**.

### Last resort: the raw Launcher binaries, only if `t7kb` itself isn't installed

Both tiers above run the Launcher's own command lines for you, gotchas and all — never shell out to the binaries as a shortcut around them. If neither `t7kb:build` nor `t7kb build` is available, each stage's command line and its shell traps (git-bash/MSYS mangling `/update` and `+medium`, the working directory `cod2map64` and `gdtdb` need, the light bake that detaches) are in **`references/raw-binaries.md`**.

Whichever way you build, these are the outputs to check (`<pp>` = the map's `zm`/`mp` prefix):

- **Outputs to expect** (confirm the build by their mtime): compile → `share/raw/maps/<pp>/<map>.d3dbsp` (+ `<map>_navmesh.hkt`, `<map>.d3dprt`); light → `share/raw/maps/<pp>/<map>.led`; link → Fast Files in `usermaps/<map>/zone/` (or `mods/<mod>/zone/`): `<map>.ff` + `<map>.xpak`. One `-language <lang>` pass writes the language-neutral `<map>.ff` **and** that language's `<lang>_<map>.ff`; the other languages' localized Fast Files keep their previous content until you pass their language too. Scripts and models are in the neutral one, so an english-only pass does refresh your code. What goes *into* those localized Fast Files — `.str` files, reference naming, `linkerflag,noloc` — is **t7kb:localization**.

## A link's errors and warnings: look each one up, fix it at its source, relink

The linker exits non-zero on warnings too (`^3Found 1 bad bulletmeshes` returns 1000 on a correct, current Fast File; verified on a real build), so its errorlogs are the verdict. `t7kb:build` returns every entry of every errorlog the link wrote, each with the asset chain the linker indents under it (`xmodel:x < csv:zone_source/<map>.zone`): `errors` (`^1`, the link failed) and `warnings` (`^3`, the Fast File is built). It adds `missing`: assets the link looked up and found in no GDT, which it links without and logs nothing about — fix the field that names one, or create the asset (verified against the linker's `.deps`).

For each entry, `t7kb:search` its fixed words, names and numbers stripped, unfiltered: the Discord threads hold the fixes. Fix it in the asset or zone line its chain names, with the skill that owns it (**t7kb:assets**, **t7kb:scripting**, **t7kb:mapping**, **t7kb:shaders**). Relink until the summary is clean, or tell the user which warning stays and why.

To prove an asset was packed rather than infer it from a green build, `t7kb:zone_explain` and `t7kb:zone_contents` read `<map>.csv` beside the errorlog; relink first if they say sources changed. Without MCP, grep it, and check the `.ff`'s modification time.

## Publishing: link fresh, then shrink your images

What Publish uploads, why the `.xpak` grows on every link, and how to cut the upload without cutting content (`fresh_xpak`, `mipBase`, `t7kb:zone_contents` `images`) are in **`references/publish.md`**.

## Don't invent

Binary names, the `TA_*` vars, the four stages, and the `mp_` rule above are from the shipped mod-tools install — treat them as ground truth. But exact `.zone` syntax, `linker`/`cod2map` command-line flags, and dvars are shipped tokens: confirm them against the raw install (or t7kb) before stating them, and don't assert a build option or flag exists if neither supports it.
