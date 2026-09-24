---
name: bo3-moving-platforms
description: How to build a moving carrier in Black Ops 3 zombies — buses, tanks — a `script_vehicle` on `info_vehicle_node` paths plus a `moving_platform_enabled` `script_brushmodel`, riders via `LinkTo`. Covers `AttachPath` vs `DrivePath`, `DYNAMICPATH`/`script_badplace`. Use when a platform drifts, teleports, freezes at path end, or sinks into the floor; zombies freeze or lose the player once someone boards; a boarding zombie's climb anim looks right then snaps back outside; a scripted anim on a mover stutters, drifts behind it, or lands wrong; riders with an accepted goal won't walk; the console floods with navmesh errors; a use prompt won't come back or vanishes between two triggers; a vehicle spawns reversed or 90° off; a vehicle path hangs the server at load (`EXE_ERR_SERVER_TIMEOUT`, black screen); or porting TranZit's bus/elevator. Distinct from bo3-zombies-ai (static-ground navmesh/spawners/traversals) and bo3-mapping (brushwork/sealing) — the moving-carrier craft and its silent failures.
---

# Moving carriers in BO3 zombies

A mover is **not one entity** — it's a `script_vehicle` that follows a path, a collision volume you stand on, and a separate `moving_platform_enabled` `script_brushmodel` that is the only thing AI can walk on. Almost every bug in this craft comes from expecting one object to do all three jobs. Look exact KVP spellings and asset names up in **t7kb** (`t7kb:search` then `t7kb:get`) and confirm them against the raw install; this skill is the division of labour and the traps around it.

For AI that never leaves the ground see **bo3-zombies-ai**; for the brushwork itself see **bo3-mapping**; for reading the resulting build errors see **bo3-debugging**.

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

**Both BO2's TranZit bus (`attachpath` + `startpath`) and BO3's Origins tank (`attachpath`) use the locked pair**; only community platform prefabs reach for `DrivePath`, which is why copying one leads you astray. Don't port BO2's `self.drivepath = 0` along with it: in BO3 only `vehicle_shared`'s own `paths()`/`go_path()` read that flag, and a map-placed zombies vehicle never runs them (below), so the line is inert.

## A closed node cycle hangs the server

**Verified the hard way in a real session.** Making the last `info_vehicle_node` `target` the first one — the obvious way to loop a route — produces a black screen at load and `Com_ERROR: EXE_ERR_SERVER_TIMEOUT`, with **no script output at all**. With `SPLINE_NODE` set it is reliably fatal, so a BO3 vehicle path must stay an **open chain**.

To loop a route anyway, keep the chain open and re-`AttachPath`/`StartPath` on the engine's own `reached_end_node` notify (`vehicle_shared.gsc`) instead of closing it. The full working pattern — the loop code, the geometry trick that hides the re-attach, the degenerate-end-node and `wait 0` traps — is in `references/route-loops-and-assets.md`.

## A vehicle that spawns reversed and straightens in the corners is looking too far ahead

Symptom: the vehicle's centre follows the path, but it faces 90° or 180° off, pivots on the spot as it starts, takes the corners tight and only lines up once the geometry forces it round. **Measured, one test each, none of which changed anything:** `angles` on the nodes, `angles` on the `script_vehicle`, the entity order in the BSP, and `SetBrake`.

Cause: `lookahead` is **seconds** and `speed` is **mph** (`bin/t7.def.json`: *"time[sec] vehicle should look ahead"*, *"speed[mph]"*), so the vehicle steers at a point `speed × 17.6 × lookahead` units ahead. A route authored at `speed 90` / `lookahead 3` aims 4,752 units ahead on a 2,249-unit loop, which is always past the end of the path. **Measured:** moving the terminal node moved the initial yaw with it, and re-authoring the nodes at `speed 19` / `lookahead 1` fixed it outright. Treyarch's own `template.map` runs its nodes at 2.5–15 mph and 0.25–1 s. Fix the KVPs, not the script: the vehicle crawled at a `SetSpeed` of 5 the whole time, so it is the node's **authored** speed that sets the distance (inferred from that — at 5 mph the same lookahead would have been a sane 264 units).

