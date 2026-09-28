---
name: assets
description: How to port a model, material/texture, or rig into Black Ops 3 and hand-author or compile its GDT — picking a ripper per source game (Saluki, Greyhound, Kobra, Cordycep), bridging `_export`/`_bin`, APE material fields (`normalHeightScale`, `colorMap00` emissive, `nocull`, `SurfaceType`), and collision (`CollisionMap`, `scaleCollMap`). Use when a rip's normal map bands per-panel, an emissive part won't glow, a ported model has zero collision, a hand-edited GDT still reports assets missing (`gdtdb /rebuild` vs `/update`), APE throws `doesn't expose a 'colorMap' texture` / `Duplicate 'material' asset` / a LOD bounding-box mismatch, or `exportxbin` aborts with `Failed to decompress binary file`. Distinct from t7kb:animation (the DCC→`.xanim_bin` compile pipeline) and t7kb:anim-retarget (cross-generation Maya HumanIK retargeting) — both assume this pipeline already got the model/rig in. Also distinct from t7kb:crossref (another title's script dumps as reference, not the port itself).
---

# Assets: models, materials, and porting

**A port that looks right in APE can still be wrong in places APE doesn't show** — a donor's leftover fields, a material slot named nothing like its job, collision that was never baked. This skill is the pipeline and those traps; look up exact APE fields, GDT syntax, and material settings in **t7kb** (`t7kb:search` then `t7kb:get`), weighing `reliability` per hit (Discord ~0.25, UGX/ModMe/T7-wiki ~0.70, schema-verified references ~0.90). Ground on the APE schemas in `deffiles/*.awi` and stock GDTs dated with the install. For the *source* game — its engine lineage and script dumps — see **t7kb:crossref**.

## Extraction tools aren't interchangeable — pick by source game and by the output you need

- **Rip** with **Saluki** (the current default, every PC CoD, emits Cast/SEModel) — or **Kobra** when you also need the old auto-GDT or **`.efx`** (Greyhound dropped both), **Cordycep** to unpack Ricochet-era titles for a ripper, **Legion** for Apex `.rpak`.
- **Bridge to `_bin`**: the shipped `export2bin.exe`/`exportxbin.exe` read only the text `*_export` format, so a Cast rip goes through Maya/Blender first (re-export as `xmodel_export`); **ExportX** converts both ways from the command line (`exportx.exe -f <file>.xmodel_bin -m export` unpacks a loose bin to text with its skeleton).
- **GDTs**: a *content* GDT (xmodel/material/image) is APE, MakeCents or Spiki; a *config* GDT (behavior trees, weaponfiles, aliases, script bundles) is decompiled by **HydraX**.

Roles, alternatives, whole-map exporters (Husky, C2M), GDT helpers (NevisX, GDTDupePurger) and their version traps: **`references/extraction-tools.md`**.

## The weapon porting pipeline (also the template for character/prop ports)

Order matters here — skipping ahead (e.g. exporting before attaching joints) is the usual cause of a silently broken port:

1. **Obtain** the source assets (Saluki/Greyhound/Cordycep/Legion — pick per source game, see above). Rip **every** piece — scope, magazine, and body are separate models.
2. **Prepare in Maya.** Open the `.ma` via `File > Open`, never drag-and-drop — dragging merges it into the scene and namespaces everything, causing problems later.
3. **Attach imported joints** to the main model's root (`j_gun`/`tag_weapon`).
4. **Export** via **Call of Duty Tools → Export XModel**, selecting the full hierarchy (**`Select > Hierarchy`**, not just the root/`tag_origin` — clicking only the root joint looks like it selected everything but doesn't, and is the single most common "export fails silently" cause).
5. **Convert** the exported `xmodel_export` from step 4 to `.xmodel_bin` with `export2bin.exe`/`exportxbin.exe` (shipped in `bin/`) or ExportX. This Maya-produced `xmodel_export` is also what bridges a Cast/SEModel rip (e.g. Saluki) into a format the converter accepts.
6. **Compile in APE**: new GDT (save it under `<bo3_root>/source_data/` — a GDT saved elsewhere silently won't reappear in APE/Radiant next session), `xmodel` asset, type **animated**, `BulletCollisionLOD` = LOD0, submodels parented to the main model's root tag.

**ADS export tags depend on the source game** — a common silent-fail point: from a Treyarch-source rig, export only `tag_view`+`tag_torso` for ADS; from an IW-source rig, export only `tag_view`+`tag_ads`. Grabbing the wrong pair for the source you ripped from is a frequent cause of broken ADS.

**Naming convention** (Treyarch's own): `<game_id>_<usage>_<class>_<name>_<type>`, e.g. `t6_attach_mag_dsr50_view` — keep ported assets consistent with this so your GDT stays navigable.

**Materials**: BO3 is PBR — diffuse/albedo, ambient occlusion, normal, specular, gloss. From an older CoD, the color map (`_c`) maps to diffuse and the environment map (`_e`/`_env`) maps to gloss. Weapon materials: `Material Category` = Geometry, `Material Type` = `lit_weapon`. Two easy-to-hit pitfalls: leaving **Surface Type** at `<error>`/`error` (use `<none>` instead), and non-power-of-2 image dimensions (BO3 requires power-of-2 textures). Set one material's fields, then duplicate + rename for the rest rather than re-entering settings each time — same trick works for xanim assets.

**Animations**: convert with the community `conversion_rig.ma` (root bone becomes a child of `t7:tag_weapon_right`, rename joints with `renameRig.mel`, import the anim, strip the rig with `removeNamespace.mel`, rename the root back to `tag_weapon`). Two silent failures there: `renameRig.mel` suffixes joints `_t7`, and any that `removeNamespace.mel` misses export bound to nothing — **grep the `.xanim_export` for `_t7` before converting**; and zero the gun root's transforms after parenting it under `tag_weapon_right`, or the gun sits offset from the hands (ModMe guide, t7kb 0.70). **Cold War's animation filenames are dehashed** — pull weapon anims from Modern Warfare instead, it works more reliably. Compile in APE as an `xanim` asset — the settings differ by animation kind: **viewmodel anims** use Use Bones unchecked / `Type` = `relative`; **everything else (world/character anims)** uses Use Bones checked / `Type` = `delta` — mixing these up is a common cause of a ported anim looking right on the weapon but broken on the world model, or vice versa. Check **Looping** for idle/sprint-loop/slide-loop/swim anims regardless of type. For the full DCC→`.xanim_export`→`export2bin`→APE pipeline and its many silent-failure traps (the Quality field's frame-doubling, the export2bin single-argument mode, notetracks that must be added in APE not Maya, the required Model File), see **t7kb:animation**.

Finish: `bulletweapon` asset in APE, add to the map's zone file (`weapon,<name>`), drag it in from Radiant's entity browser.

**Dual-wield: the left hand is a `dualwieldweapon` asset, not a second `bulletweapon`.** `dwlefthand` exists only in the dual-wield asset types (`dualwieldweapon`, `dualwieldprojectileweapon`) — `bulletweapon`'s inventory types stop at `primary | offhand | item | altmode | gadget | hero` — so a guide telling you to "set Inventory to dwlefthand" is impossible on the asset the single-weapon pipeline creates. The pair (from decompiled stock `pistol_standard`): the right hand a `bulletweapon` with `dualWield 1`, `playerAnimType dualwield`, `DualWieldWeapon <lh>`; the left a `dualwieldweapon` with `inventoryType dwlefthand`, `DualWieldWeapon <rh>`. Rename the left gun's tags (`j_gun1→tag_weapon_le`, `tag_flash1→tag_flash_le`, `tag_brass1→tag_brass_le`, ModMe 0.70) in the exported model and each `.xanim_export` — never on the conversion rig before anim import, whose tracks are keyed to the old names.

## Custom/ported characters and zombies

Same shape as weapons, plus rigging: extract the **factory zombie/character rig** (Greyhound), import your mesh in Maya/Blender, bind joints to the mesh, and weight-paint. **Split the head from the body and rig it to the default BO3 head separately** if you need jaw movement (a community `binder.mel` script automates the head-to-body attachment). An alternative some modders use: import a stock BO3 character alongside yours, copy its weight paint, then swap the armature onto your model and delete the stock one — works as long as the joint hierarchy matches.

For wiring a rigged zombie into custom AI behavior (archetypes, spawners) rather than the modeling/rigging itself, see **t7kb:zombies-ai**.

Swapping the **stock Zombies player model** is a separate, simpler task from custom-character porting: swap to another stock crew member via `zm_usermap.gsc`'s character-index override, or build a fully custom player model via `customizationtable`/`playerbodytype`/`playerbodystyle` duplication in APE (body `multiplayer body`, arms `viewhands`, legs `animated`). **From a usermap, your copied `customizationtable` is silently skipped** — stock `core_common.csv` contributes `customizationtable,zm_character_customization`, the same upstream-asset skip **t7kb:debugging** explains; comment that line out. That skip is where the "custom player models only work in a mod" folklore comes from.

## Collision: ported/custom models have zero collision by default

Unlike stock assets, a ported or custom xmodel has **no collision unless you give it one** — a bare model will let players/zombies walk straight through it. Fix via the xmodel's `CollisionMap` field pointing at a `.map` under `share/raw/collmaps/`, textured with `clip_physics`; the collmap must be **brush-only** (patches don't produce valid collision). `scaleCollMap` applies the asset's own conversion `scale` to the collmap (`xmodel.awi`) — not Radiant's `modelscale`.

**For a placed `misc_model`, the collmap is opt-in and baked by the map compiler.** `use_collmap`, `no_collmap` and worldspawn `use_misc_models_collmaps` all default off (`t7.def.json`: *"if set, cod2map will bake the misc model collmaps in the bsp"*). Symptom: `CollisionMap` is set, you relink, players still walk through it — the bake is a **Compile** step, so set the key *and recompile*; relinking does nothing. A `script_model` is reported to pick up its collmap without the flag, which is why converting to one "fixes" it (community). Easy to miss: the model looks fine right up until something needs to stand on it.

## Script bundles (the asset-pipeline side of data-driven content)

Scene/cinematic, vehicle, killstreak, and collectible data lives in **script bundles** — GDT-authored, data-driven assets read at runtime via `struct::get_script_bundle`, `get_script_bundle_list`, and `get_script_bundle_instances` (verified against shipped `struct.gsc`). Reach for a script bundle instead of hardcoding a table in GSC when the data is really asset content (e.g. a set of placeable collectible variants) — it keeps the data in APE/GDT where the rest of the pipeline expects it.

## Hand-authoring a GDT entry: copy a working one, then scrub what you didn't mean to inherit

A GDT is plain text, so you can write entries directly instead of clicking through APE — same result, and it versions. The reliable method is to **copy an existing entry of the same asset type and substitute**, because an xmodel entry alone carries ~70 fields and a `zbarrier` ~140; hand-listing them invites a missing-field failure. But a copied entry drags the donor's asset paths with it, and the resulting errors point at the *donor*, which is confusing until you know to look. All four of these were hit in one sitting porting a BO2 vehicle, in this order:

- **Backslashes in `filename` must be doubled.** `"filename" "folder\\model\\model_lod0.xmodel_bin"` — with single backslashes the GDT parser eats them as escape sequences and the linker reports a path with the separators simply gone (`folderModelmodel_lod0...`), which reads like a string-concatenation bug rather than an escaping one.
- **Scrub the donor's LOD fields.** `mediumLod` / `lowLod` / `lowestLod` still point at the *donor's* meshes, so your model links with another model's lower LODs. Symptom: `Part 'tag_animate' in lower lod '<donor>' doesn't have the same name as part 'tag_body' in higher lod '<yours>'`. Blank every LOD path you don't actually supply.
- **`BulletCollisionLOD` must name a LOD you have.** Inherited `Low` on a LOD0-only model gives `Lod 'Low' does not exist in model '<name>', but it is set as the bullet collision`. Same for `ShadowLOD`.
- **Animated props often fail the LOD bounds check.** `XModel '<name>' failed. Bounding box of all LODs 2.17 times base mesh` on ripped `*_anim_*` props — their exported bounds don't match what the linker expects. Substituting an already-ported equivalent unblocks the build while you sort the source model out.

## `export2bin` resolves its argument against its own working directory

`export2bin.exe path/to/model.xmodel_export` fails with `ERROR: Failed to read file .\model.xmodel_export` — note the `.\`. It ignores the directory you gave it. **Run it with the working directory set to the model's own folder** and pass the bare filename. It writes `.XMODEL_BIN` in caps; rename to lowercase to match the `model_export/` convention. (Same class of cwd sensitivity as `cod2map64` needing to run from `bin/` — see **t7kb:compiling**.)

## After hand-editing a GDT, `gdtdb /update` does not see it — you need `/rebuild`

The most expensive trap in this whole area, because the symptom is wildly misleading. Edit a GDT by hand, run the normal pipeline, and the linker reports **every asset in the game as missing** — `skybox_default_day`, `luts_t7_default`, the stock zombie spawner, things you never touched — while `gdtdb.exe /update` cheerfully prints `processed (0 GDTs) (0 assets)`. It looks like you corrupted the database. You didn't: the incremental pass just doesn't notice hand-written files.

```
gdtdb.exe /rebuild        # processed (3004 GDTs) (260203 assets)
```

Budget ~60-90s and run it after **every** manual GDT edit. And under git-bash, MSYS rewrites `/rebuild` into a filesystem path so the tool silently prints its usage instead of running — prefix with `MSYS2_ARG_CONV_EXCL="*"` (same MSYS argument-mangling as the `/update` and `+medium` flags in **t7kb:compiling**).

## Material settings a rip gets wrong, and how to tell

Porting a model's *materials* is where a rip stops looking like the original. These are the ones that bite, all verified porting a BO2 vehicle:

- **`normalHeightScale` — turn it down to ~0.1-0.2.** An older-title normal map reads far too strong in BO3 and produces **hard lighting bands, panel by panel**, that look for all the world like a UV, blend-mask or texture-atlas problem. Diagnose it by blanking `normalMap` for one material and rebuilding: if the banding vanishes, it's intensity, not the map.
- **`materialType` and `materialCategory` must agree**, or APE warns: `lit`→`Geometry`, `lit_plus`→`Geometry Plus`, `lit_advanced*`→`Geometry Advanced`, `lit_decal`→`Decal`. A copied donor entry usually carries the wrong pairing.
- **`lit_advanced_fullspec` silently refuses to expose `colorMap`** if `aoMap` and `glossMap` are missing — which a rip never has. Error reads `material '<name>' using technique '...' doesn't expose a 'colorMap' texture`. Drop to `lit`.
- **The `colorMap` slot needs `coreSemantic` `sRGB3chAlpha`.** Switching a diffuse to `sRGB3ch` to dodge a packed alpha breaks the binding entirely, with the same "doesn't expose a colorMap" error.
- **`baseImage` is relative to the install root**, so it includes the `texture_assets\\` prefix — not relative to `texture_assets/` itself.
- **Old-title `SurfaceType` values don't transliterate.** `PAINTED_METAL` → `paintedmetal` (no underscore), and `default` → `<none>`, else the linker aborts with `surfaceTypeName 'default' not in surfaceTypeParms array`.

## Emissive lives in `colorMap00`, and the slot names are traps

A glowing part (screens, eyes, indicator lights) needs a **`lit_emissive*`** material type — there are ~59 of them, so pick the one that also carries whatever else the material needs (`lit_emissive_plus` when you also want gloss, `lit_emissive_advanced_fullspec`, `lit_emissive_transparent`, …).

The emissive map goes in **`colorMap00`**. Nothing in that name says "emissive", and it does **not** turn up if you look for slots by grepping field names ending in *map* — which is how you end up concluding, wrongly, that BO3 has no emissive slot and that the emission has to be composited into the diffuse alpha. **870 of 1362** shipped emissive materials fill it: `mtl_char_ger_zombie_eyes` sets `colorMap00 = zombie_eye`, `mtl_p7_pro_monitor_control_tower` sets it to an `_e` texture. So an older title's separate `_e` map ports **straight across**, no channel packing.

Its image asset is `sRGB3chAlpha` / `diffuseMap` like a diffuse, but **`compressionMethod` = `compressed no alpha`**.

Two neighbours named no better: **gloss is `cosinePowerMap`** (image side `Linear1ch` / `glossMap`; only the `_plus` / `_advanced` categories expose it) and **AO is `occMap`**.

`emissiveFalloff` and `emissiveIncompetence` (APE labels the latter *gameplay intensity*) tune the result — and do **not** copy them from a donor. Across the shipped emissive materials `emissiveIncompetence` splits **681 / 680** between `0` and `1`: a genuine per-material choice, not a default with outliers. Same discipline as `SurfaceType` above — check the *distribution* before copying a value, because where it is 50/50 the donor tells you nothing.

## Double-sided: `nocull` is a material type, not a flag

A model whose backfaces don't render is fixed by a **`nocull` material type** — `lit_nocull`, `lit_alphatest_nocull`, `lit_transparent_nocull`, `lit_detail_nocull` and `_advanced`/`_plus` variants all ship. `doubleSidedLighting` is *not* it: that controls how backfaces are **lit**, not whether they're drawn, and setting it changes nothing visible.

Worth checking the geometry first so you know which problem you have: if every face of the material has a unique position triple (no duplicated triangles with reversed winding), the mesh is genuinely single-sided and only `nocull` can save it. Duplicating the faces in the export also works but doubles the triangles and is not reversible from the GDT.

## An alpha channel's *percentage* tells you nothing — its distribution does

Ripped diffuse maps frequently carry a packed gloss/spec mask in alpha, and a real alpha cutout looks identical if you only measure "what fraction of pixels are non-opaque". Alpha-testing a packed mask punches **black speckles** through the surface, which reads as a corrupt texture.

Map the alpha spatially instead — a coarse grid of "percent of pixels below threshold" per cell. A genuine cutout is a **compact, sharp-edged region**; a packed mask is scattered noise across the whole sheet. One bus material showed a solid rectangular transparent block over half the texture (real vents), another only 2-3% scattered (gloss mask), and the naming confirmed both — Treyarch shipped a `_opq` twin of the vented material.

## A failed link isn't always a failed link

A `^3Found N bad bulletmeshes` warning makes the linker exit non-zero on a perfectly good build — **t7kb:compiling** owns reading the real verdict. What's asset-side: `zone_source/all/assetinfo/<map>_bulletreport.csv` names the offending models with their triangle counts and average face area (a ripped vehicle at 25k collision tris trips it) — a real quality warning worth fixing with a proper collmap, not a build blocker.

## Common pitfalls

- **Blender's current Blender-COD plugin (the GitHub one) can break UV export.** If ported textures look wrong/shifted after export, community consensus is to fall back to a legacy release rather than debug the current one.
- **Remapping every material to a placeholder gets a rip linking, and then you forget.** Rewriting the `MATERIAL n "name"` lines in an `.xmodel_export` to one existing material is a legitimate way to see geometry in-game before the material port is done — but the model now *is* that texture, and it looks like an asset bug later. Note it, or use the xmodel's `skinOverride` field ("enter the name of the new material next to the one you want replaced", `xmodel.awi`) to swap materials without touching the export (field verified; untested in practice).
- **Duplicate GDT asset errors** (`Duplicate 'material' asset '<name>' found in ...gdt:<line>`) mean the same asset name exists in two GDTs (yours and a shared/stock one) — delete your duplicate entry, don't rename around it; it's a naming collision, not a corruption. **GDTDupePurger** clears these in bulk.
- **Ragdoll behavior for a custom model** goes through `RagdollSettings` — a dragged-in stock ragdoll setup silently keeps stock proportions/behavior unless you edit it for your model.

## Don't invent

Community export/rigging workflows here have real version-specific gotchas (a plugin build, a Maya version, a specific rig) that change over time — verify the current tool/plugin version against what the corpus and raw install actually show before asserting a fix still applies, rather than assuming yesterday's Discord answer is timeless.
