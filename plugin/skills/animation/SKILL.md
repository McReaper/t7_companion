---
name: bo3-animation
description: How to get a custom animation from a DCC (Maya/Blender) compiled into Black Ops 3 and its many silent-failure gotchas — CoDMayaTools on modern Maya, the Cast import (SEanim is deprecated), the Quality field's frame-doubling trap, 30fps/ntsc, the export2bin single-argument conversion mode, why notetracks must be added in APE not Maya, the required Model File on an xanim asset, and how to verify a .xanim_bin actually has the format the linker reads. Use when compiling/porting/converting a character, weapon, or world animation for BO3, when reading or patching a `.xanim_export` by hand (its `PART`/`FRAME`/`OFFSET` layout, and why shifting or rebasing one cannot move where the anim plays), or diagnosing xanim errors (`Unexpected error while processing binary token`, `model was not specified`, `has no XANIM_BIN file specified`, `unable to find animation '<name>' in tree`, doubled framerate/frame count, an anim that plays 2× too fast, `has bad angle delta on frame N`, `requires "delta" type animations`, an export with `NUMPARTS 1`). Also covers what the `type` field (delta / delta3d / relative / absolute) actually controls and why AI anims cannot use `relative`, and driving CoDMayaTools headlessly to batch hundreds of anims. Distinct from bo3-assets (the broader model/material porting pipeline) and bo3-compiling (the whole-map build) — this is the animation-specific path and its traps.
---

# Getting a custom animation into Black Ops 3

The pipeline is short but every stage has a silent-failure trap that produces a *different* downstream error, so the craft is knowing which stage owns which symptom. The happy path:

```
DCC (Maya) ──export──▶ .xanim_export (TEXT)  ──export2bin──▶ .xanim_bin  ──APE xanim asset──▶ zone (xanim,<name>) ──link──▶ .ff
```

Confirm exact APE fields and zone syntax in **t7kb** and against the raw install; this skill is the pipeline and the gotchas, most of them verified the hard way. For the broader model/material/rig porting flow see **bo3-assets**; for the build stages and reading link errors see **bo3-compiling** / **bo3-debugging**; for the scene/notify side that *plays* the anim see **bo3-scripting**.

## Maya setup on a modern install

