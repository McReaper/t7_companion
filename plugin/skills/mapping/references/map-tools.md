Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — building in the user's running Radiant through t7kb's map tools. Read the parent skill first; come here for the sizes to build to, what a new map already holds, zoning, and the traps of each tool; turning a plan into brushes is in `map-plans.md`, materials, views and light in `map-looks.md`.

Contents:

- [Build to the official scale standards](#build-to-the-official-scale-standards)
- [A new map is the template's room in a sky box: build inside it](#a-new-map-is-the-templates-room-in-a-sky-box-build-inside-it)
- [One batch per step the user would undo](#one-batch-per-step-the-user-would-undo)
- [Zone every space players reach: volumes, spawners, then the script's links](#zone-every-space-players-reach-volumes-spawners-then-the-scripts-links)
- [An edited world brush or brush entity comes back under a new id](#an-edited-world-brush-or-brush-entity-comes-back-under-a-new-id)
- [Build one, then copy it](#build-one-then-copy-it)
- [CSG Hollow walls are as thick as the grid](#csg-hollow-walls-are-as-thick-as-the-grid)
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

So a room's floor is a 16-unit slab, its walls 8 thick and 112 high above it, its ceiling another slab: the brushes the agent places are those numbers, on the 8-unit grid.

## A new map is the template's room in a sky box: build inside it

`t7kb:create`'s `ZM Mod Level` map already holds (verified in the install's template):

- a start room of `script_wall` walls on a `script_floor_ceiling` floor, the materials the stock template blocks out with (`caulk` draws nothing in game); the floor is one slab 16 thick under the whole start zone (±1536, z −16 to 0);
- a **sky box** of six `sky` brushes, 32 thick, around x and y −2048 to 2048 and z −64 to 1024;
- the **start zone**, an `info_volume` (`targetname` `start_zone`, `script_noteworthy` `player_volume`) over x and y ±1536, z 0 to 320, with its rising-zombie structs (`start_zone_spawners`) and the player spawns (`initial_spawn_points`, one `player_respawn_point`);
- a `volume_sun` (±1920, up to 896), an `umbra_volume`, a `volume_fpstool`, a `reflection_probe`, an `info_player_start`, the perk machines and box as `misc_prefab`s.

Build the blockout inside the sky box and the start zone first: it seals and plays with nothing else moved. A floor laid inside the template's slab flickers against it in game (verified in a test Radiant): delete the slab, or rebuild it around what you build, before laying floors there. Replacing the start room moves its contents too: its spawners, player spawns and prefabs stand where its floor was, and zombies need a floor from their structs to the player (the navmesh is compiled from floors) — a blockout run that deleted the room and built beside it left them floating, and its zombies had no path in game. A room outside the start zone is a new zone, below. A map larger than the box needs the six `sky` brushes rebuilt around it and the three volumes grown with it — delete and place brushes in one batch.

## One batch per step the user would undo

`t7kb:map_edit` lands a whole batch as one entry in Radiant's undo menu, its `label` the name the user reads there ("t7kb: kitchen"). Make a batch of what the user would take back together — a room's floor, walls and ceiling, a corridor, the lights of one space — not one call per brush: each call is its own undo step and its own read of the map. A batch lands whole or not at all: an op that fails refuses the batch before Radiant sees it.

## Zone every space players reach: volumes, spawners, then the script's links

A zone is what the zone manager reads: every `info_volume` named for it, the riser spawners its first volume targets, and an `add_adjacent_zone` call in the map's script for each passage (verified in the install's `_zm_zonemgr.gsc`). Place each volume as a `brush` with `class` `info_volume` and the keys the template's start zone carries (`targetname` the zone's name, `target` its spawners', `script_noteworthy` `player_volume`), over the floor players reach: real zones aren't rectangles (Treyarch's `zm_giant` has 12 zones of 32 volumes, some angled). For the spawners, copy one of the template's `start_zone_spawners` structs (`t7kb:map_find` `key` `targetname=start_zone_spawners`) onto the new floor, clear of walls, and give the copies the new zone's target. Then write the `add_adjacent_zone` calls in the map's script, as map_edit writes no script: `always_on` opens a link from the start, a door's flag when it is bought. A player in no zone dies within 3 seconds, and the template's `start_zone` covers ±1536: shrink or delete its volume once the start room moves. A `nav_volume` serves flying AI only (the compiler's warning says so); zombies walk the navmesh built from the floors.

## An edited world brush or brush entity comes back under a new id

A point entity (`light`, `script_struct`, `info_player_start`) keeps its id through `set` and `move`. A world brush moved or retextured, and any entity that owns brushes (`volume_sun`, triggers, `script_brushmodel`), is deleted and imported anew, and gets a **new id**: the answer's `made` gives it, with `was` the old one. So are a `reflection_probe` and a `misc_prefab`. Use the new ids from then on; when unsure, `t7kb:map_find` again. Why: Radiant's undo brings a replaced brush entity back as world brushes only (MTPlugins' notes), while an import undoes exactly (verified in a test Radiant). A deleted brush entity comes back from undo as world brushes too: undoing a batch that deleted the template's `start_zone` brought back a world brush of `volume`, the `info_volume` gone (verified live) — after such an undo, `t7kb:map_find` the volumes and rebuild the ones lost.

## Build one, then copy it

`copy` repeats ids — `by` the step, `count` how many — as a row of pillars, wall panels or lamps from one: `{"copy": {"ids": ["@3"], "by": [112, 0, 0], "count": 3}}`. A copy keeps the original's keys and materials as they are at that point of the batch, `targetname` included (rename a copy with `set`); `@n` on a copy op names every copy, so a following `texture` or `move` takes them all. Copy the entity, not one brush of it.

## CSG Hollow walls are as thick as the grid

`Selection/CSG/CSG Hollow` turns a box into six walls inside the box's bounds, as thick as the grid: 8 at grid 8, 16 at grid 16 (verified in a test Radiant). Set the grid with `t7kb:map_command` `Grid/Grid Sizes/Grid 16` before, and back to `Grid 8` after — it is the user's grid too. Hollow acts on the selection only, so it is safe beside volumes; its walls share one material, the box's.

## Save before building: the build compiles the file, not Radiant's memory

`t7kb:build` reads the `.map` on disk, so edits Radiant hasn't saved are not in the build. When the user wants to build or play, `t7kb:map_command` `File/Save` saves a named map with no dialog (verified in a test Radiant). Never pass `t7kb:radiant_open` `discard` without the user's yes: it drops their unsaved work.

## When the bridge says busy

A modal dialog open in Radiant makes every edit `busy`, naming the dialog: ask the user to close it. A menu entry that opens a dialog (most of those ending in `...`) leaves `t7kb:map_command` waiting on the user, so use the ones that act at once. A minimised Radiant draws nothing: `t7kb:map_view` asks for its window to be restored.
