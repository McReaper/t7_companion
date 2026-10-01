---
name: compiling
description: How to build a Black Ops 3 map or mod — the Mod Tools Launcher pipeline (Compile, Light/LEDs, Link the Fast Files, Run), what each mod-tools binary does, the TA_* environment, the `zm_`/`mp_` name prefix, converting GDT/assets before linking, usermap-vs-mod builds, which stage to re-run, and driving the build headlessly — the `t7kb:build` MCP tool first, `t7kb build`/raw binaries as fallbacks. Use when building, compiling, linking or lighting a map or mod, driving the Launcher's binaries from the command line, deciding what to rebuild, or judging whether a build succeeded — the linker returns non-zero on warnings too (`exit status 1000`, `Found 1 bad bulletmeshes`, `ok` false on a Fast File that is perfectly current), and the verdict lives in `zone_source/all/assetinfo/` (the map's `.errorlog` and `.csv`). Distinct from t7kb:debugging (reading the resulting errors) and t7kb:mapping (the geometry behind compile failures).
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
- **Run** — launches the game on the built map/mod. Build stages fail on *their own stdout*; once the game is up, everything it says goes to **`console_mp.log`** instead (with the `logfile` dvar set) — **t7kb:debugging** covers where that file lands and what to grep in it. **The Launcher's Dvars dialog only applies to a Launcher Run**: a headless run (`t7kb:build`'s `run` stage, or step 5 below) gets none of them, so pass `+set developer 2 +set logfile 2` yourself when the run is for debugging — otherwise no new log is written and the old one reads as current.

A normal first build ticks all four; day-to-day you re-tick only what changed (see *Iterate fast* below).

## Assets must be converted before you link

The linker packs **converted** assets, not raw source — so the asset pipeline has to run first:

- **GDT / APE** — models, materials, images, sounds, and FX are defined in **GDTs**, edited in **APE** (`asseteditor_modtools.exe` — Asset Property Editor). Save the GDT after editing; the **GdtDB** service (`GdtDBTray.exe`) indexes GDTs so the linker can find them (the Launcher runs a `gdtdb /update` pass — watch its output line). An asset edited but not saved/indexed links stale.
- **Model/anim bins** — source models and animations are converted to engine `.bin` via **`export2bin.exe`** / **`exportxbin.exe`** (usually invoked by the export step from Maya/Blender or on GDT convert). A model that shows source-but-not-in-game usually never got binned. Porting/rigging detail lives in **t7kb:assets**.

Order per iteration: **save GDT → (GdtDB indexes) → Compile/Light as needed → Link → Run**.

## Usermap vs mod — different build target, different output

Where the build lands and what can be overridden depends on the target (this mirrors the scripting/entry-file split — see **t7kb:scripting**):

- **Usermap** — built under `usermaps/<map>/`; the map's own zone. Scripts your map zones report real `file '…' line N`; only frames inside Treyarch's shipped scripts show `missing line information`. Overriding a stock script — or any stock asset — works from a usermap too, through the assetlist CSVs rather than by switching to a mod (**t7kb:debugging** owns both).
- **Mod** — built under `mods/<modname>/`; also carries line info for stock frames, which is the one debugging reason to build as a mod. It does **not** override more than a usermap can: the linker skips an upstream-contributed asset for both targets alike.

Which assets go into the Fast File is entirely the **`.zone`** (`zone_source/*.zone`) — adding a script/model/sound means adding its line there, then re-linking. Look the exact `.zone` entry syntax up in t7kb.

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