BO3 ships **no Maya plugin** — export is a community tool, **CoDMayaTools** (a Python script, not a `.mll`; it never appears in the Plug-in Manager). It lives in `%USERPROFILE%\Documents\maya\scripts\` with a `usersetup.mel` containing `python("import CoDMayaTools");` — that adds the **Call of Duty Tools** menu to the top bar.

- **Maya version:** the old Ray1235 build is **Python 2** and dies on Maya 2020+ with `Missing parentheses in call to 'print'`. On 2022+/2024 use a **Python-3 fork** (e.g. `xCortlandx/CoDMayaTools`, "Maya 2022+"). If the menu never appears, run `import CoDMayaTools` in the Script Editor and read the error — a `print` SyntaxError means wrong (Py2) fork.
- **First-run registry:** CoDMayaTools stores its config in `HKCU\Software\CoDMayaTools`. On a fresh install its first-run wizard can crash (`menuItem: Object 'AutoUpdate' not found`); pre-seed the keys to skip it — `CurrentGame`=`CoD12` (BO3's internal id), `RootPath` and `CoD12RootPath` = the BO3 root.
- **Import format is Cast.** DTZxPorter **deprecated SEanim/SEModel**; the current importer is the **Cast** plugin (`castplugin.py` + `cast.py`), loaded via the Plug-in Manager. Every modern ripper (Greyhound/Kobra/Saluki) already emits `.cast`.
- **For the FX (`.efx`) that go *with* an animation** (a rotor blur, a muzzle/impact effect tied to the anim): **rip with Kobra, not Greyhound.** Greyhound dropped XEffect/GDT export and emits **zero `.efx`**; Kobra (its fork) re-added them, so BO1/older effects come out under `Kobra/.../<game>/fx/**.efx`. If your extraction has models/anims but no effects, that's why — see **bo3-fx** for editing/playing them.

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
cd <…>/xanim_export/_reapy
export2bin.exe pb_zipline_enter.xanim_export      # ✅ correct: writes the framed bin next to it
```

- The **two-argument `export2bin in out` form is wrong** — it emits raw compressed text with no token framing, which the linker rejects at token (1).
- A **path argument** fails too: export2bin strips it to a basename and looks in the *current* directory (`Failed to read file .\…`). Run from the folder.
- **Drag-and-drop onto `export2bin.exe`** in Explorer is the same single-arg mode and works — a console flash then done.
- **From git-bash/MSYS the `/flags` are silently rewritten into paths** (`/v` → `V:/`), so drive export2bin (and gdtdb) from **PowerShell/cmd** or plain drag-drop, never MSYS.

**Verify the output format, don't trust exit 0.** A correct BO3 `.xanim_bin` is `*LZ4*`-wrapped and its decompressed body starts with a 4-byte token header then `Export filename:` (`55 c3 00 00 45 78 …`). If the decompressed body starts straight with `//` (`2f 2f`), it's the raw un-framed form and the linker will reject it. Compare against any stock `xanim_export/**/*.XANIM_BIN`.

## Reading a `.xanim_export` by hand — and why translating one changes nothing in game

The text format is simple enough to inspect or patch with a script, which is worth knowing because it settles arguments that are otherwise guesswork: a header (`NUMPARTS`, then `PART <i> "<joint>"`), then one `FRAME n` block per frame listing each `PART i` with its `OFFSET x y z`, `SCALE`, and three `X`/`Y`/`Z` rotation rows.

Two facts about the numbers, both **verified by inspection** on a Treyarch character rig:

- **`PART 0` is `tag_origin` — the root — and `PART 1` is `j_mainroot`.** Travel is the root's, so measure `PART 0`. Measuring `j_mainroot` instead manufactures a discrepancy of a few units that does not exist, and sends you hunting a bug that was never there.
- **`OFFSET`s are absolute in the animation's own space**, not parent-relative. So translating a whole clip really is one constant subtracted from every `OFFSET` of every `PART` on every frame.

**And that translation is a no-op for playback.** It is tempting — shift an export so the body ends at `(0,0,0)`, and unlinking should leave the entity exactly where the clip finished. It does nothing: `AnimScripted` is handed the **starting** transform and the engine reads travel from the root track, so shifting the file moves start and end together and the played result is identical. When an anim lands in the wrong place the **anchor** is wrong, not the file — fix it in script with `GetStartOrigin`/`GetStartAngles` (place the entity so the clip lands where you want) or `GetMoveDelta( anim, 0, 1, ent )` (read the travel), and don't touch the export. See **bo3-scripting** for playing it and **bo3-moving-platforms** for the moving-parent case.

## Notetracks: add them in APE, never bake them in Maya

**CoDMayaTools writes notetracks in a syntax the Treyarch/Scobalula converters cannot parse** — export2bin/exportxbin choke exactly on the `FRAME n "note"` line at the tail of the file (`Failed to find token … at 0x…` near EOF; or the binary form fails deep at a fixed token offset). This is a known, silent killer of an otherwise-valid anim.

So: **export with the notetrack list cleared** (or strip the `NOTETRACKS` section from the text so every `PART` is `NUMTRACKS 0`), convert cleanly, then re-add the notetracks on the **APE xanim asset** (its Notetrack / FX / Sound sections). Record the frames first — a scene/script that waits on a notify (`… waittill("my_note")`) needs them, but the *first* build/test usually doesn't, so don't let missing notetracks block getting the anim in-game.

**A ported anim usually arrives with its notetracks already gone, and nothing tells you.** Extractors keep them — a Greyhound/Wraith `.seanim` carries a `(frame, name)` list behind its `PRESENCE_NOTE` flag — but a hand-rolled `seanim → .xanim_export` converter typically *parses* that list and then writes `NUMTRACKS 0` unconditionally, so they vanish between two files that both look fine. Read them back off the **source** `.seanim` before concluding an animation never had any, then put the frames into the APE asset.

Why that matters well beyond footsteps: **BO3 delivers an AI's melee damage from a notetrack**, not from the animation itself — `_zm_behavior::notetrackBoardMelee`, registered on `NOTETRACK_ZOMBIES_BOARD_MELEE`, is what calls `DoDamage`. An attack anim stripped of its notes therefore plays perfectly and hits nobody, and re-creating the timing in script means guessing at frames the animator already chose. Reading them back is also how you discover a swing is a **double** one: BO2's bus window attack carries `fire` at frames 16 *and* 95 of 151, so a single re-timed impact lands in the pause between the two real swings and reads as random damage. (**Verified** by parsing the shipped seanims.)

Related crash, same area: an export that dies on `ValueError: No object matches name: XAnimExporterInfo.notetracks[N]` is a **CoDMayaTools bug**, not a problem with your anim — `cmds.getAttr` *raises* on a never-written element of a multi attribute, so any export slot that has never held a notetrack blows up before writing anything. The one-line patch is in **bo3-anim-retarget**.

## The APE xanim asset

Create a new `xanim` asset (don't derive from a stock one) and set:

- **Anim File** → the converted `.xanim_bin`.
- **Model File** → **required.** An empty model is the `^1model was not specified` → `xanim '…' not found` link failure. Point it at the xmodel whose skeleton the anim uses (e.g. the rig it was authored on). If that model is a shipped asset it needs no GDT of its own; otherwise compile it as an xmodel first (see **bo3-assets**).
- **Type** → `delta` for world/body/character anims, `relative` for viewmodels. When HydraX labelled the source, follow its label (`delta`/`additive`). Full list from the APE schema (`deffiles/xanim.awi`): `delta`, `delta3d`, `relative`, `absolute`, `mp_torso`, `mp_legs`, `mp_fullbody`, `additive` — where *delta* is "use for AI anims", *absolute* places everything relative to the Maya scene's (0,0,0), and *relative* relative to the parent node. See the `type` section below: for an AI anim this is not a free choice.
- **Anim File** path is **relative to the export root that matches its extension** — a `.xanim_export`/`.xanim_bin` under `xanim_export/foo/bar.xanim_bin` is written `foo\\bar.xanim_bin` (verify the root by which one actually contains the folder: `xanim_export/sword` exists, `model_export/sword` does not).
- **Use Bones** → checked for everything except viewmodels.
- **Looping** → checked only for looping anims (idle/slide/sprint loops).

Then add its line to the map/mod **`.zone`** (`xanim,<name>`), plus any model/scriptbundle it depends on, and **Link** (this is a script/asset change — no map recompile needed unless geometry changed). See **bo3-compiling**.

**An anim you play by string through an animtree has to be declared in three places, and each omission fails at a different stage** — which is why fixing one and re-linking looks like the fix didn't work:

| Declaration | Where | What its absence gives you |
|---|---|---|
| the `xanim` asset | the GDT (via APE) | a link failure naming the asset — **measured**: `xanim asset '<name>' has no XANIM_BIN file specified`, linker exit `4001000` |
| `xanim,<name>` | the `.zone` | the asset never enters the fastfile, so it is simply absent at runtime |
| the anim's name | the `.atr` animtree, under a group | links clean, then `unable to find animation '<name>' in tree '<tree>'` when you play it |

The animtree also needs its own `rawfile,animtrees/<tree>.atr` zone line — and on the script side `#using_animtree` without it kills the server silently at load (**bo3-moving-platforms**).