Reading that `speed` back from script, and running per-node stops off node KVPs, each carry a trap of their own — both are in `references/route-loops-and-assets.md`.

## Zombies path on the linked brushmodel, never on the vehicle

**Verified in-game with `ai_showNavMesh 1`:** both the vehicle and the linked brushmodel generate a navmesh, but zombies only ever move on the **brushmodel's** — the vehicle's is inert, even while the platform is moving.

So the `moving_platform_enabled` brush is the load-bearing piece — deleting it to "simplify" removes AI from the mover entirely. The collmap is the more dispensable of the two, since a solid brushmodel already collides.

Symptom that trips people first: **the moment a player stands on a `moving_platform_enabled` brush, every zombie freezes**. That is a known, unresolved BO3 behaviour with the plain `script_brushmodel` + `MoveTo` approach; driving a real `script_vehicle` instead is what fixes it.

**Nothing connects the ground navmesh to the mover's**, and nothing ever will — a moving surface can't be stitched into baked navmesh. Zombies can't *walk on*, so they need a scripted boarding step (next section); once aboard, they path around on the linked brush.

Three consequences of that island being separate:

- **`GetClosestPointOnNavMesh` answers from the static mesh only** (**measured**). It cannot see the mover's island, so using it to validate or snap a goal on a deck rejects perfectly good points. Don't gate a rider's goal on it.
- **Overlapping `moving_platform_enabled` brushes generate competing meshes** (**measured**), and the console fills with navmesh errors at over a hundred a second. Where the mover's walkable surface is one volume, model it as **one convex brush** — the flood stops dead.
- **The mesh has to fit an AI.** `bin/default_navmesh_settings.json` sets `characterHeight 72`, `minCharacterWidth 31`, `simplification.minCorridorWidth 30`, `maxStepHeight 18` — a gangway or bus aisle narrower than ~31 units silently produces no mesh at all, reading as "the platform doesn't work" rather than "the corridor is too tight".

**Negative result: Treyarch never pathfinds an AI on a moving vehicle.** `vehicleriders_shared` puts every rider in `PathMode( "dont move" )` for the whole ride, and nothing in the shipped scripts walks an actor around on a mover in motion. On a real port, riders holding a valid accepted goal on a moving deck frequently just stood still, with every readable state healthy (`pathmode move allowed`, `scripted 0`, `enemy 1`, goal accepted, `ignoreall 0`) and no difference when the vehicle stopped. Budget for the tank's model instead — park the rider (`setgoalpos( self.origin )` with a generous radius) and script any movement between authored, vehicle-local points — rather than for making free pathing work.

For the *parked* case the tank does connect the two worlds, but by cutting the ground rather than extending itself — a linked `navmesh_cutter` entity that disconnects/reconnects paths as it stops and leaves. Full mechanism in `references/route-loops-and-assets.md` (from the decompiled `zm_tomb_tank.gsc`, t7kb reliability 0.95).

## Anything that must travel gets `LinkTo`'d — including zombies

This is the single idea the whole craft rests on. `LinkTo( <entity>, [tag], [originOffset], [anglesOffset] )` is server-side, and the offset-preserving idiom is:

```gsc
ent LinkTo( mover, "", mover WorldToLocalCoords( ent.origin ), ent.angles - mover.angles );
```

BO2's bus links **everything** this way: its window `zbarrier`s, the rebuild and zombie triggers (with `EnableLinkTo()` first — triggers need it, models don't), the plow clip, and the zombies themselves. A boarding zombie is not pathed anywhere; it is made a **passenger**:

```gsc
self.attachent = level.the_bus;
self.attachtag = self.opening.bindtag;
self linkto( self.attachent, self.attachtag );
```

