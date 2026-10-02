---
name: mapping
description: How to build Black Ops 3 maps in Radiant — grid/brushwork discipline, structural vs detail brushes, sealing against BSP leaks (`.lin` leakfile, sky brush box), CSG/patches for terrain and curves, prefab-then-rotate ordering, zombies zones (`info_volume` coverage, respawn points, door/debris clips), and clip textures (`clip_player`, `weaponClip`, `clip_nosight`) vs caulk and a `dyn_model`'s collmap/physpreset. Use when a compile hits `WROTE BSP LEAKFILE`, `MAX_MAP_TRIANGLES`, a hang on "coalescing coincident windings" or an `-onlyents` brush-count mismatch; a prefab distorts after rotating the source brush; a player dies just standing still; power-ups never drop in an area or land out of reach; zombies won't path through an opened door; or the assert `You must specify a script noteworthy with the zone name`. Distinct from t7kb:compiling (running the build) and t7kb:zombies-ai (spawners, navmesh) — this is the geometry and placement behind those failures.
---

# Building BO3 maps in Radiant

Radiant is brush/patch geometry, not code — the craft here is grid discipline, sealing the level, and knowing which compile error points at which kind of geometry mistake. Look up exact texture names, dvars, and specific error strings in **t7kb** (`t7kb:search` then `t7kb:get`); this skill is the method and the recurring gotchas around it.

## Grid discipline and brush basics

Work on the standard **8-unit grid** (hotkeys 1–4 step the grid size; `~` drops to 0.5, `[`/`]` go finer/coarser) — staying grid-aligned is what keeps brushes meeting cleanly instead of leaving micro-gaps that cause leaks or bad lighting seams. Draw a brush by dragging on the 2D view; copy with Ctrl+C/Ctrl+V rather than the old space-bar nudge from earlier CoD titles.

