---
name: animation
description: Getting a DCC (Maya/Blender) anim into Black Ops 3 and its silent-failure gotchas — CoDMayaTools on modern Maya, Cast import (SEanim deprecated), the Quality field's frame-doubling trap, 30fps/ntsc, export2bin's single-argument mode, notetracks belong in APE not Maya, the required Model File, verifying a `.xanim_bin`'s format, and the `type` field (delta/delta3d/relative/absolute). Use when compiling/porting a character, weapon, or world anim; patching a `.xanim_export` by hand (`PART`/`FRAME`/`OFFSET`); or diagnosing xanim errors (`Unexpected error while processing binary token`, `model was not specified`, `has no XANIM_BIN file specified`, `unable to find animation '…' in tree`, an anim that plays 2x too fast, `has bad angle delta on frame N`, `requires "delta" type animations`, `NUMPARTS 1`). Distinct from t7kb:anim-retarget (cross-gen retargeting), t7kb:assets (broader model/material porting), and t7kb:compiling (whole-map build) — this is the DCC→`.xanim_bin` compile pipeline.
---

# Getting a custom animation into Black Ops 3

The pipeline is short but every stage has a silent-failure trap that produces a *different* downstream error, so the craft is knowing which stage owns which symptom. The happy path:

```
DCC (Maya) ──export──▶ .xanim_export (TEXT)  ──export2bin──▶ .xanim_bin  ──APE xanim asset──▶ zone (xanim,<name>) ──link──▶ .ff
```

Confirm exact APE fields and zone syntax in **t7kb** (`t7kb:search` then `t7kb:get`) and against the raw install; this skill is the pipeline and the gotchas, most of them verified the hard way. For the broader model/material/rig porting flow see **t7kb:assets**; for retargeting an anim from an older CoD generation onto the BO3 skeleton see **t7kb:anim-retarget**; for the build stages and reading link errors see **t7kb:compiling** / **t7kb:debugging**; for the scene/notify side that *plays* the anim see **t7kb:scripting**.

## Maya setup on a modern install