If this skill fired over MCP, the `t7kb` server is very likely already registered — it's the same server that exposes `t7kb:search`/`t7kb:get` — so call the `build` tool directly instead of shelling out. Its parameters (from the tool's own schema): `name` (required, e.g. `"zm_mymap"`), `stages` (comma list `compile,light,link,run`, default `"compile,light,link"`; pass `"link"` alone for a script-only change), `mod` (bool, target is `mods/<name>` instead of a usermap; default `false`), `light` (`low`|`medium`|`high`, default `"medium"`), `onlyents` (bool, fast entity-only compile; default `false`), `language` (default `"english"`), `skip_gdt` (bool, skip the `gdtdb /update` pass; default `false`), `gdt_rebuild` (bool, run `gdtdb /rebuild` instead — the recovery when a GDT change leaves every asset missing; default `false`), `tools_path`/`game_path` (default `$TA_TOOLS_PATH`/`$TA_GAME_PATH`). It returns the same compact per-stage JSON report as `t7kb build --json` below, first-actionable-error included — and it inherits the same `ok: false`-on-a-warning trap, see the exit-code section further down.

**It runs synchronously and can take minutes (link) to 20–30 minutes (a full compile+light)** — set a long client-side timeout; a long wait is normal, not a hang.

### `t7kb build` — the shell fallback, when no MCP server is registered

No MCP server this session, or you specifically want the CLI's `--json`/`--verbose` output? The same tool ships this subcommand. It runs the whole pipeline with every gotcha below handled — cwd, arg passing, the detached light poll, output-file verification — and prints a **compact per-stage summary** (or `--json`) with the first actionable error, instead of the hundreds of lines each tool spews:

```
t7kb build zm_mymap                          # usermap: compile,light,link (reads $TA_TOOLS_PATH)
t7kb build zm_mymap --stages link            # script-only iteration — just re-link
t7kb build my_mod --mod --stages link        # a mod's zone
t7kb build zm_mymap --onlyents --json        # fast entity-only compile, machine-readable report
```

Flags: `--stages compile,light,link,run`, `--light low|medium|high`, `--onlyents`, `--mod`, `--tools-path`/`--game-path` (default `$TA_TOOLS_PATH`/`$TA_GAME_PATH`), `--verbose` to stream raw tool output; for the run stage, `--launcher-dvars` and `--dvar name=value` (MCP `launcher_dvars`, `dvars`). The run stage starts the game as the Launcher's Run does — `+set fs_game <map> +devmap <map>` for a usermap, `+set fs_game <mod>` for a mod — and reports a failure if the game exits within a few seconds (Steam not running or not signed in). It exits non-zero and surfaces the parsed error (e.g. a linker `SCRIPT ERROR … line N`) when a stage fails — hand that to **t7kb:debugging**.

### Last resort: the raw Launcher binaries, only if `t7kb` itself isn't installed

Both tiers above already run these exact command lines for you, gotchas and all — reach for them directly only when neither `t7kb:build` nor `t7kb build` is available, never as a shortcut around them. `%T` = `TA_TOOLS_PATH`, `%G` = `TA_GAME_PATH` (both the BO3 root; the `TA_*` vars must be set — the tools resolve their paths from them), `<map>` = full map name, `<pp>` = its first two letters (`mp`/`zm`), `<mod>`/`<zone>` = mod container and zone name.

```
# 1. Index GDTs (always first; assets edited but not indexed link stale) — run from %T\gdtdb\
%T\gdtdb\gdtdb.exe /update

# 2. Compile map geometry (BSP) — run from %T\bin\
%T\bin\cod2map64.exe -platform pc -navmesh -navvolume -loadFrom %G\map_source\<pp>\<map>.map %G\share\raw\maps\<pp>\<map>.d3dbsp
#   entity-only fast recompile: swap "-navmesh -navvolume" for "-onlyents"

# 3. Bake lighting / LEDs — headless, no GUI (quality: +low | +medium | +high)
%T\bin\radiant_modtools.exe -ledSilent +medium +localprobes +forceclean +recompute %G\map_source\<pp>\<map>.map

# 4. Link Fast Files
%T\bin\linker_modtools.exe -language english -modsource <map>                          # a map
%T\bin\linker_modtools.exe -language english -fs_game <mod> -modsource <zone>           # a mod (repeat per zone)

# 5. Run
%G\BlackOps3.exe +set fs_game <mod> +devmap <map>      # drop "+set fs_game <mod>" for a plain usermap
```

