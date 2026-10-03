---
name: assets
description: How to port a model, material or rig into Black Ops 3 and author or check its GDT (by hand or via gdt_* tools, images from textures too) — picking a ripper (Saluki, Kobra, Cordycep), bridging `_export`/`_bin`, APE material fields (`normalHeightScale`, `colorMap00` emissive, `nocull`, `SurfaceType`), and collision (`CollisionMap`, `scaleCollMap`). Use when a rip's normal map bands per-panel, an emissive part won't glow, a ported model has zero collision, a GDT edit leaves every asset missing (`gdtdb /rebuild`), a material fails with `failed to open source file` on a `techsetdef_*.hlsl`, APE throws `doesn't expose a 'colorMap' texture` / `Duplicate 'material' asset` / a LOD bounding-box mismatch, or `exportxbin` aborts with `Failed to decompress binary file`. Distinct from t7kb:animation (the DCC→`.xanim_bin` compile pipeline) and t7kb:anim-retarget (cross-generation Maya HumanIK retargeting) — both assume the model/rig is already in — and t7kb:crossref (another title's source as reference).
---

# Assets: models, materials, and porting

**A port that looks right in APE can still be wrong in places APE doesn't show** — a donor's leftover fields, a material slot named nothing like its job, collision that was never baked. This skill is the pipeline and those traps; look up APE fields, GDT syntax and material settings in **t7kb** (`t7kb:search` then `t7kb:get`), weighing `reliability` per hit (Discord ~0.25, UGX/ModMe/T7-wiki ~0.70, schema-verified references ~0.90). Ground on the APE schemas in `deffiles/*.awi` and stock GDTs dated with the install. For the *source* game — its engine lineage and script dumps — see **t7kb:crossref**.

## Extraction tools aren't interchangeable — pick by source game and by the output you need

- **Rip** with **Saluki** (the current default, every PC CoD, emits Cast/SEModel) — or **Kobra** when you also need the old auto-GDT or **`.efx`** (Greyhound dropped both), **Cordycep** to unpack Ricochet-era titles for a ripper, **Legion** for Apex `.rpak`.
- **Bridge to `_bin`**: the shipped `export2bin.exe`/`exportxbin.exe` read only the text `*_export` format, so a Cast rip goes through Maya/Blender first (re-export as `xmodel_export`); **ExportX** converts both ways from the command line (`exportx.exe -f <file>.xmodel_bin -m export` unpacks a loose bin to text with its skeleton).
- **GDTs**: a *content* GDT (xmodel/material/image) is APE, MakeCents or Spiki; a *config* GDT (behavior trees, weaponfiles, aliases, script bundles) is decompiled by **HydraX**.

Roles, alternatives, whole-map exporters (Husky, C2M), GDT helpers (NevisX, GDTDupePurger) and their version traps: **`references/extraction-tools.md`**.

## The weapon porting pipeline (also the template for character/prop ports)

Order matters — skipping ahead (e.g. exporting before attaching joints) silently breaks the port:

1. **Obtain** the source assets (Saluki/Greyhound/Cordycep/Legion — pick per source game, see above). Rip **every** piece — scope, magazine and body are separate models.
2. **Prepare in Maya.** Open the `.ma` via `File > Open`, never drag-and-drop — dragging merges it into the scene and namespaces everything, causing problems later.
3. **Attach imported joints** to the main model's root (`j_gun`/`tag_weapon`).
4. **Export** via **Call of Duty Tools → Export XModel**, selecting the full hierarchy (**`Select > Hierarchy`**, not just the root/`tag_origin` — selecting only the root joint looks complete but is the most common "export fails silently" cause).
5. **Convert** the exported `xmodel_export` from step 4 to `.xmodel_bin` with `export2bin.exe`/`exportxbin.exe` (shipped in `bin/`) or ExportX. This Maya-produced `xmodel_export` is also what bridges a Cast/SEModel rip (e.g. Saluki) into a format the converter accepts.
6. **Compile in APE**: new GDT (save it under `<bo3_root>/source_data/` — a GDT saved elsewhere silently won't reappear in APE/Radiant next session), `xmodel` asset, type **animated**, `BulletCollisionLOD` = LOD0, submodels parented to the main model's root tag.

**ADS export tags depend on the source game** — a common silent-fail point: from a Treyarch-source rig, export only `tag_view`+`tag_torso` for ADS; from an IW-source rig, export only `tag_view`+`tag_ads`. Grabbing the wrong pair for the source you ripped from is a frequent cause of broken ADS.

**Naming convention** (Treyarch's own): `<game_id>_<usage>_<class>_<name>_<type>`, e.g. `t6_attach_mag_dsr50_view` — keep ported assets consistent with this so your GDT stays navigable.

**Materials**: BO3 is PBR — diffuse/albedo, ambient occlusion, normal, specular, gloss. From an older CoD, the color map (`_c`) maps to diffuse and the environment map (`_e`/`_env`) maps to gloss. Weapon materials: `Material Category` = Geometry, `Material Type` = `lit_weapon`. Two easy-to-hit pitfalls: leaving **Surface Type** at `<error>`/`error` (use `<none>` instead), and non-power-of-2 image dimensions (BO3 requires power-of-2 textures). Set one material's fields, then duplicate + rename for the rest (same for xanim assets).

**Animations**: convert with the community `conversion_rig.ma` (root bone becomes a child of `t7:tag_weapon_right`, rename joints with `renameRig.mel`, import the anim, strip the rig with `removeNamespace.mel`, rename the root back to `tag_weapon`). Two silent failures there: `renameRig.mel` suffixes joints `_t7`, and any that `removeNamespace.mel` misses export bound to nothing — **grep the `.xanim_export` for `_t7` before converting**; and zero the gun root's transforms after parenting it under `tag_weapon_right`, or the gun sits offset from the hands (ModMe guide, t7kb 0.70). **Cold War's animation filenames are dehashed** — pull weapon anims from Modern Warfare instead, it works more reliably. Compile in APE as an `xanim` asset — the settings differ by animation kind: **viewmodel anims** use Use Bones unchecked / `Type` = `relative`; **everything else (world/character anims)** uses Use Bones checked / `Type` = `delta` — mixing them up gives an anim right on the weapon but broken on the world model, or vice versa. Check **Looping** for loop anims regardless of type. The full DCC→`.xanim_export`→`export2bin`→APE pipeline and its traps are in **t7kb:animation**.

Finish: `bulletweapon` asset in APE, add to the map's zone file (`weapon,<name>`), drag it in from Radiant's entity browser.

**Dual-wield: the left hand is a `dualwieldweapon` asset, not a second `bulletweapon`.** `dwlefthand` exists only in the dual-wield asset types (`dualwieldweapon`, `dualwieldprojectileweapon`) — `bulletweapon`'s inventory types stop at `primary | offhand | item | altmode | gadget | hero` — a guide saying "set Inventory to dwlefthand" is impossible on the single-weapon asset. The pair (from decompiled stock `pistol_standard`): the right hand a `bulletweapon` with `dualWield 1`, `playerAnimType dualwield`, `DualWieldWeapon <lh>`; the left a `dualwieldweapon` with `inventoryType dwlefthand`, `DualWieldWeapon <rh>`. Rename the left gun's tags (`j_gun1→tag_weapon_le`, `tag_flash1→tag_flash_le`, `tag_brass1→tag_brass_le`, ModMe 0.70) in the exported model and each `.xanim_export` — never on the conversion rig before anim import, whose tracks are keyed to the old names.

## Custom/ported characters and zombies

Same shape as weapons, plus rigging: extract the **factory zombie/character rig** (Greyhound), import your mesh, bind joints and weight-paint. **Split the head from the body and rig it to the default BO3 head separately** if you need jaw movement (a community `binder.mel` script automates the head-to-body attachment). Alternative: import a stock BO3 character alongside yours, copy its weight paint, swap the armature onto your model and delete the stock one (needs a matching joint hierarchy).

For wiring a rigged zombie into custom AI behavior (archetypes, spawners) rather than the modeling/rigging itself, see **t7kb:zombies-ai**.

Swapping the **stock Zombies player model** is a separate, simpler task from custom-character porting: swap to another stock crew member via `zm_usermap.gsc`'s character-index override, or build a fully custom player model via `customizationtable`/`playerbodytype`/`playerbodystyle` duplication in APE (body `multiplayer body`, arms `viewhands`, legs `animated`). **From a usermap, your copied `customizationtable` is silently skipped** — stock `core_common.csv` contributes `customizationtable,zm_character_customization`, the same upstream-asset skip **t7kb:debugging** explains; comment that line out.

## Collision: ported/custom models have zero collision by default

A ported or custom xmodel has **no collision unless you give it one** — players/zombies walk through it. Fix via the xmodel's `CollisionMap` field pointing at a `.map` under `share/raw/collmaps/`, textured with `clip_physics`; the collmap must be **brush-only** (patches don't produce valid collision). `scaleCollMap` applies the asset's own conversion `scale` to the collmap (`xmodel.awi`) — not Radiant's `modelscale`.

**For a placed `misc_model`, the collmap is opt-in and baked by the map compiler.** `use_collmap`, `no_collmap` and worldspawn `use_misc_models_collmaps` all default off (`t7.def.json`: *"if set, cod2map will bake the misc model collmaps in the bsp"*). Symptom: `CollisionMap` set, relinked, players still walk through — the bake is a **Compile** step, so set the key *and recompile*. A `script_model` is reported to pick up its collmap without the flag, which is why converting to one "fixes" it (community).

## Script bundles (the asset-pipeline side of data-driven content)

Scene/cinematic, vehicle, killstreak and collectible data lives in **script bundles** — GDT-authored assets read at runtime via `struct::get_script_bundle`, `get_script_bundle_list` and `get_script_bundle_instances` (verified against shipped `struct.gsc`). Use one instead of a hardcoded GSC table when the data is really asset content.

## Hand-authoring a GDT entry: copy a working one, then scrub what you didn't mean to inherit

**Over MCP, use the `gdt_*` tools instead of editing the text:** `t7kb:gdt_find` / `t7kb:gdt_get` to locate and read an asset, `t7kb:gdt_schema` for what APE declares for a type (and a material type's real texture slots), `t7kb:gdt_edit` to create or change assets (validated, dry run by default, several at once, images straight from textures), `t7kb:gdt_check` to diagnose a whole GDT before building, and `t7kb:gdt_refs` before renaming or deleting an asset. What each one checks, and how to make a material from your own textures in one call: **`references/gdt-tools.md`**. The traps below are what they guard against, and still apply by hand. `gdt_edit` keeps one `.bak` per GDT, not a history: in a git-tracked root each write is a one-asset diff to commit on its own. **If the `t7kb` server lists `search`/`get` but no `gdt_*` tool, its binary predates them (they ship from 2.1.0):** tell the user and offer `/t7kb:setup` to update it, rather than silently editing the GDT text instead.

A GDT is plain text, so entries can be written directly (and versioned). **Copy an existing entry of the same asset type and substitute** — an xmodel entry carries ~70 fields and a `zbarrier` ~140, and hand-listing invites a missing-field failure. But a copied entry drags the donor's asset paths along, and the errors point at the *donor* (verified porting a BO2 vehicle):

- **Backslashes in `filename` must be doubled.** `"filename" "folder\\model\\model_lod0.xmodel_bin"` — single backslashes are eaten as escape sequences and the linker reports the path with separators gone (`folderModelmodel_lod0...`).
- **Scrub the donor's LOD fields.** `mediumLod` / `lowLod` / `lowestLod` still point at the *donor's* meshes, so your model links with another model's lower LODs. Symptom: `Part 'tag_animate' in lower lod '<donor>' doesn't have the same name as part 'tag_body' in higher lod '<yours>'`. Blank every LOD path you don't actually supply.
- **`BulletCollisionLOD` must name a LOD you have.** Inherited `Low` on a LOD0-only model gives `Lod 'Low' does not exist in model '<name>', but it is set as the bullet collision`. Same for `ShadowLOD`.
- **Animated props often fail the LOD bounds check.** `XModel '<name>' failed. Bounding box of all LODs 2.17 times base mesh` on ripped `*_anim_*` props — their exported bounds don't match what the linker expects. Substituting an already-ported equivalent unblocks the build while you sort the source model out.

## `export2bin` resolves its argument against its own working directory

`export2bin.exe path/to/model.xmodel_export` fails with `ERROR: Failed to read file .\model.xmodel_export` — it ignores the directory you gave it. **Run it with the working directory set to the model's own folder** and pass the bare filename. It writes `.XMODEL_BIN` in caps; rename to lowercase to match the `model_export/` convention. 

## After editing a GDT, index it from `gdtdb`'s own folder — and know what "everything missing" means

A GDT written outside APE is picked up by the normal pass (verified): `gdtdb /update`, run from the `gdtdb/` folder, reports `processed (1 GDTs) (1 assets)`. So a plain rebuild (the build tool's default `gdtdb /update`) is the first thing to try.

The expensive failure: the linker reports **every asset in the game as missing** — `skybox_default_day`, `luts_t7_default`, things you never touched — while `gdtdb /update` prints `processed (0 GDTs) (0 assets)`. It looks like a corrupted database; recover with a full re-index:

```
gdtdb.exe /rebuild        # processed (3004 GDTs) (260203 assets) — ~60-90s
```

(the build tool's `gdt_rebuild=true`). The likely cause is **where** `gdtdb` ran rather than the hand edit: it records asset paths relative to its working directory, so run it from `gdtdb/` exactly as the Launcher does (**t7kb:compiling**) — and under git-bash, MSYS rewrites `/rebuild`/`/update` into filesystem paths so the tool silently prints its usage; prefix with `MSYS2_ARG_CONV_EXCL="*"` or use PowerShell.

## A material is built through what uses it — never zone it on its own

(Verified on a real link.) A new `lit` material added to a zone as a bare `material,<name>` line fails with `error X1507: failed to open source file: 'techsetdef_buildshadowmap.hlsl'` → `lit#da7372ff.build shadowmap depth: GetDrawMethod(build shadowmap depth) Failed` → `One or more shaders failed to compile`. It looks like a broken install or GDT; it is neither.

The linker never compiles Treyarch's shaders from source (the mod tools don't ship it): it looks each techset variant up in the compiled cache under `share/assetconvert/shaders/pc/v7`. A material used by an **xmodel** is built as `mc/<name>` and one used by **map geometry** as `wc/<name>` (the prefixes a techset lists in `availablePrefixes`) — those variants are cached. A bare zone line asks for the unprefixed variant, which nothing ships precompiled, so the linker tries to compile it and fails. Your `<map>.csv` shows the real name: `techset,mc/lit_alphatest_nocull#e6142445` under `mc/<your_material>`.

So zone the **model** (or place the material on brushes) and let it pull the material in; don't add `material,<name>` for a model material. (Zoning `material,mc/<name>` directly hangs the linker with no output.)

## Material settings a rip gets wrong, and how to tell

Where a rip stops looking like the original (verified porting a BO2 vehicle):

- **`normalHeightScale` — turn it down to ~0.1-0.2.** An older-title normal map reads far too strong in BO3 and produces **hard lighting bands, panel by panel**, that look like a UV, blend-mask or atlas problem. Diagnose by blanking `normalMap` for one material and rebuilding: if the banding vanishes, it's intensity, not the map.
- **`materialType` and `materialCategory` must agree**, or APE warns: `lit`→`Geometry`, `lit_plus`→`Geometry Plus`, `lit_advanced*`→`Geometry Advanced`, `lit_decal`→`Decal`. A copied donor entry usually carries the wrong pairing.
- **`lit_advanced_fullspec` silently refuses to expose `colorMap`** if `aoMap` and `glossMap` are missing — which a rip never has. Error reads `material '<name>' using technique '...' doesn't expose a 'colorMap' texture`. Drop to `lit`.
- **The `colorMap` slot needs `coreSemantic` `sRGB3chAlpha`.** Switching a diffuse to `sRGB3ch` to dodge a packed alpha breaks the binding entirely, with the same "doesn't expose a colorMap" error.
- **`baseImage` is relative to the install root**, so it includes the `texture_assets\\` prefix — not relative to `texture_assets/` itself.
- **Old-title `SurfaceType` values don't transliterate.** `PAINTED_METAL` → `paintedmetal` (no underscore), and `default` → `<none>`, else the linker aborts with `surfaceTypeName 'default' not in surfaceTypeParms array`. `t7kb:gdt_edit` and `t7kb:gdt_check` reject a value that isn't in APE's list and warn on one left on `<error>`, APE's default, which stops the linker the same way once the material reaches collision.

## Emissive, double-sided and packed alpha each look like a different bug

- **Emissive goes in `colorMap00`** of a `lit_emissive*` material type — a name that says nothing about emission, so searching for slots by name concludes, wrongly, that BO3 has none; an older title's `_e` map ports straight across.
- **Double-sided is a `nocull` material type** (`lit_nocull`, …), not `doubleSidedLighting`, which only changes how backfaces are lit.
- **An alpha channel's percentage tells you nothing** — a packed gloss mask and a real cutout measure the same; alpha-testing the mask punches black speckles. Map the alpha spatially instead.

Evidence, the slot names around them (`cosinePowerMap`, `occMap`), and how to check each: **`references/material-traps.md`**.

## A failed link isn't always a failed link

A `^3Found N bad bulletmeshes` warning makes the linker exit non-zero on a good build (**t7kb:compiling** owns the verdict). Asset-side: `zone_source/all/assetinfo/<map>_bulletreport.csv` names the offending models with their triangle counts and average face area (a ripped vehicle at 25k collision tris trips it) — worth a proper collmap, not a build blocker.

## Common pitfalls

- **Blender's current Blender-COD plugin (the GitHub one) can break UV export.** If ported textures look wrong/shifted after export, community consensus is to fall back to a legacy release rather than debug the current one.
- **Remapping every material to a placeholder gets a rip linking, and then you forget.** Rewriting the `MATERIAL n "name"` lines in an `.xmodel_export` to one existing material shows geometry in-game early, but the model then *is* that texture and later looks like an asset bug. Note it, or use the xmodel's `skinOverride` field ("enter the name of the new material next to the one you want replaced", `xmodel.awi`) to swap materials without touching the export (field verified; untested in practice).
- **Duplicate GDT asset errors** (`Duplicate 'material' asset '<name>' found in ...gdt:<line>`) mean the same asset name exists in two GDTs (yours and a shared/stock one) — delete your duplicate entry, don't rename around it; it's a naming collision, not a corruption. **GDTDupePurger** clears these in bulk.
- **Ragdoll behavior for a custom model** goes through `RagdollSettings` — a dragged-in stock ragdoll setup silently keeps stock proportions/behavior unless you edit it for your model.

## Don't invent

Community export/rigging workflows have version-specific gotchas (plugin build, Maya version, rig) — verify the tool version against the corpus and raw install before asserting a fix still applies.