## `type` on an AI anim is not a free choice — and `delta3d` is the escape hatch

For anything played on a character, `delta` is close to mandatory, and two independent constraints say so:

- **The animation selector tables enforce it.** A xanim reached through an `.ai_ast` fails at link with `… is being used in the animation selector table "<x>" in "<map>.ai_ast" which **requires "delta" type animations**`.
- **`AnimScripted` depends on it.** A delta anim carries the root's motion as its own track, which is what `GetMoveDelta` / `GetAngleDelta` read and, above all, what **`GetStartOrigin`** needs — *"get the starting origin for an animation, in world coordinates, given its current position and angles"*, i.e. place the entity so the clip **ends** where you want. BO2's TranZit bus does exactly that (`start_origin = getstartorigin(...)` then `animscripted(start_origin, ...)`). In `relative`/`absolute` there is no separate root track: the bones move, the entity stays put.

So when the converter rejects an anim with **`has bad angle delta on frame N`**, do **not** "fix" it by switching to `relative`. It links, and then breaks the moment the anim enters a selector table — later, and further from the cause. **Try `delta3d` instead**: same family, still a root-motion type, and on one BO2→BO3 batch it converted 10 anims that `delta` refused, with no other change.

That fix is empirical, not understood. On those 10, what was *ruled out*: corrupt data (rotation matrices orthonormal to 1e-6), an Euler flip (`filterCurve -euler` changed nothing), the file format (rejected identically as `XANIM_EXPORT` and `XANIM_BIN`), and root tilt (constant at 3.35° across all 91 anims of the batch — a `tag_origin` bind offset, not motion). The only pattern was semantic: all 10 were "character detaches from the vehicle" clips. If `delta3d` is *also* refused by the selector table, that closes the loop and the real problem is the anim data.