**Structural vs detail matters for compile, not just organization.** Structural brushes define the level's portal/visibility skeleton (walls, floors — anything that should block visibility); detail brushes (`CSG → Make Detail`) are excluded from that calculation. Do all layout/structure in structural brushes first, add cosmetic detail on top — mixing them in makes the level harder to portal and slower to compile. Texture anything the player won't see with **caulk** (invisible, doesn't render — check with F9) to cut triangle count; **mitre corners** to 45° where structural brushes meet to save texture seams and keep portaling clean.

## Sealing the level: BSP leaks

A **BSP leak** means the level isn't fully sealed — compile writes a `.lin` **leakfile** and the log prints `WROTE BSP LEAKFILE: <path>.lin`; load that file in Radiant to see the exact leak path traced from map center to the gap. The most common cause is a missing or incomplete **sky brush box**: draw one big brush enclosing the whole map, `Selection → CSG → CSG Hollow`, texture the resulting shell with the `sky` texture. If you already have one, check for something (a model, a stray brush) poking *through* it — that alone is enough to leak even though the rest of the map looks intact. A leak doesn't necessarily break the playable map (it can still run), but it's a real correctness issue worth fixing rather than ignoring.

## Terrain, curves, and patches

Terrain/detail work uses **patches**, not brushes — drag one out, pick a row×column grid size, then edit verts (hold `V`). Extend a patch with Alt+scroll on a selected edge; join two patches by selecting both (parent first, then child), hitting `V`, then `W` to weld matching verts, or Ctrl+Shift+J for a **tolerant weld** across a looser vert selection. Curve patches build roads/arches the same way, just with more manipulable control points.

## Prefabs: rotate the prefab, not the brush

**Rotating a raw brush and then prefabbing it can distort its scale** (verified; recurring report). The fix: prefab the brush *before* rotating, then rotate the placed prefab instance. Don't re-prefab an already-rotated brush model expecting it to normalize. Prefabs also nest (a complex build can be prefabs several layers deep) — expect to unpack/re-save when you need to edit something buried inside one.

## Collision: clip brushes are the default, not the only option

Models have **no collision by default**, but they aren't limited to it — an xmodel can carry its own **collmap** (assigned in APE, baked in via `use_collmap`), in which case the placed model collides on its own and you don't hand-clip it. Absent that, block player/zombie movement around a model with a separate clip-textured brush; it doesn't need to match the model's shape (a single tall, long clip brush over a car or barrel is enough). For a tighter fit than a crude box, `Select → Make Geo → Generate Clip` auto-builds a clip hull matching the model's exact silhouette — but it can generate too many faces and fail the compile on a very detailed model, so fall back to a crude brush if it does.

**`dyn_model` entities are the exception — don't clip them at all.** A `dyn_model`'s collision and physics response come from the model + its `physpreset`, not from brushes placed around it; wrapping one in a clip brush is fighting the entity type, not supporting it.

**Clip textures aren't one-size-fits-all** — `clip`/`clip_full` (everything), `clip_player`, `weaponClip`/`clip_weapon` (bullets only, no player), `clip_missile`, `clip_ai`, `clip_vehicle`, and `clip_nosight` (blocks movement, not AI sightlines) each gate a different actor/projectile class — pick the one that matches what you're actually trying to block, not just `clip` by habit. A common stair/ramp trick: `Make Weapon Clip` on the visible geometry (keeps bullet collision, drops player collision) paired with a smooth `clip_player` ramp underneath, so players glide up the stairs' silhouette instead of catching on each step. Don't confuse clip with **caulk**: caulk is a *non-drawing* face, not a non-colliding one — a caulked brush still blocks the player (community-confirmed: a caulk doorway with colliding patches blocked the player until the patches were weapon clip). Use caulk to cut rendering, clip to shape collision, and neither as a stand-in for the other.

## Zones: sealing the level isn't just BSP leaks

Beyond geometric leaks, a **zone** is the unit of playable space in a custom zombies map — an `info_volume` brush with `script_noteworthy = player_volume`, `target`ing that zone's spawners, wired into GSC via `add_adjacent_zone`. The zone volumes are read by more than the spawner logic, and each reader fails silently:

- **Un-zoned ground kills.** A player not inside an *enabled* zone is killed by `_zm.gsc`'s out-of-playable-area monitor, which re-checks every `3` seconds (`get_player_out_of_playable_area_monitor_wait_time`) — so a hole in coverage reads as a mystery instant-death, not a leak or a collision bug. Check zone coverage first when a player dies just standing somewhere. A spot that must be safe without being a zone (a teleporter's holding room, a cutscene position) takes a `script_noteworthy "life_brush"` volume, which the monitor honours.
- **Power-ups only drop where a zone covers the kill.** `_zm_powerups.gsc` keeps a drop only if it `istouching` a `player_volume` — so zombies killed in an un-zoned area never drop anything, and a volume that spills past a window barrier drops power-ups where nobody can reach them. Zone exactly the reachable space.
- **A `player_respawn_point` must name its zone** in `script_noteworthy`, or `_zm_zonemgr` asserts `player_respawn_point: You must specify a script noteworthy with the zone name`. A typo'd name gives no error at all — the point simply never unlocks (inferred from the unlock-by-name logic).

**Door and debris clips don't need `DYNAMICPATH`.** `_zm_blockers.gsc` calls `DisconnectPaths()` on every door `script_brushmodel` at init and `ConnectPaths()` on the `script_noteworthy "clip"` piece when it opens — the community fix of ticking `DYNAMICPATH` on the clip is not the missing piece. A door zombies still won't path through after opening is usually a clip too thin to cut the navmesh cleanly (community report: use thick clip brushes).

## Compile errors and triangle budget

- **Compile hangs after "coalescing coincident windings."** Usually non-manifold/overlapping geometry or overly detailed brushwork. Use Radiant's "Check Geometry" tool, simplify the offending brushwork, or try compiling in sections to isolate it. On a first compile on a given machine, also just give it time — some systems are legitimately slow here.
- **`ERROR: MAX_MAP_TRIANGLES (3072000) exceeded`.** Literally too many triangles — reduce patch detail, caulk faces the player will never see, cut high-poly models. Radiant's **Tris Density** debug view shows where the budget is being spent.
- **`-onlyents` (fast entity-only recompile) errors with a brush-count mismatch** if any brush geometry changed since the last full compile — it's only valid for entity-only iteration; any brush edit invalidates it and forces a full recompile.

## Don't invent

For anything Treyarch-shipped named here (dvars, exact texture names, KVPs) confirm against the raw mod-tools install before stating it as fact, same as any other shipped token. If neither t7kb nor the raw install supports a specific brush/patch/prefab mechanic, don't assert it exists.