BO3 ships **no Maya plugin** — export is a community tool, **CoDMayaTools** (a Python script, not a `.mll`; it never appears in the Plug-in Manager). It lives in `%USERPROFILE%\Documents\maya\scripts\` with a `usersetup.mel` containing `python("import CoDMayaTools");` — that adds the **Call of Duty Tools** menu to the top bar.

- **Maya version:** the old Ray1235 build is **Python 2** and dies on Maya 2020+ with `Missing parentheses in call to 'print'`. On 2022+/2024 use a **Python-3 fork** (e.g. `xCortlandx/CoDMayaTools`, "Maya 2022+"). If the menu never appears, run `import CoDMayaTools` in the Script Editor and read the error — a `print` SyntaxError means wrong (Py2) fork.
- **First-run registry:** CoDMayaTools stores its config in `HKCU\Software\CoDMayaTools`. On a fresh install its first-run wizard can crash (`menuItem: Object 'AutoUpdate' not found`); pre-seed the keys to skip it — `CurrentGame`=`CoD12` (BO3's internal id), `RootPath` and `CoD12RootPath` = the BO3 root.
- **Import format is Cast.** DTZxPorter **deprecated SEanim/SEModel**; the current importer is the **Cast** plugin (`castplugin.py` + `cast.py`), loaded via the Plug-in Manager. Every modern ripper (Greyhound/Kobra/Saluki) already emits `.cast`.
- **For the FX (`.efx`) that go *with* an animation** (a rotor blur, a muzzle/impact effect tied to the anim): **rip with Kobra, not Greyhound.** Greyhound dropped XEffect/GDT export and emits **zero `.efx`**; Kobra (its fork) re-added them, so BO1/older effects come out under `Kobra/.../<game>/fx/**.efx`. If your extraction has models/anims but no effects, that's why — see **t7kb:fx-editing** for editing/playing them.

## Importing a ripped anim to re-export it

A ripped **anim `.cast` is curves-only — it carries no skeleton.** Import it into an empty scene and *every* track is `Skipping curve track "…" no matching node was found` and nothing moves. **Import the rig model first** (its `.cast`/`.semodel`), *then* import the anim `.cast` onto it. A handful of leftover skips (`tag_camera`, cosmetic/`torso_stabilizer` bones) are normal — those tags simply aren't in the mesh.

**First-person body vs viewmodel:** a `pb_*` ("player body") anim animates the **full body skeleton** and is a *first-person body* setup (the camera rides a `tag_camera`, you see that body's arms) — export it `Type delta`, **Use Bones checked**. A `vm_*` viewmodel anim is hands-only on `tag_view` — `Type relative`, **Use Bones unchecked**. Broken/lazy legs on a `pb_` anim are expected (never rendered) — don't "fix" them.

## Exporting from Maya — the two traps that corrupt timing

**1. Scene FPS must be 30 (`ntsc`), not 60 (`ntscf`).** CoD anims are 30fps. `New Scene` resets to 24fps and the Preferences default often won't stick, so set it explicitly each session: `currentUnit -time ntsc;` (verify: `currentUnit -q -time` → `ntsc`). At 24fps a 30fps import is resampled (a 161-frame anim shows **128**); at 60fps everything is authored double.

**2. The "Quality (0–10)" field is a `2^quality` frame *multiplier*, not a keyframe-quality knob.** Its code is `fps = fps * 2^quality` and the frame range is scaled the same. So **Quality 1 doubles** framerate and frame count (30fps/85f → 60fps/170f), Quality 7 is ×128 (a multi-hundred-MB file). **Use Quality 0** — native sampling, true 30fps, true frame count. (Symptom of getting this wrong: APE's *Exported Notetracks* shows `frameRate 60`/`120` and `totalFrames` at 2×/4× the real count, and the anim plays too fast.)

**Sanity-check the export in APE by its `frameRate`/`totalFrames`, and against a known-30fps *stock* xanim** loaded the same way — APE reads stock correctly, so if stock shows 30 and yours shows 60, the doubling is in your file, not APE.

## Conversion: `.xanim_export` (text) → `.xanim_bin` — the mode matters

**Save from Maya as `.xanim_export`, not `.xanim_bin`.** CoDMayaTools decides format by the **Save-to extension**: `.xanim_bin` runs its own `WriteFile_Bin` (a binary the BO3 linker rejects with `Unexpected error while processing binary token`); anything else runs `WriteFile_Raw` — the **text** `.xanim_export` the shipped converter expects.

Convert that text with the mod tools' **`bin/export2bin.exe`**, and **invoke it as a single argument from the file's own directory**:

```
cd <bo3_root>/xanim_export/<your_folder>
export2bin.exe pb_zipline_enter.xanim_export      # ✅ correct: writes the framed bin next to it
```

- The **two-argument `export2bin in out` form is wrong** — it emits raw compressed text with no token framing, which the linker rejects at token (1).
- A **path argument** fails too: export2bin strips it to a basename and looks in the *current* directory (`Failed to read file .\…`). Run from the folder.
- **Drag-and-drop onto `export2bin.exe`** in Explorer is the same single-arg mode and works — a console flash then done.
- **Why the single argument behaves this way:** the tool's own usage line is `export2bin [/single input] [/s] [/v] [/u] [/nt=N] [/o=dirname] [input output|pattern]` — one argument is a **pattern matched in the current directory**, so a whole folder converts with `export2bin /s /u *` from the export root. `/single` and `/o=dirname` are official but untested here. The Launcher's built-in Export2Bin writes to `model_export/export2bin/` by default and, with Overwrite off, logs `Skipping file '…' (file already exists)` — a stale bin with no error. (**Verified** from both binaries' strings.)
- **From git-bash/MSYS the `/flags` are silently rewritten into paths** (`/v` → `V:/`), so drive export2bin (and gdtdb) from **PowerShell/cmd** or plain drag-drop, never MSYS.

**Verify the output format, don't trust exit 0.** A correct BO3 `.xanim_bin` is `*LZ4*`-wrapped and its decompressed body starts with a 4-byte token header then `Export filename:` (`55 c3 00 00 45 78 …`). If the decompressed body starts straight with `//` (`2f 2f`), it's the raw un-framed form and the linker will reject it. Compare against any stock `xanim_export/**/*.XANIM_BIN`.

## Reading a `.xanim_export` by hand — and why translating one changes nothing in game

The text format (`NUMPARTS`, `PART <i> "<joint>"`, then per-`FRAME` `OFFSET`/`SCALE`/rotation rows) is simple enough to inspect or patch with a script. Two facts settle most arguments: **`PART 0` is `tag_origin`** (measure travel there, not on `j_mainroot`), and **`OFFSET`s are absolute** in the anim's own space. And shifting every `OFFSET` so the clip "ends at (0,0,0)" changes nothing in game — `AnimScripted` is handed the **start** transform and reads travel from the root track, so fix a wrong landing with `GetStartOrigin` in script (**t7kb:scripting**), never in the file. Detail: **`references/xanim-export-format.md`**.

## Notetracks: add them in APE, never bake them in Maya

**CoDMayaTools writes notetracks in a syntax the Treyarch/Scobalula converters cannot parse** — export2bin/exportxbin choke exactly on the `FRAME n "note"` line at the tail of the file (`Failed to find token … at 0x…` near EOF; or the binary form fails deep at a fixed token offset). This is a known, silent killer of an otherwise-valid anim.

So: **export with the notetrack list cleared** (or strip the `NOTETRACKS` section from the text so every `PART` is `NUMTRACKS 0`), convert cleanly, then re-add the notetracks on the **APE xanim asset** (its Notetrack / FX / Sound sections). Record the frames first — a scene/script that waits on a notify (`… waittill("my_note")`) needs them, but the *first* build/test usually doesn't, so don't let missing notetracks block getting the anim in-game.

**A ported anim usually arrives with its notetracks already gone, and nothing tells you.** Extractors keep them — a Greyhound/Wraith `.seanim` carries a `(frame, name)` list behind its `PRESENCE_NOTE` flag — but a hand-rolled `seanim → .xanim_export` converter typically *parses* that list and then writes `NUMTRACKS 0` unconditionally, so they vanish between two files that both look fine. Read them back off the **source** `.seanim` before concluding an animation never had any, then put the frames into the APE asset.

Why that matters well beyond footsteps: **BO3 delivers an AI's melee damage from a notetrack**, not from the animation itself — `_zm_behavior::notetrackBoardMelee`, registered on `NOTETRACK_ZOMBIES_BOARD_MELEE`, is what calls `DoDamage`. An attack anim stripped of its notes therefore plays perfectly and hits nobody, and re-creating the timing in script means guessing at frames the animator already chose. Reading them back is also how you discover a swing is a **double** one: BO2's bus window attack carries `fire` at frames 16 *and* 95 of 151, so a single re-timed impact lands in the pause between the two real swings and reads as random damage. (**Verified** by parsing the shipped seanims.)

Related crash, same area: an export that dies on `ValueError: No object matches name: XAnimExporterInfo.notetracks[N]` is a **CoDMayaTools bug**, not a problem with your anim — `cmds.getAttr` *raises* on a never-written element of a multi attribute, so any export slot that has never held a notetrack blows up before writing anything. The one-line patch is in **t7kb:anim-retarget**.

## The APE xanim asset

Create a new `xanim` asset (don't derive from a stock one) and set:

- **Anim File** → the converted `.xanim_bin`.
- **Model File** → **required.** An empty model is the `^1model was not specified` → `xanim '…' not found` link failure. Point it at the xmodel whose skeleton the anim uses (e.g. the rig it was authored on). If that model is a shipped asset it needs no GDT of its own; otherwise compile it as an xmodel first (see **t7kb:assets**).
- **Type** → `delta` for world/body/character anims, `relative` for viewmodels. When HydraX labelled the source, follow its label (`delta`/`additive`). Full list from the APE schema (`deffiles/xanim.awi`): `delta`, `delta3d`, `relative`, `absolute`, `mp_torso`, `mp_legs`, `mp_fullbody`, `additive` — where *delta* is "use for AI anims", *absolute* places everything relative to the Maya scene's (0,0,0), and *relative* relative to the parent node. See the `type` section below: for an AI anim this is not a free choice.
- **Anim File** path is **relative to the export root that matches its extension** — a `.xanim_export`/`.xanim_bin` under `xanim_export/foo/bar.xanim_bin` is written `foo\\bar.xanim_bin` (verify the root by which one actually contains the folder: `xanim_export/sword` exists, `model_export/sword` does not).
- **Use Bones** → checked for everything except viewmodels.
- **Looping** → checked only for looping anims (idle/slide/sprint loops).

With the MCP tools, `t7kb:gdt_edit` creates it: asset type `xanim`, and in `set` the GDT keys behind those APE labels — `filename` (Anim File), `model` (Model File), `type`, `useBones`, `looping` (verified in `deffiles/xanim.awi`; `t7kb:gdt_schema` lists the rest). `t7kb:gdt_check` then confirms the Anim File exists under `xanim_export/`, which catches the path-root mistake above. It doesn't check that `model` is set: do that yourself.

Then add its line to the map/mod **`.zone`** (`xanim,<name>`), plus any model/scriptbundle it depends on, and **Link** (this is a script/asset change — no map recompile needed unless geometry changed). See **t7kb:compiling**.

**An anim you play by string through an animtree has to be declared in three places, and each omission fails at a different stage** — which is why fixing one and re-linking looks like the fix didn't work:

| Declaration | Where | What its absence gives you |
|---|---|---|
| the `xanim` asset | the GDT (via APE) | a link failure naming the asset — **measured**: `xanim asset '<name>' has no XANIM_BIN file specified`, linker exit `4001000` |
| `xanim,<name>` | the `.zone` | the asset never enters the fastfile, so it is simply absent at runtime |
| the anim's name | the `.atr` animtree, under a group | links clean, then `unable to find animation '<name>' in tree '<tree>'` when you play it |

The animtree also needs its own `rawfile,animtrees/<tree>.atr` zone line — and on the script side `#using_animtree` without it kills the server silently at load (**t7kb:moving-platforms**).