## Batch/headless export: `ExportXAnim` needs three things the GUI gives it for free

Driving CoDMayaTools from a script skips the export **button**, which is where some of the setup lives. Each omission fails differently:

- **It exports only what is SELECTED.** `GetJointList` walks the whole hierarchy but includes a joint only if `selectedObjects.hasItem(dagPath)` — so selecting just the root writes a valid file with **`NUMPARTS 1`**, no error. Select every joint (`listRelatives(group, ad=True, type="joint")`), and **assert `NUMPARTS`** against that count after writing; nothing else catches it.
- **The progress bar is created by the button, not the window.** `ExportXAnim` does `cmds.progressBar(OBJECT_NAMES['progress'][0], edit=True, …)` and dies with `Object 'CoDMayaToolsProgressbar' not found`. Re-create it as `GeneralWindow_ExportSelected` does: a window named `"w" + <progress name>`, a `columnLayout`, then the `progressBar`.
- **`XAnimExporterInfo` is a SCENE node.** `RefreshXAnimWindow()` creates it (a `renderLayer` holding the notetrack/path attrs). The window's controls survive a scene change; this node does not. In a loop that re-opens a template scene per anim, call `RefreshXAnimWindow()` **after every open** or every export dies on `No object matches name: XAnimExporterInfo.notetracks[1]`.

Set the frame range and FPS by editing the window's fields directly (`<win>_FrameStartField`, `_FrameEndField`, `_FPSField`, `_qualityField`) — `ExportXAnim` reads them, not the playback range.

**`exportxbin.exe` breaks down in bulk.** A folder argument prints `No files processed` despite the tool's own help offering folders, and passing ~90 files in one call **segfaults partway** (48 converted, then a crash — with no non-zero exit to warn you). Convert **one file per invocation** in a loop and count the outputs; that is reliable.

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
- Right pose, wrong **place** → the **anchor** passed to `AnimScripted`, never the export. Editing the file cannot fix it.

## Don't invent

Tool names, flags, APE field names, and the `.xanim_bin` header bytes above are things to confirm against the raw install and the tool's own output (formats and forks churn). Where a claim here is a hard-won empirical finding on one install rather than documented ground truth, verify it still holds before relying on it, and prefer reproducing the *check* (the `55 c3` header, the APE `frameRate`) over trusting a remembered value.
