---
name: bo3-assets
description: How to get a model, material/texture, or animation into Black Ops 3 — extracting from other CoD titles (Saluki, Greyhound, Cordycep), the weapon/character porting pipeline (Maya/Blender export → APE compile → materials → anims), rigging custom models, and common export/GDT pitfalls. Use for porting, custom modeling, texturing, or animation work, as distinct from GSC/CSC scripting.
---

# Assets: models, materials, porting, animation

Sourcing here is mixed: raw Discord threads run low reliability (~0.25), but a real chunk of this domain is backed by UGX/ModMe/T7-wiki writeups (~0.70) and a few schema/source-verified references (~0.90 — e.g. collmaps, script bundles). Check the `reliability` score per hit rather than assuming this whole domain is low-confidence. Look up exact APE fields, GDT syntax, and material settings in **t7kb** (`search` then `get`); this skill is the pipeline and the gotchas around it. For the *source* game itself — decoding its engine lineage and finding its GSC/asset dumps to learn the real names and structure of what you're ripping — see **bo3-crossref**.

## Extraction tools aren't interchangeable

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
- **Harmony** (Scobalula; the repo/folder may read "Harmonix") — edits sound aliases live from CSV, the audio counterpart to NevisX; the bulk of audio work lives in **bo3-atmosphere**.
- **CoDCharacterTools** (KingslayerKyle, Maya) — automates porting a playable character rig from another CoD to T7.

Look up exact tool versions, flags, and export settings in t7kb or the tool's own docs — this list is a starting point that goes stale.

## The weapon porting pipeline (also the template for character/prop ports)

Order matters here — skipping ahead (e.g. exporting before attaching joints) is the usual cause of a silently broken port:

