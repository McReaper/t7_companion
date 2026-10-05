// Package deps is the asset dependency graph: what an asset pulls into a
// fastfile, following the references the linker follows. Graph.Children
// answers for one asset, Graph.Closure for everything some roots (a zone's
// lines, ZoneRoots) pull in, each asset with the one that first pulled it.
//
// The edges, by what the asset is:
//
//   - a GDT asset: its typed fields (Workspace.Refs: AssetCombo targets, an
//     item list's entries up to its count), packed as the linker's type
//     (Workspace.LinkerType); a target the GDTs define only as another type
//     than the deffile declares is that type (refID). Fields naming files: an
//     .efx is an fx, an AI table (.ai_am…) is packed under its file name, a
//     zone package (csvInclude) adds its lines.
//   - a material: one image per texture slot of its techset — the field's or
//     the slot's default ($white…) — not every image field (material.go).
//   - an xmodel: the materials its LOD and collision files use, with its
//     skinOverride's replacements instead of the originals.
//   - a camo table: its enabled camo sets inline, the sets not packed (camo.go).
//   - an animation mapping table: its xanims (animtable.go).
//   - an fx: what its .efx elements name (efx.go).
//
// Raw files (animtables/, fx/…) are read from the map's folder first, then
// share/raw, as the linker does (New's mapDir).
//
// The linker descends into a stock asset an ignore list provides as it does
// into any other: whether an asset is stock changes what it costs (a reference
// to the shipped asset), not what it pulls in. Some nodes are not packed as
// assets of their own (Packed).
//
// TestGraphMatchesLinker (T7KB_ORACLE=1) scores the graph against what the
// linker packed for a linked map, edge family by edge family; what the map's
// BSP pulls in (placed models, entities) is not in the graph yet.
package deps
