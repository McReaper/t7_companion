---
name: moving-platforms
description: How to build a moving carrier in Black Ops 3 zombies — buses, tanks — a `script_vehicle` on `info_vehicle_node` paths plus a `moving_platform_enabled` `script_brushmodel`, riders via `LinkTo`. Covers `AttachPath` vs `DrivePath`, `DYNAMICPATH`/`script_badplace`. Use when a platform drifts, teleports, freezes at path end, or sinks into the floor; zombies freeze or lose the player once someone boards; a boarding zombie's climb anim looks right then snaps back outside; a scripted anim on a mover stutters, drifts behind it, or lands wrong; riders with an accepted goal won't walk; the console floods with navmesh errors; a use prompt won't come back or vanishes between two triggers; a vehicle spawns reversed or 90° off; a vehicle path hangs the server at load (`EXE_ERR_SERVER_TIMEOUT`, black screen); or porting TranZit's bus/elevator. Distinct from t7kb:zombies-ai (static-ground navmesh/spawners/traversals) and t7kb:mapping (brushwork/sealing) — the moving-carrier craft and its silent failures.
---

# Moving carriers in BO3 zombies

A mover is **not one entity** — it's a `script_vehicle` that follows a path, a collision volume you stand on, and a separate `moving_platform_enabled` `script_brushmodel` that is the only thing AI can walk on. Almost every bug in this craft comes from expecting one object to do all three jobs. Look exact KVP spellings and asset names up in **t7kb** (`t7kb:search` then `t7kb:get`) and confirm them against the raw install; this skill is the division of labour and its traps.

For AI that never leaves the ground see **t7kb:zombies-ai**; for the brushwork itself see **t7kb:mapping**; for reading the resulting build errors see **t7kb:debugging**.

## The three parts, and why one entity can't do all of it

| Part | Entity | Provides |
|---|---|---|
| the mover | `script_vehicle` with `vehicletype` | follows the `info_vehicle_node` path |
| collision | the vehicle's xmodel, usually a **collmap** | what players physically stand on |
| AI surface | `script_brushmodel` with `moving_platform_enabled` `1`, linked to the vehicle | the navmesh zombies walk on |

The pairing is by **`script_string`**, not `target`: the vehicle's `script_string` must equal the brushmodel's `targetname`. `target` is reserved for the path's first node. The stock template does `brushes = GetEntArray(self.script_string, "targetname")` then `brush EnableLinkTo(); brush LinkTo(self, "tag_origin")` — note `tag_origin` with no offset **teleports** the brush onto the vehicle, usually desirable since it auto-aligns the AI surface to the collision.

## `AttachPath` + `StartPath`, never `DrivePath`

Symptom: the vehicle drifts off, runs in a straight line forever, wanders in circles, or cuts straight to the last node and parks there.

Cause is in the shipped docs: `DrivePath( [node index] , [allow free drive] )` — *"Starts the vehicle driving this path and uses the vehicle physics, **not locked to the spline**"*. Its first argument is a node **index**, not a start node. On a `type=plane` asset (see below) "not locked to the spline" means it flies *at* the path rather than following it.

Use the spline-locked pair instead:

```gsc
self AttachPath( n_start );      // AttachPath( <node> ) - "Attaches this vehicle to the given path"
self StartPath();
self SetSpeed( speed, accel );
```

**Both BO2's TranZit bus (`attachpath` + `startpath`) and BO3's Origins tank (`attachpath`) use the locked pair**; only community platform prefabs use `DrivePath`. BO2's `self.drivepath = 0` is inert in BO3: only `vehicle_shared`'s `paths()`/`go_path()` read it, and a map-placed zombies vehicle never runs them.

## A closed node cycle hangs the server

(Verified on a real build.) Making the last `info_vehicle_node` `target` the first one — the obvious way to loop a route — produces a black screen at load and `Com_ERROR: EXE_ERR_SERVER_TIMEOUT`, with **no script output at all**. With `SPLINE_NODE` set it is reliably fatal, so a BO3 vehicle path must stay an **open chain**.

To loop a route, keep the chain open and re-`AttachPath`/`StartPath` on the engine's `reached_end_node` notify (`vehicle_shared.gsc`). The loop code, the geometry trick that hides the re-attach, and the degenerate-end-node and `wait 0` traps are in `references/route-loops-and-assets.md`.

## A vehicle that spawns reversed and straightens in the corners is looking too far ahead

Symptom: the vehicle's centre follows the path, but it faces 90° or 180° off, pivots on the spot as it starts, takes the corners tight and only lines up once the geometry forces it round. Not the cause (each tested, no effect): `angles` on the nodes, `angles` on the `script_vehicle`, the entity order in the BSP, `SetBrake`.