1. **Obtain** the source assets (Saluki/Greyhound/Cordycep/Legion — pick per source game, see above). Rip **every** piece — scope, magazine, and body are separate models.
2. **Prepare in Maya.** Open the `.ma` via `File > Open`, never drag-and-drop — dragging merges it into the scene and namespaces everything, causing problems later.
3. **Attach imported joints** to the main model's root (`j_gun`/`tag_weapon`).
4. **Export** via **Call of Duty Tools → Export XModel**, selecting the full hierarchy (**`Select > Hierarchy`**, not just the root/`tag_origin` — clicking only the root joint looks like it selected everything but doesn't, and is the single most common "export fails silently" cause).
5. **Convert** the exported `xmodel_export` from step 4 to `.xmodel_bin` with `export2bin.exe`/`exportxbin.exe` (shipped in `bin/`) or ExportX. This Maya-produced `xmodel_export` is also what bridges a Cast/SEModel rip (e.g. Saluki) into a format the converter accepts.
6. **Compile in APE**: new GDT (save it under `Black Ops 3\source_data` — a GDT saved elsewhere silently won't reappear in APE/Radiant next session), `xmodel` asset, type **animated**, `BulletCollisionLOD` = LOD0, submodels parented to the main model's root tag.

**ADS export tags depend on the source game** — a common silent-fail point: from a Treyarch-source rig, export only `tag_view`+`tag_torso` for ADS; from an IW-source rig, export only `tag_view`+`tag_ads`. Grabbing the wrong pair for the source you ripped from is a frequent cause of broken ADS.

**Naming convention** (Treyarch's own): `<game_id>_<usage>_<class>_<name>_<type>`, e.g. `t6_attach_mag_dsr50_view` — keep ported assets consistent with this so your GDT stays navigable.

**Materials**: BO3 is PBR — diffuse/albedo, ambient occlusion, normal, specular, gloss. From an older CoD, the color map (`_c`) maps to diffuse and the environment map (`_e`/`_env`) maps to gloss. Weapon materials: `Material Category` = Geometry, `Material Type` = `lit_weapon`. Two easy-to-hit pitfalls: leaving **Surface Type** at `<error>`/`error` (use `<none>` instead), and non-power-of-2 image dimensions (BO3 requires power-of-2 textures). Set one material's fields, then duplicate + rename for the rest rather than re-entering settings each time — same trick works for xanim assets.

**Animations**: convert with the community `conversion_rig.ma` (root bone becomes a child of `t7:tag_weapon_right`, rename joints with `renameRig.mel`, import the anim, strip the rig with `removeNamespace.mel`, rename the root back to `tag_weapon`). **Cold War's animation filenames are dehashed** — pull weapon anims from Modern Warfare instead, it works more reliably. Compile in APE as an `xanim` asset — the settings differ by animation kind: **viewmodel anims** use Use Bones unchecked / `Type` = `relative`; **everything else (world/character anims)** uses Use Bones checked / `Type` = `delta` — mixing these up is a common cause of a ported anim looking right on the weapon but broken on the world model, or vice versa. Check **Looping** for idle/sprint-loop/slide-loop/swim anims regardless of type. For the full DCC→`.xanim_export`→`export2bin`→APE pipeline and its many silent-failure traps (the Quality field's frame-doubling, the export2bin single-argument mode, notetracks that must be added in APE not Maya, the required Model File), see **bo3-animation**.

Finish: `bulletweapon` asset in APE, add to the map's zone file (`weapon,<name>`), drag it in from Radiant's entity browser.

## Custom/ported characters and zombies

Same shape as weapons, plus rigging: extract the **factory zombie/character rig** (Greyhound), import your mesh in Maya/Blender, bind joints to the mesh, and weight-paint. **Split the head from the body and rig it to the default BO3 head separately** if you need jaw movement (a community `binder.mel` script automates the head-to-body attachment). An alternative some modders use: import a stock BO3 character alongside yours, copy its weight paint, then swap the armature onto your model and delete the stock one — works as long as the joint hierarchy matches.

For wiring a rigged zombie into custom AI behavior (archetypes, spawners) rather than the modeling/rigging itself, see **bo3-zombies-ai**.

Swapping the **stock Zombies player model** is a separate, simpler task from custom-character porting: swap to another stock crew member via `zm_usermap.gsc`'s character-index override, or build a fully custom player model via `customizationtable`/`playerbodytype`/`playerbodystyle` duplication in APE.

## Collision: ported/custom models have zero collision by default

Unlike stock assets, a ported or custom xmodel has **no collision unless you give it one** — a bare model will let players/zombies walk straight through it. Fix via the xmodel's `CollisionMap` field pointing at a `.map` under `share/raw/collmaps/`, textured with `clip_physics`; the collmap must be **brush-only** (patches don't produce valid collision). Scaled models need `scaleCollMap` set to match, and `use_collmap`/`no_collmap`/`use_misc_models_collmaps` control whether it actually gets baked in. This is easy to miss because the model looks and behaves fine right up until something needs to path around or stand on it.

## Script bundles (the asset-pipeline side of data-driven content)

Scene/cinematic, vehicle, killstreak, and collectible data lives in **script bundles** — GDT-authored, data-driven assets read at runtime via `struct::get_script_bundle`, `get_script_bundle_list`, and `get_script_bundle_instances` (verified against shipped `struct.gsc`). Reach for a script bundle instead of hardcoding a table in GSC when the data is really asset content (e.g. a set of placeable collectible variants) — it keeps the data in APE/GDT where the rest of the pipeline expects it.

## Hand-authoring a GDT entry: copy a working one, then scrub what you didn't mean to inherit

A GDT is plain text, so you can write entries directly instead of clicking through APE — same result, and it versions. The reliable method is to **copy an existing entry of the same asset type and substitute**, because an xmodel entry alone carries ~70 fields and a `zbarrier` ~140; hand-listing them invites a missing-field failure. But a copied entry drags the donor's asset paths with it, and the resulting errors point at the *donor*, which is confusing until you know to look. All four of these were hit in one sitting porting a BO2 vehicle, in this order:

- **Backslashes in `filename` must be doubled.** `"filename" "folder\\model\\model_lod0.xmodel_bin"` — with single backslashes the GDT parser eats them as escape sequences and the linker reports a path with the separators simply gone (`folderModelmodel_lod0...`), which reads like a string-concatenation bug rather than an escaping one.
- **Scrub the donor's LOD fields.** `mediumLod` / `lowLod` / `lowestLod` still point at the *donor's* meshes, so your model links with another model's lower LODs. Symptom: `Part 'tag_animate' in lower lod '<donor>' doesn't have the same name as part 'tag_body' in higher lod '<yours>'`. Blank every LOD path you don't actually supply.
- **`BulletCollisionLOD` must name a LOD you have.** Inherited `Low` on a LOD0-only model gives `Lod 'Low' does not exist in model '<name>', but it is set as the bullet collision`. Same for `ShadowLOD`.
- **Animated props often fail the LOD bounds check.** `XModel '<name>' failed. Bounding box of all LODs 2.17 times base mesh` on ripped `*_anim_*` props — their exported bounds don't match what the linker expects. Substituting an already-ported equivalent unblocks the build while you sort the source model out.

## `export2bin` resolves its argument against its own working directory

`export2bin.exe path/to/model.xmodel_export` fails with `ERROR: Failed to read file .\model.xmodel_export` — note the `.\`. It ignores the directory you gave it. **Run it with the working directory set to the model's own folder** and pass the bare filename. It writes `.XMODEL_BIN` in caps; rename to lowercase to match the `model_export/` convention. (Same class of cwd sensitivity as `cod2map64` needing to run from `bin/` — see **bo3-compiling**.)

## After hand-editing a GDT, `gdtdb /update` does not see it — you need `/rebuild`

The most expensive trap in this whole area, because the symptom is wildly misleading. Edit a GDT by hand, run the normal pipeline, and the linker reports **every asset in the game as missing** — `skybox_default_day`, `luts_t7_default`, the stock zombie spawner, things you never touched — while `gdtdb.exe /update` cheerfully prints `processed (0 GDTs) (0 assets)`. It looks like you corrupted the database. You didn't: the incremental pass just doesn't notice hand-written files.

```
gdtdb.exe /rebuild        # processed (3004 GDTs) (260203 assets)
```

Budget ~60-90s and run it after **every** manual GDT edit. And under git-bash, MSYS rewrites `/rebuild` into a filesystem path so the tool silently prints its usage instead of running — prefix with `MSYS2_ARG_CONV_EXCL="*"` (same MSYS argument-mangling as the `/update` and `+medium` flags in **bo3-compiling**).

## Material settings a rip gets wrong, and how to tell

Porting a model's *materials* is where a rip stops looking like the original. These are the ones that bite, all verified porting a BO2 vehicle:

- **`normalHeightScale` — turn it down to ~0.1-0.2.** An older-title normal map reads far too strong in BO3 and produces **hard lighting bands, panel by panel**, that look for all the world like a UV, blend-mask or texture-atlas problem. Diagnose it by blanking `normalMap` for one material and rebuilding: if the banding vanishes, it's intensity, not the map.
- **`materialType` and `materialCategory` must agree**, or APE warns: `lit`→`Geometry`, `lit_plus`→`Geometry Plus`, `lit_advanced*`→`Geometry Advanced`, `lit_decal`→`Decal`. A copied donor entry usually carries the wrong pairing.
- **`lit_advanced_fullspec` silently refuses to expose `colorMap`** if `aoMap` and `glossMap` are missing — which a rip never has. Error reads `material '<name>' using technique '...' doesn't expose a 'colorMap' texture`. Drop to `lit`.
- **The `colorMap` slot needs `coreSemantic` `sRGB3chAlpha`.** Switching a diffuse to `sRGB3ch` to dodge a packed alpha breaks the binding entirely, with the same "doesn't expose a colorMap" error.
- **`baseImage` is relative to the install root**, so it includes the `texture_assets\\` prefix — not relative to `texture_assets/` itself.
- **Old-title `SurfaceType` values don't transliterate.** `PAINTED_METAL` → `paintedmetal` (no underscore), and `default` → `<none>`, else the linker aborts with `surfaceTypeName 'default' not in surfaceTypeParms array`.

## Double-sided: `nocull` is a material type, not a flag

A model whose backfaces don't render is fixed by a **`nocull` material type** — `lit_nocull`, `lit_alphatest_nocull`, `lit_transparent_nocull`, `lit_detail_nocull` and `_advanced`/`_plus` variants all ship. `doubleSidedLighting` is *not* it: that controls how backfaces are **lit**, not whether they're drawn, and setting it changes nothing visible.

Worth checking the geometry first so you know which problem you have: if every face of the material has a unique position triple (no duplicated triangles with reversed winding), the mesh is genuinely single-sided and only `nocull` can save it. Duplicating the faces in the export also works but doubles the triangles and is not reversible from the GDT.

## An alpha channel's *percentage* tells you nothing — its distribution does

Ripped diffuse maps frequently carry a packed gloss/spec mask in alpha, and a real alpha cutout looks identical if you only measure "what fraction of pixels are non-opaque". Alpha-testing a packed mask punches **black speckles** through the surface, which reads as a corrupt texture.

Map the alpha spatially instead — a coarse grid of "percent of pixels below threshold" per cell. A genuine cutout is a **compact, sharp-edged region**; a packed mask is scattered noise across the whole sheet. One bus material showed a solid rectangular transparent block over half the texture (real vents), another only 2-3% scattered (gloss mask), and the naming confirmed both — Treyarch shipped a `_opq` twin of the vented material.

## A failed link isn't always a failed link

The linker can print `done: 0m5.58s` for every zone, write fresh Fast Files, and *still* exit non-zero — a `^3Found N bad bulletmeshes` warning is enough to do it, and wrappers that gate on exit status will report FAIL. **Check the `.ff` timestamps before believing the failure.** The accompanying `zone_source/all/assetinfo/<map>_bulletreport.csv` names the offending models with their triangle counts and average face area (a ripped vehicle at 25k collision tris against a recommended 50-unit average area will trip it) — a real quality warning worth fixing with a proper collmap, but not a build blocker.

## Common pitfalls

- **Blender's current Blender-COD plugin (the GitHub one) can break UV export.** If ported textures look wrong/shifted after export, community consensus is to fall back to a legacy release rather than debug the current one.
- **Remapping every material to a placeholder gets a rip linking, and then you forget.** Rewriting the `MATERIAL n "name"` lines in an `.xmodel_export` to one existing material is a legitimate way to see geometry in-game before the material port is done — but the model now *is* that texture, and it looks like an asset bug later. Note it, or don't do it.
- **Duplicate GDT asset errors** (`Duplicate 'material' asset '<name>' found in ...gdt:<line>`) mean the same asset name exists in two GDTs (yours and a shared/stock one) — delete your duplicate entry, don't rename around it; it's a naming collision, not a corruption. **GDTDupePurger** clears these in bulk.
- **Ragdoll behavior for a custom model** goes through `RagdollSettings` — a dragged-in stock ragdoll setup silently keeps stock proportions/behavior unless you edit it for your model.

## Don't invent

Community export/rigging workflows here have real version-specific gotchas (a plugin build, a Maya version, a specific rig) that change over time — verify the current tool/plugin version against what the corpus and raw install actually show before asserting a fix still applies, rather than assuming yesterday's Discord answer is timeless.