Everything downstream follows from that: the tear/climb anims are `animscripted` anchored on a `gettagorigin( self.attachtag )` lookup, with `animmode( "noclip" )` while passing through geometry and `animmode( "gravity" )` restored after. The handoff back to navmesh is `unlink()` then a fresh goal. BO2 re-reads that tag on **every loop iteration** — do not port that part. In BO3 the link already holds the anim in the moving frame, and re-anchoring on top of it stutters.

**A `zbarrier` really does survive `LinkTo`** — boards and all, `SetMovingPlatformEnabled(1)` on the barrier plus a linked parent. **Verified in-game.** Community reports claiming zbarriers can't be moved because their state models carry their own origins are wrong; don't rebuild barriers out of `script_model`s on that basis.

**Debugging a link: measure the offset in the parent's local space, not world space.** `WorldToLocalCoords` is rotation-invariant; a world-space delta also changes when the parent merely turns, which reports a false "drift" on every corner — a false alarm on a link that was working fine.

Two more link facts, both **measured**, both cheap to lose a session to. **Re-issuing `LinkTo` on an entity the engine already considers linked does nothing** — a second call with a different offset is silently ignored, so a re-anchor has to `Unlink()` first. And **a `LinkTo` does not move the entity until the next server tick**, so anything sampled on the same frame reads the *pre-link* placement; a probe that skips a `WAIT_SERVER_FRAME` reports the link's own work as drift.

## Don't hand-roll the anchor — `animation::play( anim, mover, tag )` already is the boarding sequence

Reaching for `AnimScripted` directly on a mover reinvents `animation_shared` — which is how the traps below get discovered one at a time. `animation::play( animation, <entity>, <tag> )` takes an **entity and a tag** rather than a transform, and `animation_shared::_play` is worth reading once because it *is* the correct shape:

```gsc
v_pos = ent GetTagOrigin( tag );                    // the anchor is composed ONCE
v_ang = ent GetTagAngles( tag );
self ForceTeleport( v_pos, v_ang );                 // an actor; a non-actor gets .origin/.angles
self LinkTo( ent, tag, ( 0, 0, 0 ), ( 0, 0, 0 ) );
self AnimScripted( animation, v_pos, v_ang, animation, "normal", undefined, n_rate, n_blend_in, n_lerp, n_start_time, true, … );
self waittillmatch( animation, "end" );
self Unlink();                                      // unless b_unlink_after_completed is false
```

**The link is what makes the animation ride the mover. Not a loop.** The anchor is a world point fixed at the moment of the call and never touched again; the parent carries the body. `zm_tomb_tank::climb_tag` is the same five lines written by hand — `linkto` / `animscripted` / `donotetracks` / `unlink` / `setgoalpos( self.origin )` — with no `AnimMode`, `OrientMode`, `PathMode` or teleport anywhere in the file.

**Measured, at the cost of an afternoon:** re-reading the tag every server frame and resuming the anim through `AnimScripted`'s `animationTime` argument *does* hold the alignment, but it visibly stutters. When an anim drifts off the back of a mover, the missing piece is the **link**, not a finer correction. The one shipped case that re-anchors per frame (this repo's zipline, **bo3-scripting**) has no parent to link to; a mover always does.

Two shipped details worth knowing. Treyarch's own comment on that teleport-then-link — *"LinkTo was not working correctly with animation and always positioning the object as if it was linked to the tag_origin. Moving the object to the tag position fixes this."* — so the `ForceTeleport` onto the tag **before** linking is deliberate, not redundant. And `waittillmatch( animation, "end" )` needs the anim to carry an `end` notetrack: a ported anim that lost its notetracks hangs there forever, and `wait GetAnimLength( anim )` is the fallback (**bo3-scripting** owns that trap).

BO3 ships the full AI-boards-a-vehicle flow around it, in `vehicleriders_shared.gsc`: `animation::reach` walks the AI to the start, `animation::play` runs the enter anim on the align tag, a threaded `RideAnim` plays with unlink disabled, and the rider gets `PathMode( "dont move" )` for the ride.

