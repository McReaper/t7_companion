# Extraction and conversion tools, by role

Referenced from `plugin/skills/assets/SKILL.md` — read that file first. This is the tool landscape behind its one-paragraph summary. It churns: the tool that was standard a few years ago is usually superseded now, so treat names as roles and verify the current build in t7kb or the tool's own docs.

## Contents

- Rippers (source game → raw model/anim/image)
- Getting to `_bin` (the format bridge)
- Producing a GDT (content vs config)
- Whole-map geometry
- Iterating faster / keeping GDTs clean

Pick by two axes: the **source game** (which tool can even read it) and the **output you need** (raw model / `_bin` / GDT). This landscape churns — the tool that was standard a few years ago is usually superseded now, so treat names below as roles and verify the current build.

**Rippers — source game → raw model/anim/image (Cast, SEModel, SEAnim, `.MA`, `.xmodel_export`):**
- **Saluki** — the current default; a Rust rewrite that succeeds Greyhound and reads every PC CoD from CoD1 through the latest. Exports models/textures/anims/sounds as Cast/SEModel. On BO3, treat it as the *extract* step and build the GDT afterwards in APE (or MakeCents/Spiki).
- **Greyhound** — the long-time BO3-era workhorse (a Wraith fork, now maintained by dest1yo); can live-load a running game to pull what's in memory (a factory rig, a stock character). Older builds emitted a `WraithBO3.gdt` (auto GDT of models+materials, written on close); recent builds ship raw formats only.
- **Kobra** — a Greyhound fork (VenomModding) that re-adds the GDT (and XEffect) support Greyhound cut — reach for it to get the old auto-`WraithBO3.gdt` behaviour back.
- **Wraith (Archon)** — the original (DTZxPorter), superseded by Greyhound→Saluki and effectively frozen. Keep it only for an old tutorial that specifically calls for it.
- **Cordycep** — a loader/dumper for modern Ricochet-protected titles (AW, IW, MW2019, MWII, MWIII, Cold War, Vanguard, Warzone): it unpacks the game, then you export with Greyhound/Saluki. Pair it with a ripper rather than expecting it to export on its own.
- **Legion** — the tool for Apex Legends `.rpak` archives. Confirm the source game's archive format first, since one ripper rarely covers everything "newer."

**Getting to `_bin` (what APE loads) — mind the format bridge:**
- **`export2bin.exe` / `exportxbin.exe`** ship in the mod tools' `bin/` folder. They take `xmodel_export`/`xanim_export` (the text format) and produce `_bin`; `exportxbin` also converts the other way (`_bin → _export`). Their input is the `*_export` format.
  - ⚠️ On the `_bin → _export` direction, `exportxbin` v1.0.0 may abort with `ERROR: Failed to decompress binary file … return: 0`. That's the **tool**, not a corrupt rip — the bins decompress fine (they're plain LZ4: magic `*LZ4*`, size at +5, block stream at +9). Its drag-and-drop mode is its documented primary usage; from the command line **`exportx.exe -m export`** (next bullet) is the reliable route.
- Since those converters read `*_export` while Saluki emits Cast/SEModel, route Cast through **Maya/Blender** (import via the Cast/SEModel plugin, re-export as `xmodel_export` with the CoD tools) and *then* to `_bin`. A ripper that already emits `xmodel_export` (older Wraith/Greyhound) feeds the converter directly. For a weapon/character you open Maya anyway (joints, materials), so this bridge is free; for a bare prop it is one extra hop.
- **ExportX** (DTZxPorter, standalone — <https://dtzxporter.com/tools/exportx>) does the same conversion with a watcher mode that converts on save — the modern stand-in for the older **Kronos** converter (also DTZxPorter's, now superseded). Prefer ExportX or the shipped `export2bin`. It also unpacks a `_bin` **back** to text (`exportx.exe -f <file>.xmodel_bin -m export`, vs the default `-m bin`), giving a `VERSION 7` `.xmodel_export` with mesh *and* skeleton — that's how a loose ripped `.xmodel_bin` gets into Maya without recompiling and re-ripping. Check `NUMBONES`/`NUMVERTS`/`NUMFACES` in the first lines.
- **GameImageUtil** (Scobalula) preps ripped images into what BO3 wants (power-of-2, TIFF).

**Producing a GDT — two different meanings:**
- *Content GDT* (the xmodel/material/image asset for a ported model): author it in APE, or automate it — **MakeCents** (drag-drop: `_export`→`_bin`, dds→TIFF, and writes the xmodel+material GDT) or **Spiki's tools** (Treyarch-title GDT/APE automation for BO3/BO4).
- *Config GDT* (BO3's own logic assets — AI behavior trees/ASM, physicspresets, weaponfiles, sounddefs/aliases, tables, FX, script bundles): **HydraX** (Scobalula) decompiles these into GDTs under `source_data`. It is BO3→BO3 and covers the logic assets, so pair it with a ripper for the models/images.

**Whole-map geometry:**
- **Husky** (Scobalula) — extracts a whole level's BSP geometry to OBJ.
- **C2M** (sheilan102) — load a map in-game, then export it (OBJ/MTL + PNG).

**Iterate faster / keep GDTs clean:**
- **NevisX** — live GDT updates, so changes show without a full recompile.
- **GDTDupePurger** — clears duplicate GDT assets (the `Duplicate '<type>' asset` error).
- **Harmony** (Scobalula; the repo/folder may read "Harmonix") — edits sound aliases live from CSV, the audio counterpart to NevisX; the bulk of audio work lives in **t7kb:atmosphere**.
- **CoDCharacterTools** (KingslayerKyle, Maya) — automates porting a playable character rig from another CoD to T7.

Look up exact tool versions, flags, and export settings in t7kb or the tool's own docs — this list is a starting point that goes stale.
