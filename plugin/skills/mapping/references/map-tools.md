Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — the traps of working in the mapper's Radiant through t7kb's map tools, beyond what each tool's description says. Read the parent skill first. Building from a plan is in `map-plans.md`, materials and views in `map-looks.md`.

Contents:

- [A new map's start room: build inside it, mind its floor and its contents](#a-new-maps-start-room-build-inside-it-mind-its-floor-and-its-contents)
- [One batch per step the mapper would undo](#one-batch-per-step-the-mapper-would-undo)
- [Undoing a deleted brush entity brings back world brushes](#undoing-a-deleted-brush-entity-brings-back-world-brushes)
- [Build one, then copy it, and rename the copies](#build-one-then-copy-it-and-rename-the-copies)
- [Zone every space players reach: volumes, spawners, then the script's links](#zone-every-space-players-reach-volumes-spawners-then-the-scripts-links)
- [Radiant's Patch menu reshapes what it is run on](#radiants-patch-menu-reshapes-what-it-is-run-on)
- [CSG Hollow walls are as thick as the grid](#csg-hollow-walls-are-as-thick-as-the-grid)
- [A dialog in Radiant stops the tools](#a-dialog-in-radiant-stops-the-tools)

## A new map's start room: build inside it, mind its floor and its contents

A map made from `ZM Mod Level` opens on the template's start room inside a sky box, with the start zone over it, the sun, umbra and fpstool volumes around it, the player spawns, the zombies' risers and the perk machines and box as prefabs (`t7kb:map_find` lists them; verified in the install's template). Inside the sky box and the start zone a new room seals and plays with nothing else moved. Three traps:

- **Its floor is one slab under the whole start zone**: a floor laid inside it flickers against it in game (verified in a test Radiant). Delete the slab, or rebuild it around the new floor, first.
- **Replacing the start room moves its contents too**: its spawners, player spawns and prefabs stand where its floor was, and zombies need a floor from their structs to the player, since the navmesh is compiled from floors. The start zone covers the whole room: shrink or delete its volume once the room moves, since a player in no zone dies (parent skill).
- **A map larger than the sky box** needs the six `sky` brushes rebuilt around it and the sun, umbra and fpstool volumes grown with it — delete and place them in one batch.

## One batch per step the mapper would undo

A batch is one entry in Radiant's undo menu, its `label` the name the mapper reads there ("t7kb: kitchen"). Make a batch of what the mapper would take back together — a room's floor, walls and ceiling, a corridor, the lights of one space — not one call per brush: each call is its own undo step and its own read of the map.

## Undoing a deleted brush entity brings back world brushes

Radiant's undo restores a deleted brush entity as plain world brushes (MTPlugins' notes): undoing a batch that deleted the template's `start_zone` brought back a world brush of `volume`, the `info_volume` gone (verified live). After such an undo, `t7kb:map_find` the volumes and rebuild the ones lost.

## Build one, then copy it, and rename the copies

A row of pillars, wall panels or lamps is one piece built and textured, then copied, in one batch. A copy keeps the original's keys, `targetname` included: rename the copies that must be told apart, and keep the name where several pieces are one thing (a zone's volumes share its name). Copy the entity, not one brush of it.

## Zone every space players reach: volumes, spawners, then the script's links

A zone is what the zone manager reads: every `info_volume` named for it, the riser spawners its first volume targets, and an `add_adjacent_zone` call in the map's script for each passage (verified in the install's `_zm_zonemgr.gsc`). Place each volume as a brush entity of class `info_volume` with the keys the template's start zone carries (`targetname` the zone's name, `target` its spawners', `script_noteworthy` `player_volume`), over the floor players reach: real zones aren't rectangles (Treyarch's `zm_giant` has 12 zones of 32 volumes, some angled). For the spawners, copy one of the template's `start_zone_spawners` structs onto the new floor, clear of walls, and set the copies' `targetname` to the new volume's `target`. Then write the `add_adjacent_zone` calls in the map's script, since the map tools write no script: `always_on` opens a link from the start, a door's flag when it is bought. Zombies walk the navmesh compiled from the floors (**t7kb:zombies-ai**).

## Radiant's Patch menu reshapes what it is run on

Run through `t7kb:map_command` on a placed brush or patch (verified in a test Radiant):

- `Patch/Primitives/…` replaces the selected brush with that primitive, stretched along z: shape the brush first.
- `Patch/Cap Texture` refits a patch's texture coordinates; a `Terrain Triangle` comes with `$default` and stretched coordinates until capped.
- `Square End Cap` gives the same shape as `Square Bevel`.

## CSG Hollow walls are as thick as the grid

`Selection/CSG/CSG Hollow` turns a box into six walls inside the box's bounds, as thick as the grid: 8 at grid 8, 16 at grid 16 (verified in a test Radiant). Set the grid (`Grid/Grid Sizes/Grid 16`) before and put it back after — it is the mapper's grid too. Hollow acts on the selection only, so it is safe beside volumes; its walls share one material, the box's.

## A dialog in Radiant stops the tools

A modal dialog open in Radiant blocks every edit until it closes: ask the mapper to close it. A menu entry that opens a dialog (most of those ending in `...`) waits on the mapper, so use the ones that act at once. A minimised Radiant draws nothing, so views need its window restored.