## `type` on an AI anim is not a free choice — and `delta3d` is the escape hatch

For anything played on a character, `delta` is close to mandatory, and two independent constraints say so:

- **The animation selector tables enforce it.** A xanim reached through an `.ai_ast` fails at link with `… is being used in the animation selector table "<x>" in "<map>.ai_ast" which **requires "delta" type animations**`.
- **`AnimScripted` depends on it.** A delta anim carries the root's motion as its own track, which is what `GetMoveDelta` / `GetAngleDelta` read and, above all, what **`GetStartOrigin`** needs — *"get the starting origin for an animation, in world coordinates, given its current position and angles"*, i.e. place the entity so the clip **ends** where you want. BO2's TranZit bus does exactly that (`start_origin = getstartorigin(...)` then `animscripted(start_origin, ...)`). In `relative`/`absolute` there is no separate root track: the bones move, the entity stays put.

So when the converter rejects an anim with **`has bad angle delta on frame N`**, do **not** "fix" it by switching to `relative`. It links, and then breaks the moment the anim enters a selector table — later, and further from the cause. **Try `delta3d` instead**: same family, still a root-motion type, and on one BO2→BO3 batch it converted 10 anims that `delta` refused, with no other change.

That fix is empirical, not understood. On those 10, what was *ruled out*: corrupt data (rotation matrices orthonormal to 1e-6), an Euler flip (`filterCurve -euler` changed nothing), the file format (rejected identically as `XANIM_EXPORT` and `XANIM_BIN`), and root tilt (constant at 3.35° across all 91 anims of the batch — a `tag_origin` bind offset, not motion). The only pattern was semantic: all 10 were "character detaches from the vehicle" clips. If `delta3d` is *also* refused by the selector table, that closes the loop and the real problem is the anim data.