Cause: `lookahead` is **seconds** and `speed` is **mph** (`bin/t7.def.json`: *"time[sec] vehicle should look ahead"*, *"speed[mph]"*), so the vehicle steers at a point `speed × 17.6 × lookahead` units ahead. A route authored at `speed 90` / `lookahead 3` aims 4,752 units ahead on a 2,249-unit loop, which is always past the end of the path. Moving the terminal node moves the initial yaw with it; `speed 19` / `lookahead 1` fixes it. Treyarch's own `template.map` runs its nodes at 2.5–15 mph and 0.25–1 s. Fix the KVPs, not the script: the node's **authored** speed sets the distance, not `SetSpeed` (inferred).

Reading `speed` back from script and per-node stops off node KVPs each have a trap: `references/route-loops-and-assets.md`.

## Zombies path on the linked brushmodel, never on the vehicle

(Verified in-game with `ai_showNavMesh 1`.) Both the vehicle and the linked brushmodel generate a navmesh, but zombies only ever move on the **brushmodel's** — the vehicle's is inert, even while the platform is moving.

So the `moving_platform_enabled` brush is load-bearing — deleting it removes AI from the mover entirely. The collmap is the more dispensable, since a solid brushmodel already collides.

Symptom: **the moment a player stands on a `moving_platform_enabled` brush, every zombie freezes**. Known BO3 behaviour with a plain `script_brushmodel` + `MoveTo`; driving a real `script_vehicle` instead fixes it.

**Nothing connects the ground navmesh to the mover's**, and nothing ever will — a moving surface can't be stitched into baked navmesh. Zombies can't *walk on*, so they need a scripted boarding step (next section); once aboard, they path around on the linked brush.

Three consequences of that island being separate:

- **`GetClosestPointOnNavMesh` answers from the static mesh only** (measured). It cannot see the mover's island, so using it to validate or snap a goal on a deck rejects perfectly good points. Don't gate a rider's goal on it.
- **Overlapping `moving_platform_enabled` brushes generate competing meshes** (measured), and the console fills with navmesh errors at over a hundred a second. Where the walkable surface is one volume, model it as **one convex brush** — the flood stops.
- **The mesh has to fit an AI.** `bin/default_navmesh_settings.json` sets `characterHeight 72`, `minCharacterWidth 31`, `simplification.minCorridorWidth 30`, `maxStepHeight 18` — a gangway or bus aisle narrower than ~31 units silently produces no mesh at all, reading as "the platform doesn't work" rather than "the corridor is too tight".

**Treyarch never pathfinds an AI on a moving vehicle.** `vehicleriders_shared` puts every rider in `PathMode( "dont move" )` for the ride; no shipped script walks an actor around on a moving mover. Riders holding a valid accepted goal on a moving deck frequently just stand still with every readable state healthy (`pathmode move allowed`, `scripted 0`, `enemy 1`, goal accepted, `ignoreall 0`), even with the vehicle stopped. Use the tank's model instead: park the rider (`setgoalpos( self.origin )` with a generous radius) and script movement between authored, vehicle-local points.

For the *parked* case the tank cuts the ground with a linked `navmesh_cutter` entity (mechanism in `references/route-loops-and-assets.md`).

## Anything that must travel gets `LinkTo`'d — including zombies

`LinkTo( <entity>, [tag], [originOffset], [anglesOffset] )` is server-side, and the offset-preserving idiom is:

```gsc
ent LinkTo( mover, "", mover WorldToLocalCoords( ent.origin ), ent.angles - mover.angles );
```

BO2's bus links **everything** this way: window `zbarrier`s, rebuild and zombie triggers (with `EnableLinkTo()` first — triggers need it, models don't), the plow clip, and the zombies. A boarding zombie is not pathed anywhere; it is made a **passenger**:

```gsc
self.attachent = level.the_bus;
self.attachtag = self.opening.bindtag;
self linkto( self.attachent, self.attachtag );
```

Downstream: the tear/climb anims are `animscripted` anchored on a `gettagorigin( self.attachtag )` lookup, with `animmode( "noclip" )` while passing through geometry and `animmode( "gravity" )` restored after. The handoff back to navmesh is `unlink()` then a fresh goal. BO2 re-reads that tag on **every loop iteration**; don't port that — in BO3 the link already holds the anim in the moving frame, and re-anchoring on top of it stutters.

**A `zbarrier` survives `LinkTo`** — boards and all, `SetMovingPlatformEnabled(1)` on the barrier plus a linked parent (verified in-game). Community claims that zbarriers can't be moved are wrong; don't rebuild barriers out of `script_model`s.

