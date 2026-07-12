---
name: bo3-animation
description: How to get a custom animation from a DCC (Maya/Blender) compiled into Black Ops 3 and its many silent-failure gotchas — CoDMayaTools on modern Maya, the Cast import (SEanim is deprecated), the Quality field's frame-doubling trap, 30fps/ntsc, the export2bin single-argument conversion mode, why notetracks must be added in APE not Maya, the required Model File on an xanim asset, and how to verify a .xanim_bin actually has the format the linker reads. Use when compiling/porting/converting a character, weapon, or world animation for BO3, or diagnosing xanim errors (`Unexpected error while processing binary token`, `model was not specified`, doubled framerate/frame count, an anim that plays 2× too fast). Distinct from bo3-assets (the broader model/material porting pipeline) and bo3-compiling (the whole-map build) — this is the animation-specific path and its traps.
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

## Notetracks: add them in APE, never bake them in Maya

**CoDMayaTools writes notetracks in a syntax the Treyarch/Scobalula converters cannot parse** — export2bin/exportxbin choke exactly on the `FRAME n "note"` line at the tail of the file (`Failed to find token … at 0x…` near EOF; or the binary form fails deep at a fixed token offset). This is a known, silent killer of an otherwise-valid anim.

So: **export with the notetrack list cleared** (or strip the `NOTETRACKS` section from the text so every `PART` is `NUMTRACKS 0`), convert cleanly, then re-add the notetracks on the **APE xanim asset** (its Notetrack / FX / Sound sections). Record the frames first — a scene/script that waits on a notify (`… waittill("my_note")`) needs them, but the *first* build/test usually doesn't, so don't let missing notetracks block getting the anim in-game.

## The APE xanim asset

Create a new `xanim` asset (don't derive from a stock one) and set:

- **Anim File** → the converted `.xanim_bin`.
- **Model File** → **required.** An empty model is the `^1model was not specified` → `xanim '…' not found` link failure. Point it at the xmodel whose skeleton the anim uses (e.g. the rig it was authored on). If that model is a shipped asset it needs no GDT of its own; otherwise compile it as an xmodel first (see **bo3-assets**).
- **Type** → `delta` for world/body/character anims, `relative` for viewmodels. When HydraX labelled the source, follow its label (`delta`/`additive`).
- **Use Bones** → checked for everything except viewmodels.
- **Looping** → checked only for looping anims (idle/slide/sprint loops).

Then add its line to the map/mod **`.zone`** (`xanim,<name>`), plus any model/scriptbundle it depends on, and **Link** (this is a script/asset change — no map recompile needed unless geometry changed). See **bo3-compiling**.

## Error → cause quick map

- `Unexpected error while processing binary token (1)` → raw un-framed bin (two-arg export2bin, or a `.xanim_bin` saved straight from Maya). Re-convert single-arg from the folder.
- `Unexpected error while processing binary token (<large>)` / `Failed to find token … at 0x…` near EOF → **notetracks** baked in Maya. Strip them, add in APE.
- `model was not specified` / `xanim '…' not found` → empty **Model File** on the asset.
- APE shows `frameRate 60`/`120`, anim plays 2×/4× too fast → **Quality** field > 0 in the Maya export.
- Import shows only `Skipping curve track … no matching node` → the **rig wasn't imported first** (anim `.cast` is curves-only).
- Menu missing / `Missing parentheses in call to 'print'` → **Python-2 CoDMayaTools on modern Maya**; use a Py3 fork.

## Don't invent

Tool names, flags, APE field names, and the `.xanim_bin` header bytes above are things to confirm against the raw install and the tool's own output (formats and forks churn). Where a claim here is a hard-won empirical finding on one install rather than documented ground truth, verify it still holds before relying on it, and prefer reproducing the *check* (the `55 c3` header, the APE `frameRate`) over trusting a remembered value.
