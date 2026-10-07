// Package deps is the asset dependency graph: what an asset pulls into a
// fastfile, following the references the linker follows. Graph.Children
// answers for one asset, Graph.Closure for everything some roots pull in —
// a zone's lines (ZoneRoots) and its map source (MapRoots) — each asset with
// the one that first pulled it.
//
// The edges, by what the asset is:
//
//   - a GDT asset: its typed fields (Workspace.Refs: AssetCombo targets, an
//     item list's entries up to its count), packed as the linker's type
//     (Workspace.LinkerType); a target the GDTs define only as another type
//     than the deffile declares is that type (refID), and a field declared
//     with several types takes the one its value is defined as (variesChild).
//     Fields naming files: an effect is an fx (a path under share/raw may
//     drop the .efx), an AI table (.ai_am…) is packed under its file name, a
//     path under pc/main is an image, a zone package (csvInclude) adds its
//     lines. Not followed: fields only the editors read (editorOnly), the
//     parameters of a note whose action is None.
//   - a material: one image per texture slot of its techset — the field's,
//     the slot's default ($white…) or its fixed image — not every image field
//     (material.go).
//   - an xmodel: the materials its LOD files use (and its custom bullet mesh's,
//     when BulletCollisionLOD is Custom), with its skinOverride's replacements
//     instead of the originals.
//   - an xanim: the rumbles its notetracks play (format/xanimbin).
//   - a camo table: its enabled camo sets inline (camo.go); an attachment
//     cosmetic variant: the variants that have a model (acv.go); a weapon:
//     also the default cosmetic variant, which every weapon carries.
//   - an animation mapping table: its xanims; an fx: what its exported .efx
//     elements name; a lens flare: its images.
//
// A map source (MapRoots, mapsrc.go and maproots.go) pulls in what its BSP
// does: placed models and prefabs (read recursively, a prefab's worldspawn
// for its brushes only), entity classes (an actor_ spawner's aitype, a
// zbarrier, a glass's type, a physics dyn_model's preset) and keys
// (entityKeys, a lighting state's ssi), the materials brushes and patches
// draw (not Tools'), and what every compiled map packs (everyMap). Entities
// and brushes in a layer flagged ignore are not compiled; a script_struct's
// or a spawner's model is Radiant's preview.
//
// Raw files (animtables/, fx/, lensflares/…) are read from the map's folder
// first, then share/raw, as the linker does (New's mapDir).
//
// The linker descends into a stock asset an ignore list provides as it does
// into any other — it packs a reference to the shipped asset instead of the
// asset — except a script, which it leaves out (SetStock). Some nodes are not
// packed as assets of their own (Packed).
//
// Zone.Dangling calls an asset missing only for a type whose every source the
// graph reads (GDTs, effects, raw files): the linker links a lens flare with no
// .klf, from data the graph doesn't see, so a lens flare is never called one.
//
// TestGraphMatchesLinker (T7KB_ORACLE=1) scores the graph against what the
// linker loaded for a linked map, edge family by edge family, traces each miss
// and each extra to the edge that causes it, and fails if an asset Dangling
// lists was loaded with data.
package deps
