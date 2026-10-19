---
name: mapping
description: How to build Black Ops 3 maps in Radiant, by hand or through t7kb's map tools (`t7kb:map_edit`) — grid discipline, structural vs detail brushes, sealing BSP leaks (`.lin` leakfile, sky box), CSG/patches for terrain and curves, zombies zones (`info_volume` coverage, respawn points, door/debris clips), clip textures (`clip_player`, `weaponClip`, `clip_nosight`) vs caulk and a `dyn_model`'s collmap. Use when creating or blocking out a map (`t7kb:create`), a compile hits `WROTE BSP LEAKFILE`, `MAX_MAP_TRIANGLES`, a hang on "coalescing coincident windings" or an `-onlyents` brush-count mismatch; a prefab distorts after rotating the source brush; a player dies standing still; power-ups never drop in an area or land out of reach; zombies won't path through an opened door; or the assert `You must specify a script noteworthy with the zone name`. Distinct from t7kb:compiling (running the build) and t7kb:zombies-ai (spawners, navmesh) — this is the geometry behind those failures.
---

# Building BO3 maps in Radiant

Mapping is judgment about space, flow and looks, built out of brushes and patches. The agent works alongside the mapper in their own Radiant: the map tools are its eyes and hands, the game's own tools its check, and the mapper decides. This skill is the method and the traps that fail silently; the details live in **t7kb** (`t7kb:search` then `t7kb:get`) — search it before building anything, since no skill holds them all. **t7kb:compiling** runs the build, **t7kb:lighting** lights the map, **t7kb:zombies-ai** owns the AI that walks it.

## Start a new map with `t7kb:create`, not from an empty `.map`

A usermap needs more than its `.map` — its zone, scripts and sound config — and `t7kb:create` makes them all from the mod tools' templates; `t7kb:radiant_open` then opens the map, and **t7kb:compiling** builds it. Pick the template knowingly: `ZM Mod Level` is Treyarch's plain one; `ZM Basic Level` and `ZM Advanced Level` come from a community asset pack and add a starter HUD, a weapons table and sound aliases under `share/raw/sound/` that every map built from them shares (verified in the install's templates). What a new map's start room means for the first edits is in `references/map-tools.md`.

## Work in the mapper's Radiant, never in the `.map` file

Radiant holds the open map, and its next save overwrites the file on disk, so every change goes through t7kb's map tools into the Radiant the mapper has open, where they watch it land and take it back from Radiant's undo menu. The order of work:

1. Open the map (`t7kb:radiant_open`); drop the mapper's unsaved changes only on their yes.
2. Find what is there (`t7kb:map_find`) before touching it.
3. Edit in batches the mapper would undo as a whole (`t7kb:map_edit`). A batch refused because the mapper changed something since it was read means read again and check the request still holds before resending.
4. Look at the result (`t7kb:map_view`).
5. Use Radiant's own tools for what they do better — CSG, the Patch menu (`t7kb:map_command`).
6. Save through Radiant (`File/Save`) before a build: the build compiles the file on disk, not Radiant's memory.

The traps of working this way — the template's start room, batches, undo, copies, zones, Radiant's tools — are in `references/map-tools.md`.

## The mapper decides; the agent places what it is asked

The map is the mapper's: its layout, geometry detail, materials, props, lights, probes and mood. The agent places the pieces it is asked for, each one its own, so every piece is a choice it can name and the mapper can read in the batch. It leaves the rest of the map alone: nothing retextured, propped or reshaped beyond the request.

**When a request leaves a choice open, ask.** A look, a layout, a size: the mapper's answer, the plan, or Treyarch's own references settle it, never a number the agent picks alone. Treyarch's reference sizes (walls, floors, storeys, doors, windows, hallways, stairs, jumps) are in `docs_modtools/Scale_Standards.pdf`, searchable in t7kb; a zombies map trades real proportions for gameplay, so a plan's sizes are a start, not a limit. Building from a plan or a brief is in `references/map-plans.md`; materials and looking at the result in `references/map-looks.md`.

## Learn each task from Treyarch's maps before building it

How a thing is built in BO3 is in Treyarch's own sources: the shipped maps and prefabs under `map_source/` (the ones dated with the install, not a community pack's), the `docs_modtools` PDFs and the tutorials in t7kb, and Radiant's own commands (Auto Caulk, Generate Clip, Make Detail) before redoing by hand what one already does. Before building something new — a doorway, a staircase, a light fixture, a zone — find a stock example and measure it, and say which one the batch follows. A stock map places much of its geometry as `misc_prefab`s: read each prefab's own `.map` with the prefab's `origin`, `angles` and `modelscale` applied, recursively, and take a brush's corners from its planes' intersections, never from the three points that define each plane.