**Debug a link in the parent's local space, not world space.** `WorldToLocalCoords` is rotation-invariant; a world-space delta also changes when the parent merely turns, reporting a false "drift" on every corner.

Two more link facts (measured). **Re-issuing `LinkTo` on an entity the engine already considers linked does nothing** — a second call with a different offset is silently ignored, so a re-anchor has to `Unlink()` first. And **a `LinkTo` does not move the entity until the next server tick**, so anything sampled on the same frame reads the *pre-link* placement; skipping a `WAIT_SERVER_FRAME` before probing reports the link's own work as drift.

## Don't hand-roll the anchor — `animation::play( anim, mover, tag )` already is the boarding sequence

Calling `AnimScripted` directly on a mover reinvents `animation_shared`. `animation::play( animation, <entity>, <tag> )` takes an **entity and a tag** rather than a transform, and `animation_shared::_play` is worth reading once because it *is* the correct shape:

```gsc
v_pos = ent GetTagOrigin( tag );                    // the anchor is composed ONCE
v_ang = ent GetTagAngles( tag );
self ForceTeleport( v_pos, v_ang );                 // an actor; a non-actor gets .origin/.angles
self LinkTo( ent, tag, ( 0, 0, 0 ), ( 0, 0, 0 ) );
self AnimScripted( animation, v_pos, v_ang, animation, "normal", undefined, n_rate, n_blend_in, n_lerp, n_start_time, true, … );
self waittillmatch( animation, "end" );
self Unlink();                                      // unless b_unlink_after_completed is false
```

**The link is what makes the animation ride the mover. Not a loop.** The anchor is a world point fixed at the call and never touched again; the parent carries the body. `zm_tomb_tank::climb_tag` is the same five lines by hand (`linkto` / `animscripted` / `donotetracks` / `unlink` / `setgoalpos( self.origin )`), with no `AnimMode`, `OrientMode`, `PathMode` or teleport.

Re-reading the tag every server frame and resuming the anim through `AnimScripted`'s `animationTime` argument holds alignment but visibly stutters (measured). When an anim drifts off the back of a mover, the missing piece is the **link**, not a finer correction. Re-anchoring per frame is only right when there is no parent to link to at all (a zipline — **t7kb:scripting**); a mover always has one.

Two shipped details. Treyarch's comment — *"LinkTo was not working correctly with animation and always positioning the object as if it was linked to the tag_origin. Moving the object to the tag position fixes this."* — means the `ForceTeleport` onto the tag **before** linking is deliberate. And `waittillmatch( animation, "end" )` needs the anim to carry an `end` notetrack: a ported anim that lost its notetracks hangs there forever, and `wait GetAnimLength( anim )` is the fallback (**t7kb:scripting** owns that trap).

BO3's AI-boards-a-vehicle flow is `vehicleriders_shared.gsc`: `animation::reach` walks the AI to the start, `animation::play` runs the enter anim on the align tag, a threaded `RideAnim` plays with unlink disabled, and the rider gets `PathMode( "dont move" )` for the ride.

## An anim never moves a linked entity — and `AnimScripted`'s `origin` is where it STARTS

Confusing either of these produces an animation that looks perfect and lands in the wrong place.

Sampled in the vehicle's own frame, a boarding zombie's position shows a **peak displacement of 0** for the whole climb-in anim (measured). `AnimScripted` parks the entity on the anchor it is handed; the **rendered pose** follows that anchor, the **entity** follows its link, and the two are independent. Nothing the animation does will ever relocate a linked actor — which is also why a stale anchor "leaves the zombie behind" visually while its collision never moved at all.

And the anchor is the **start**, not an origin for the animation's coordinate space — the shipped docs for its siblings say so plainly (`GetStartOrigin`: *"Get the starting origin for an animation, in world coordinates, given its current position, and angles"*). The end is therefore only ever start-plus-travel. **Corollary: translating an export is a no-op for playback.** Shifting every `OFFSET` so the clip "ends at (0,0,0)" changes the travel by nothing, so it changes where the anim plays by nothing — see **t7kb:animation**.