## An anim never moves a linked entity — and `AnimScripted`'s `origin` is where it STARTS

Two independent facts, and confusing either one produces an animation that looks perfect and lands in the wrong place.

**Measured:** sampling a boarding zombie's position *in the vehicle's own frame* for the whole climb-in anim reported a **peak displacement of 0** while the body was plainly climbing through the window and ending up inside. `AnimScripted` parks the entity on the anchor it is handed; the **rendered pose** follows that anchor, the **entity** follows its link, and the two are independent. Nothing the animation does will ever relocate a linked actor — which is also why a stale anchor "leaves the zombie behind" visually while its collision never moved at all.

And the anchor is the **start**, not an origin for the animation's coordinate space — the shipped docs for its siblings say so plainly (`GetStartOrigin`: *"Get the starting origin for an animation, in world coordinates, given its current position, and angles"*). The end is therefore only ever start-plus-travel. **Corollary that costs a day if you miss it: translating an export is a no-op for playback.** Shifting every `OFFSET` so the clip "ends at (0,0,0)" changes the travel by nothing, so it changes where the anim plays by nothing — see **bo3-animation**.

So the destination is computed, never authored into the file. **The normal case is a tag** — and why the tank needs no fixup: *its tags are the destinations*. Pass the tag transform to `GetStartOrigin( v_org, v_ang, anim )` / `GetStartAngles(...)` and you get where the entity must begin for the clip to land there. `vehicleriders_shared::can_get_in` is the canonical use — tag in, boarding spot out, then `FindPath` to it; `animation::reach` `force_goal`s to the same point. (A port whose joints sit outside the vehicle, e.g. BO2's window anims, has no tag to land on — deriving that destination instead is in `references/route-loops-and-assets.md`.)

This is the *destination*, not the anchor — and unlike the anchor it must be composed **at the moment of the move**, because by then the mover has travelled. Composing it before a 3.5-second anim cost a 325-unit miss: a second of animation is ~110 units of bus, plus its rotation. `zm_tomb_tank::tank_get_jump_down_offset` builds its arrival the same way, from a `tank_offset` field authored on a map struct.

## A use prompt on a mover has to be a map-placed trigger

**A player holds one use trigger at a time**, so two overlapping volumes cancel and someone standing between them gets **no prompt at all**. BO3's per-player escape for barriers — `zm_unitrigger::unitrigger_force_per_player_triggers` — doesn't reach a mover: unitriggers register at a fixed origin, and there is nothing in `_zm_unitrigger.gsc` to follow a parent.

So shape the volumes so they never touch — Radiant's job, not a radius guessed in script — the way BO2's bus rebuild trigger does: **placed in the map**, wired with `enablelinkto()` / `linkto( bus )` with no tag or offset (preserving the mapper's transform) / `setmovingplatformenabled( 1 )`.

Getting the prompt to actually appear is its own trap: `SetInvisibleToAll()` and `TriggerEnable` don't behave the way their names suggest for a player already standing inside the volume. The full three-part mechanism, plus how stock signals "nothing left to repair," is in `references/route-loops-and-assets.md`.

## `moving_platform_enabled` and `DYNAMICPATH` are different jobs — they never co-occur

Both are real, both are engine-side, and mixing them up is easy because both sound like "make pathing work on this".

- **`moving_platform_enabled`** `1` — on a `script_brushmodel`/`script_model`. "AI can stand on this and be carried." Shipped on Treyarch's own elevator prefabs and MP traversal prefabs.
- **`DYNAMICPATH`** `1` — note the **uppercase, singular** spelling. "Recut the navmesh when this moves or disappears." Shipped on `zm_giant`'s debris/doors and the MP bomb-site prefabs.

Across every entity in the shipped `map_source/` that carries either, **not one carries both**. Obstacle versus floor. A moving platform does **not** want `DYNAMICPATH` — it reconnects paths unnecessarily.

**Verified in-game — the one remedy the community offers:** ticking `DYNAMICPATH` on a bus's `moving_platform_enabled` brush made zombies stop seeing a player aboard *entirely* — strictly worse than without it, and removing it restored the previous behaviour. The advice to pair the two comes from a Discord thread that is itself unresolved (t7kb, 0.25); treat it as ruled out rather than untried.

Two spelling traps that cost real time: the KVP is `DYNAMICPATH`, so a case-sensitive search for `dynamicpaths` finds nothing and you conclude it doesn't exist. And `moving_platform` (no `_enabled`) exists too — but as a **value** (`vehicletype`/`targetname`), not a key.

## Two stock vehicle defaults fight a platform

Radiant sets **`script_badplace`** and **`script_disconnectpaths`** on every `script_vehicle` by default, and both actively sabotage a platform zombies must reach — both gate on `isdefined()`, so neither opts out just by setting it to `0`, and **omitting `script_disconnectpaths` doesn't opt out either** (the default disconnects). Kill both from script, using the `endon`s Treyarch built in: `self notify( "kill_badplace_forever" )` / `self notify( "kill_disconnect_paths_forever" )`. Full KVP semantics and BO2's surgical alternative (no `script_badplace`/`script_disconnectpaths` at all) are in `references/route-loops-and-assets.md`.

## `#using_animtree` needs a zone entry, or the server dies with no error

**Verified from `console_mp.log`.** Adding `#using_animtree("generic")` + `UseAnimTree(#animtree)` (both required by the stock platform setup, via the vehicle blackboard) without the matching zone line kills the server during load: black screen, `Com_ERROR: EXE_ERR_SERVER_TIMEOUT`, and **not one line of your script's output** — the VM dies before reaching any print.

```
rawfile,animtrees/generic.atr
```

The link succeeds regardless, so a green build proves nothing here — same class of trap as an unknown function name, which also links clean and only fails at runtime. Zone a vehicle asset as `vehicle,<name>` plus its `xmodel,<name>`.

The diagnostic that actually works: if a feature script produces **zero** prints, it isn't a logic bug — the VM never got there. Grep `console_mp.log` for your prefix before touching the code.

## The shipped platform assets are collmap-only and `type=plane`

If a community prefab pack put `t7_moving_platforms.gdt` in your `source_data/`, the ready-made `moving_platform_32x32`/`_64x64`/`_128x128` and `hovering_platform_128x128` assets need no APE authoring for a first pass — but they're **`type=plane`** (never a ground vehicle, which is why `DrivePath` misbehaves so badly on them) with a **collision-only** xmodel, so the platform you stand on renders invisible. Sizing one to a real bus, and the `vehicletype` naming trap in the prefab that ships with them, is in `references/route-loops-and-assets.md`.

## Traversals can't move

BO3 exposes only `LinkTraversal( <node> )` and `UnlinkTraversal( <node> )` — *"Creates / Destroys a user edge connecting two path nodes"*. You can enable and disable a traversal; there is no API to reposition one, and the geometry is compiled. Community reports add that a traversal needs a static brush underneath to work at all.

So a traversal on a moving carrier is a dead end, and BO2's bus uses none: boarding is `linkto` plus scripted jump anims. Treat the Origins tank's mantle traversal as a stationary-only mechanism unless you've confirmed otherwise.

## Don't invent

The KVP spellings, notify names, and function semantics above were read off the shipped install (`share/raw/scripts/shared/vehicle_shared.gsc`, `animation_shared.gsc`, `vehicleriders_shared.gsc`, `docs_modtools/bo3_scriptapifunctions.htm`, `bin/default_navmesh_settings.json`, `map_source/_prefabs/`) or observed in a real session, and the items marked *verified* were reproduced in-game. Everything else about vehicles is easy to guess wrong because the community prefabs disagree with the shipped code — check `vehicle_shared.gsc` and the API docs before asserting a flag, notify, or KVP exists, and prefer what BO2's bus and `zm_tomb_tank` actually do over what a prefab pack does.