It is less exotic than it sounds: in the stock GDTs under `xanim_export/` and `model_export/`, `ai_*` anims are `delta`/useBones 1 (665), the viewmodel GDTs' `vm_*` are `relative`/useBones 0 (plus a few movement-bob `additive`), and **33 of 46 Treyarch fxanim props are `delta3d`**.

## Batch/headless export: the script path skips setup the GUI does for you

Driving CoDMayaTools' `ExportXAnim` from a script exports only the **selection** (select every joint and assert `NUMPARTS`, or you silently get `NUMPARTS 1`), needs the progress bar and the `XAnimExporterInfo` scene node the export **button** creates (`Object 'CoDMayaToolsProgressbar' not found`, `No object matches name: XAnimExporterInfo.notetracks[1]`), and reads frame range/FPS from the window's fields. `exportxbin.exe` segfaults partway through a large batch with no non-zero exit — convert one file per call and count the outputs. Full recipe: **`references/batch-export.md`**.

## Siege models are a different pipeline — they refuse xanims

A ported `p7_fxanim_*_smod` prop (rope, cloth, debris destruction) is a **siege** model (APE's xmodel shows *Is Siege*): APE refuses an xanim on it — `This model is a siege model, you can not use it with xanims, you should use sanims.` — and requires the model be typed `rigid` even though it animates (`This is a siege model, it has to be 'rigid'.`). Its animation is a `.siege_anim_source` in a **`sanim`** asset (`animationFile` + `boneLayout` = the `_smod`; stock `model_export/t7_fxanim_zm.gdt` shows the shape), and it plays **client-side only** — a Client-script-type scene (`scriptbundle.awi`: *"it has to be Client Script Type"*), `SiegeCmd`/`animation::play_siege` from CSC, or the `misc_model` KVP `siege_anim`. No GSC can play it. (**Verified in the install**; the Maya "CoD Siege Anim Source" export path is community.)

## Error → cause quick map

- `Unexpected error while processing binary token (1)` → raw un-framed bin (two-arg export2bin, or a `.xanim_bin` saved straight from Maya). Re-convert single-arg from the folder.
- `Unexpected error while processing binary token (<large>)` / `Failed to find token … at 0x…` near EOF → **notetracks** baked in Maya. Strip them, add in APE.
- `model was not specified` / `xanim '…' not found` → empty **Model File** on the asset.
- APE shows `frameRate 60`/`120`, anim plays 2×/4× too fast → **Quality** field > 0 in the Maya export.
- Import shows only `Skipping curve track … no matching node` → the **rig wasn't imported first** (anim `.cast` is curves-only).
- Menu missing / `Missing parentheses in call to 'print'` → **Python-2 CoDMayaTools on modern Maya**; use a Py3 fork.
- `has bad angle delta on frame N` → try **`type delta3d`**, not `relative` (see the `type` section — `relative` links but breaks at the selector table).
- `… requires "delta" type animations` (at the `.ai_ast`) → the asset's **Type** is not in the delta family. This is the failure `relative` defers to.
- Exported file has **`NUMPARTS 1`** → only the root was selected; CoDMayaTools exports the **selection**, not the hierarchy under it.
- `Object 'CoDMayaToolsProgressbar' not found` / `No object matches name: XAnimExporterInfo.notetracks[1]` → **scripted** export without the button's setup; see the batch section.
- `xanim asset '<name>' has no XANIM_BIN file specified` (linker exit `4001000`) → zoned but **not declared in the GDT**; three declarations are needed, see the APE section.
- `unable to find animation '<name>' in tree '<tree>'` → the anim is built and zoned but **not listed in the `.atr`**.
- `This model is a siege model, you can not use it with xanims` → a `_smod` prop: author a **`sanim`**, play it client-side (siege section).
- Right pose, wrong **place** → the **anchor** passed to `AnimScripted`, never the export. Editing the file cannot fix it.

## Don't invent

Tool names, flags, APE field names, and the `.xanim_bin` header bytes above are things to confirm against the raw install and the tool's own output (formats and forks churn). Where a claim here is a hard-won empirical finding on one install rather than documented ground truth, verify it still holds before relying on it, and prefer reproducing the *check* (the `55 c3` header, the APE `frameRate`) over trusting a remembered value.