Notes: `-language english` is the minimum (Treyarch's launcher repeats `-language <lang>` per language for an all-languages build); the linker prints an `L3akMod` banner then the zone's link log; a failing stage names itself in its output — feed that to **t7kb:debugging**. Run only the stages whose input changed (see *Iterate fast*): a script-only change is `gdtdb /update` → `linker … -modsource` and nothing else.

**Shell gotchas (verified on a real headless build):**

- **Run these from PowerShell or `cmd`, not git-bash/MSYS.** MSYS rewrites the `/update` and `+low`/`+medium` arguments into filesystem paths (silently breaks `gdtdb` and the light step) *and* mis-reports a native exe's exit code — a clean `exit 0` came back as `127`. In PowerShell read the true code from `$LASTEXITCODE`.
- **Run `cod2map64` with the working directory set to `bin/`.** It loads `default_navmesh_settings.json` from the current directory; launched from elsewhere it aborts navmesh with `ERROR: Unable to load navigation mesh generation settings` (the geometry `.d3dbsp` still writes, but you get no navmesh — AI won't path).
- **Run `gdtdb` with the working directory set to its own `gdtdb/` folder**, as the Launcher does. It records asset paths relative to its cwd, so run from anywhere else against a Launcher-built database it flags **every asset as a duplicate** — which reads as a corrupted GDT set rather than a wrong folder.
- **The light step detaches.** `radiant_modtools.exe -ledSilent` is a GUI-subsystem exe: it returns immediately with no captured stdout and no usable exit code, then bakes in the background. Wait for it by polling for the output `.led` (or for the process to exit), not on a synchronous return.
- **Outputs to expect** (confirm the build by their mtime): compile → `share/raw/maps/<pp>/<map>.d3dbsp` (+ `<map>_navmesh.hkt`, `<map>.d3dprt`); light → `share/raw/maps/<pp>/<map>.led`; link → Fast Files in `usermaps/<map>/zone/` (or `mods/<mod>/zone/`): `<map>.ff` + `<map>.xpak`. One `-language <lang>` pass writes the language-neutral `<map>.ff` **and** that language's `<lang>_<map>.ff`; the other languages' localized Fast Files keep their previous content until you pass their language too. Scripts and models are in the neutral one, so an english-only pass does refresh your code. What goes *into* those localized Fast Files — `.str` files, reference naming, `linkerflag,noloc` — is **t7kb:localization**.

Confirm any flag not shown here against the raw install / t7kb before relying on it.

## The linker exits non-zero on warnings — read the errorlog, not the exit code

**Measured on a real build.** A link whose only complaint was `^3Found 1 bad bulletmeshes, dumped to …_bulletreport.csv`, and which printed `done: 0m7.08s` for every zone, still returned **1000**. The Fast File was correct and current. Anything that gates on the exit code — including `t7kb build`/`t7kb:build`, either of which then reports `ok: false` with `exit status 1000` and no message — will call that build failed, and you can lose real time "fixing" a build that already works.

The `^3` prefix is a colour code marking the line as a warning. A genuine failure names the asset and does **not** print `done:`.

So verify at the artefacts rather than the return value, all under `<map>/zone_source/all/assetinfo/`:

- **`<map>.errorlog`** — the authoritative verdict. It holds the literal `return <code>` line plus the message that produced it.
- **`<map>.csv`** — the built assetlist. Grep it for the asset you just added; that is how you prove a new xanim/model actually got packed, rather than inferring it from a green build.
- **`<map>_bulletreport.csv`** — names the bad bulletmesh, if you'd rather clear the warning than keep explaining it.

Plus the `.ff` mtime. A non-zero exit with a fresh `.ff`, a `done:` per zone, and your asset in the CSV is a **successful build**.

## Don't invent

Binary names, the `TA_*` vars, the four stages, and the `mp_` rule above are from the shipped mod-tools install — treat them as ground truth. But exact `.zone` syntax, `linker`/`cod2map` command-line flags, and dvars are shipped tokens: confirm them against the raw install (or t7kb) before stating them, and don't assert a build option or flag exists if neither supports it.