## Check against the game's own tools, then with the mapper

The game's tools say what is wrong before the mapper has to look: the build's report (cod2map's problems with where they are, the link's errors and warnings: **t7kb:compiling**), the bake's warnings (a probe out of range, a shadow dropped), and the compiled navmesh, where a passage it doesn't cover is one zombies never take (**t7kb:zombies-ai** has where it is and how to read it). Then the mapper walks the map in game: their eyes are the last check, not the first.

## Grid discipline and brush basics

Work on the standard **8-unit grid** (keys 1–9 pick a grid of 1, 2, 4, 8, 16, 32, 64, 256 or 512, so 8 is key 4; the `` ` `` key picks 0.5, `[` and `]` step finer and coarser; Radiant's key bindings, verified in its settings) — staying grid-aligned is what keeps brushes meeting cleanly instead of leaving micro-gaps that cause leaks or bad lighting seams. Draw a brush by dragging on the 2D view; copy with Ctrl+C/Ctrl+V rather than the old space-bar nudge from earlier CoD titles.

**Structural vs detail matters for compile, not just organization.** Structural brushes define the level's portal/visibility skeleton (walls, floors — anything that should block visibility); detail brushes (Radiant's Make Detail) are excluded from that calculation. Build the structure in structural brushes first, cosmetic detail on top — mixing them makes the level harder to portal and slower to compile. Texture anything the player won't see with **caulk** (invisible, doesn't render — check with F9) to cut triangle count; **mitre corners** to 45° where structural brushes meet to save texture seams and keep portaling clean.

## Sealing the level: BSP leaks

A **BSP leak** means the level isn't fully sealed — compile writes a `.lin` **leakfile** and the log prints `WROTE BSP LEAKFILE: <path>.lin`; load that file in Radiant to see the leak path traced from the map's centre to the gap. The most common cause is a missing or incomplete **sky brush box**: one big brush enclosing the whole map, `Selection → CSG → CSG Hollow`, the resulting shell textured `sky`. If there is one, look for something (a model, a stray brush) poking *through* it — that alone leaks even though the rest of the map looks intact. A leaking map can still run, but the leak is a real fault worth fixing rather than ignoring.

## Terrain, curves, and patches

Terrain and curved detail use **patches**, not brushes — drag one out, pick a row×column grid size, then drag its vertices (`V`). Extend a patch with Alt+scroll on a selected edge (community). Join two patches by welding their shared points (Radiant's Weld Equal Patch Pts, which has no key), or toggle a **tolerant weld** (Ctrl+Shift+J) for points that only nearly meet (verified in Radiant's key bindings). Curve patches build roads and arches the same way, with more control points. A patch draws on one side only: seen from its back it is invisible.

## Prefabs: rotate the prefab, not the brush

**Rotating a raw brush and then prefabbing it can distort its scale** (verified; recurring report). Prefab the brush *before* rotating, then rotate the placed prefab instance; re-prefabbing an already-rotated brush doesn't normalize it. Prefabs nest (a complex build can be several layers deep) — expect to unpack and re-save to edit something buried inside one.

## Collision: clip brushes are the default, not the only option

Models have **no collision by default**, but they aren't limited to it — an xmodel can carry its own **collmap** (assigned in APE, baked in via `use_collmap`), and then the placed model collides on its own and needs no hand clip. Absent that, block movement around a model with a separate clip-textured brush; it needn't match the model's shape (one tall, long clip brush over a car or barrel is enough). For a tighter fit, Radiant's Generate Clip builds a clip hull matching the model's silhouette — but it can generate too many faces and fail the compile on a very detailed model, so fall back to a crude brush if it does.

**`dyn_model` entities are the exception — don't clip them at all.** A `dyn_model`'s collision and physics come from the model and its `physpreset`, not from brushes around it; wrapping one in a clip brush fights the entity type.

**Clip textures aren't one-size-fits-all** — `clip`/`clip_full` (everything), `clip_player`, `weaponClip`/`clip_weapon` (bullets only, no player), `clip_missile`, `clip_ai`, `clip_vehicle`, and `clip_nosight` (blocks movement, not AI sightlines) each gate a different actor or projectile class — pick the one that matches what is to be blocked, not `clip` by habit. A common stair or ramp trick: `Make Weapon Clip` on the visible geometry (keeps bullet collision, drops player collision) over a smooth `clip_player` ramp, so players glide up the stairs' silhouette instead of catching on each step. **Caulk is not clip**: caulk is a *non-drawing* face, not a non-colliding one — a caulked brush still blocks the player (community-confirmed: a caulk doorway with colliding patches blocked the player until the patches were weapon clip). Use caulk to cut rendering, clip to shape collision, neither as a stand-in for the other. A face's material is also what it collides as, so **never caulk a clip brush's hidden faces**: one caulk face makes a `clip_player` brush solid to the AI too, and the navmesh, which `radiant/configs/navmesh.json` builds straight through `clip_player`, stops at it (verified on a real build, the navmesh read back).

## Treyarch draws with detail and collides with clip

In Treyarch's shipped sources (zm_giant, mp_combine, mp_sector and their prefabs) 60% of the brushes carry a `contents` flag: `nonColliding` (11,482: trims, beams and ironwork nobody bumps into), `weaponClip detail`, with `ai_nosight` or not (about 19,000: bullets hit them, nobody walks into them), `detail` (1,496). Movement collides with a few big hulls laid over that detail: zm_giant has 576 `clip` brushes (median 41 × 163 × 216), 93 `clip_player`, 36 `clip_ai` (verified in the install). Radiant's Make Non-Colliding, Make Weapon Clip and Make Detail set them, as a brush's or patch's `contents`.

**A doorway is an opening with a lintel and a trim frame, not a gap to the ceiling.** zm_giant's `tp_east_door`: an opening through a 12-thick wall, the wall continuing above it as a lintel; a 2-unit trim of `weaponClip detail` round the opening; `clip` hulls on each side; the buyable door a `clip` `script_brushmodel` filling the opening, its look a `script_model` (verified in the install). The trim never narrows the way through: players and zombies stop on the same clip. A blockout's plain walls collide as they are; a finished room's detail takes its flags and the movement its hulls.

## Zones: sealing the level isn't just BSP leaks

Beyond geometric leaks, a **zone** is the unit of playable space in a custom zombies map — an `info_volume` brush with `script_noteworthy = player_volume`, `target`ing that zone's spawners, wired into GSC via `add_adjacent_zone` (placing one through the map tools: `references/map-tools.md`). The zone volumes are read by more than the spawner logic, and each reader fails silently:

- **Un-zoned ground kills.** A player not inside an *enabled* zone is killed by `_zm.gsc`'s out-of-playable-area monitor, which re-checks every `3` seconds (`get_player_out_of_playable_area_monitor_wait_time`) — so a hole in coverage reads as a mystery instant death, not a leak or a collision bug. Check zone coverage first when a player dies just standing somewhere. A spot that must be safe without being a zone (a teleporter's holding room, a cutscene position) takes a `script_noteworthy "life_brush"` volume, which the monitor honours.
- **Power-ups only drop where a zone covers the kill.** `_zm_powerups.gsc` keeps a drop only if it `istouching` a `player_volume` — so zombies killed in an un-zoned area never drop anything, and a volume that spills past a window barrier drops power-ups where nobody can reach them. Zone exactly the reachable space.
- **A `player_respawn_point` must name its zone** in `script_noteworthy`, or `_zm_zonemgr` asserts `player_respawn_point: You must specify a script noteworthy with the zone name`. A typo'd name gives no error at all — the point simply never unlocks (inferred from the unlock-by-name logic).

**`DYNAMICPATH` isn't what makes an opened door passable.** `_zm_blockers.gsc` calls `DisconnectPaths()` on every door `script_brushmodel` at init and `ConnectPaths()` on the `script_noteworthy "clip"` piece when it opens, so the community fix of ticking `DYNAMICPATH` on the clip is not the missing piece (zm_giant's one door clip carries it, beside the script's own calls). A door zombies still won't path through after opening is usually a clip too thin to cut the navmesh cleanly (community report: use thick clip brushes).

## Compile errors and triangle budget

- **Compile hangs after "coalescing coincident windings."** Usually non-manifold or overlapping geometry, or overly detailed brushwork. Run Radiant's Check Map for Errors, simplify the offending brushwork, or compile in sections to isolate it. On a first compile on a machine, also give it time — some systems are legitimately slow here.
- **`ERROR: MAX_MAP_TRIANGLES (3072000) exceeded`.** Literally too many triangles — reduce patch detail, caulk faces the player never sees, cut high-poly models. Radiant's **Tri Density** view shows where the budget goes.
- **`-onlyents` (fast entity-only recompile) errors with a brush-count mismatch** if any brush changed since the last full compile — it's only valid for entity-only iteration; any brush edit forces a full recompile.

## Don't invent

For anything Treyarch-shipped named here (dvars, exact texture names, KVPs) confirm against the raw mod-tools install before stating it as fact, same as any other shipped token. If neither t7kb nor the raw install supports a specific brush, patch or prefab mechanic, don't assert it exists.
