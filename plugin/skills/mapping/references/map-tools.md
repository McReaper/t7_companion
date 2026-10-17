Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — building in the user's running Radiant through t7kb's map tools. Read the parent skill first; come here for the sizes to build to, what a new map already holds, and the traps of each tool.

Contents:

- [Build to the official scale standards](#build-to-the-official-scale-standards)
- [A new map is the template's room in a sky box: build inside it](#a-new-map-is-the-templates-room-in-a-sky-box-build-inside-it)
- [One batch per step the user would undo](#one-batch-per-step-the-user-would-undo)
- [An edited world brush or brush entity comes back under a new id](#an-edited-world-brush-or-brush-entity-comes-back-under-a-new-id)
- [Cut doors by building around the hole, not with CSG Subtract](#cut-doors-by-building-around-the-hole-not-with-csg-subtract)
- [CSG Hollow walls are as thick as the grid](#csg-hollow-walls-are-as-thick-as-the-grid)
- [Look from the top first](#look-from-the-top-first)
- [Save before building: the build compiles the file, not Radiant's memory](#save-before-building-the-build-compiles-the-file-not-radiants-memory)
- [When the bridge says busy](#when-the-bridge-says-busy)

## Build to the official scale standards

One unit is one inch, and BO3's world runs at about 120% of real sizes. Treyarch's numbers, for a blockout's holes and volumes (official, `docs_modtools/Scale_Standards.pdf` in the install, t7kb 0.70):

| What | Size |
| --- | --- |
| Player (collision) | 72 high, 32 wide; crouched 52, prone 32 |
| Wall | 8 thick |
| Floor | 16 thick |
| Storey | 112 floor to ceiling, 128 floor to floor; a high one 144 and 160 |
| Door hole | 56 × 96; double 110 × 96 |
| Window hole | 56 × 64, sill at 32; the smallest a player climbs through 40 × 60 |
| Hallway | 128 wide without combat, 160 with; 96 the least clearance, for 128 at most |
| Stairs | 8 rise, 12 run, 80 wide at least |
| Jump | standing ~100 far, 39 high; sprinting ~220 far, 50 high |

So a room's floor is a 16-unit slab, its walls 8 thick and 112 high above it, its ceiling another slab: the boxes `t7kb:map_edit` takes are those numbers, on the 8-unit grid.

## A new map is the template's room in a sky box: build inside it

`t7kb:create`'s `ZM Mod Level` map already holds (verified in the install's template):

- a start room of `script_wall` walls and a `script_floor_ceiling` floor, the materials the stock template blocks out with (`caulk` draws nothing in game);
- a **sky box** of six `sky` brushes, 32 thick, around x and y −2048 to 2048 and z −64 to 1024;
- the **start zone**, an `info_volume` (`targetname` `start_zone`, `script_noteworthy` `player_volume`) over x and y ±1536, z 0 to 320, with its rising-zombie structs (`start_zone_spawners`) and the player spawns (`initial_spawn_points`, one `player_respawn_point`);
- a `volume_sun` (±1920, up to 896), an `umbra_volume`, a `volume_fpstool`, a `reflection_probe`, an `info_player_start`, the perk machines and box as `misc_prefab`s.

Build the blockout inside the sky box and the start zone first: it seals and plays with nothing else moved. A room outside the start zone is a new zone (the parent skill's zones section: an `info_volume`, its spawners, `add_adjacent_zone` in the map's GSC). A map larger than the box needs the six `sky` brushes rebuilt around it and the three volumes grown with it — there is no resize op: delete and add boxes in one batch.

## One batch per step the user would undo

`t7kb:map_edit` lands a whole batch as one entry in Radiant's undo menu, its `label` the name the user reads there ("t7kb: kitchen"). Make a batch of what the user would take back together — a room's floor, walls and ceiling, a corridor, the lights of one space — not one call per brush: each call is its own undo step and its own read of the map. A batch lands whole or not at all: an op that fails refuses the batch before Radiant sees it.

## An edited world brush or brush entity comes back under a new id

A point entity (`light`, `script_struct`, `info_player_start`) keeps its id through `set` and `move`. A world brush moved or retextured, and any entity that owns brushes (`volume_sun`, triggers, `script_brushmodel`), is deleted and imported anew, and gets a **new id**: the answer's `made` gives it, with `was` the old one. So are a `reflection_probe` and a `misc_prefab`. Use the new ids from then on; when unsure, `t7kb:map_find` again. Why: Radiant's undo brings a replaced brush entity back as world brushes only (MTPlugins' notes), while a delete and an import undo exactly (verified in a test Radiant).

## Cut doors by building around the hole, not with CSG Subtract

**`Selection/CSG/CSG Subtract` carves the selected brush out of every brush it touches, volumes included**, with no warning: a door cutter through a wall of the template's start room also split the start zone's `info_volume`, the `volume_sun`, the `umbra_volume` and the `volume_fpstool` six ways each (verified in a test Radiant). A zone or sun volume in pieces no longer covers what it did. Build a wall with a door as boxes around the hole instead — the wall's two sides and the lintel above, each a box of the same batch. Done by mistake, one Ctrl+Z (`t7kb:map_command` with `Edit/Undo`) takes the whole subtract back.

## CSG Hollow walls are as thick as the grid

`Selection/CSG/CSG Hollow` turns a box into six walls inside the box's bounds, as thick as the grid: 8 at grid 8, 16 at grid 16 (verified in a test Radiant). Set the grid with `t7kb:map_command` `Grid/Grid Sizes/Grid 16` before, and back to `Grid 8` after — it is the user's grid too. Hollow acts on the selection only, so it is safe beside volumes; its walls share one material, the box's.

## Look from the top first

`t7kb:map_view` with `view` `top` is the layout: outlines, entity names, the ids written on it, and the `scale` (image pixels per unit) to measure with. The 3D view shows the shapes: an unbaked map is black under real lighting, so pass `render` `fake`; and a camera outside a volume brush's bounds (the `volume_sun`, the sky box) sees that brush's texture over everything (verified: sun icons covering the shot) — frame what is inside the map, or shoot from a `camera` inside it. Every view puts the user's camera, render method, 2D axis, zoom and centre back, but they see them move: take the views the work needs.

## Save before building: the build compiles the file, not Radiant's memory

`t7kb:build` reads the `.map` on disk, so edits Radiant hasn't saved are not in the build. When the user wants to build or play, `t7kb:map_command` `File/Save` saves a named map with no dialog (verified in a test Radiant). Never pass `t7kb:radiant_open` `discard` without the user's yes: it drops their unsaved work.

## When the bridge says busy

A modal dialog open in Radiant makes every edit `busy`, naming the dialog: ask the user to close it. A menu entry that opens a dialog (most of those ending in `...`) leaves `t7kb:map_command` waiting on the user, so use the ones that act at once. A minimised Radiant draws nothing: `t7kb:map_view` asks for its window to be restored.