So the destination is computed, never authored into the file. **The normal case is a tag** — and why the tank needs no fixup: *its tags are the destinations*. Pass the tag transform to `GetStartOrigin( v_org, v_ang, anim )` / `GetStartAngles(...)` and you get where the entity must begin for the clip to land there. `vehicleriders_shared::can_get_in` is the canonical use — tag in, boarding spot out, then `FindPath` to it; `animation::reach` `force_goal`s to the same point. (A port whose joints sit outside the vehicle, e.g. BO2's window anims, has no tag to land on — deriving that destination instead is in `references/route-loops-and-assets.md`.)

Unlike the anchor, the destination must be composed **at the moment of the move**, because by then the mover has travelled. Composing it before the anim misses by the distance the bus travels meanwhile (~110 units per second of anim, plus rotation). `zm_tomb_tank::tank_get_jump_down_offset` builds its arrival the same way, from a `tank_offset` field authored on a map struct.

## A use prompt on a mover has to be a map-placed trigger

A player holds **one use trigger at a time**, so two overlapping volumes cancel and someone between them gets no prompt; `zm_unitrigger`'s per-player escape can't follow a moving parent. Place the trigger in Radiant, `enablelinkto()` / `linkto( bus )` with no offset / `setmovingplatformenabled( 1 )`, and keep volumes from touching. Why `SetInvisibleToAll()` and `TriggerEnable` don't behave as named for a player already inside: **`references/route-loops-and-assets.md`**.

## `moving_platform_enabled` and `DYNAMICPATH` are different jobs — they never co-occur

Both are engine-side and both sound like "make pathing work on this".

- **`moving_platform_enabled`** `1` — on a `script_brushmodel`/`script_model`. "AI can stand on this and be carried." Shipped on Treyarch's own elevator prefabs and MP traversal prefabs.
- **`DYNAMICPATH`** `1` — note the **uppercase, singular** spelling. "Recut the navmesh when this moves or disappears." Shipped on `zm_giant`'s debris/doors and the MP bomb-site prefabs.

Across every entity in the shipped `map_source/` that carries either, **not one carries both**. Obstacle versus floor. A moving platform does **not** want `DYNAMICPATH` — it reconnects paths unnecessarily.

Pairing them is community advice (Discord, t7kb 0.25) and is ruled out: ticking `DYNAMICPATH` on a bus's `moving_platform_enabled` brush made zombies stop seeing a player aboard entirely (verified in-game).

Two spelling traps: the KVP is `DYNAMICPATH`, so a case-sensitive search for `dynamicpaths` finds nothing and you conclude it doesn't exist. And `moving_platform` (no `_enabled`) exists too — but as a **value** (`vehicletype`/`targetname`), not a key.

## Two stock vehicle defaults fight a platform

Radiant sets **`script_badplace`** and **`script_disconnectpaths`** on every `script_vehicle`, and both sabotage a platform zombies must reach — both gate on `isdefined()`, so setting `0` doesn't opt out, and **omitting `script_disconnectpaths` doesn't either** (the default disconnects). Kill both from script, using the `endon`s Treyarch built in: `self notify( "kill_badplace_forever" )` / `self notify( "kill_disconnect_paths_forever" )`. Full KVP semantics and BO2's surgical alternative (no `script_badplace`/`script_disconnectpaths` at all) are in `references/route-loops-and-assets.md`.

## `#using_animtree` needs a zone entry, or the server dies with no error

(Verified from `console_mp.log`.) Adding `#using_animtree("generic")` + `UseAnimTree(#animtree)` (both required by the stock platform setup, via the vehicle blackboard) without the matching zone line kills the server during load: black screen, `Com_ERROR: EXE_ERR_SERVER_TIMEOUT`, and **not one line of your script's output** — the VM dies before reaching any print.

```
rawfile,animtrees/generic.atr
```

The link succeeds regardless, so a green build proves nothing (an unknown function name also links clean). Zone a vehicle as `vehicle,<name>` plus `xmodel,<name>`.

Diagnostic: if a feature script produces **zero** prints, it isn't a logic bug — the VM never got there. Grep `console_mp.log` for your prefix before touching the code.

## The shipped platform assets are collmap-only and `type=plane`

The community `t7_moving_platforms.gdt` platforms are `type=plane` (why `DrivePath` misbehaves on them) with a collision-only xmodel, so they render invisible. Sizing and the prefab's `vehicletype` trap: **`references/route-loops-and-assets.md`**.

## Traversals can't move

BO3 can only enable/disable a traversal (`LinkTraversal`/`UnlinkTraversal`), never reposition one — so boarding a moving carrier is `linkto` plus scripted jump anims, as BO2's bus does. Detail: **`references/route-loops-and-assets.md`**.

## Don't invent

KVP spellings, notify names and function semantics above come from the shipped install (`share/raw/scripts/shared/vehicle_shared.gsc`, `animation_shared.gsc`, `vehicleriders_shared.gsc`, `docs_modtools/bo3_scriptapifunctions.htm`, `bin/default_navmesh_settings.json`, `map_source/_prefabs/`) or real sessions; items marked *verified* were reproduced in-game. Community prefabs disagree with the shipped code — check `vehicle_shared.gsc` and the API docs before asserting a flag, notify or KVP exists; prefer what BO2's bus and `zm_tomb_tank` do over a prefab pack.
