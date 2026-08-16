---
name: bo3-moving-platforms
description: How to build something in Black Ops 3 zombies that moves and carries things — buses, trains, elevators, tanks, moving platforms — using a `script_vehicle` on `info_vehicle_node` paths plus a `moving_platform_enabled` `script_brushmodel`, and how to make zbarriers, clips, triggers and zombies ride along with `LinkTo`. Covers the `AttachPath`-vs-`DrivePath` split, the `moving_platform_enabled` / `DYNAMICPATH` / `script_disconnectpaths` / `script_badplace` KVP semantics, and why AI pathing on a mover only works on the linked brushmodel. Use when a platform drifts instead of following its nodes, teleports or freezes at the end of its path, sinks into the floor, when zombies freeze or stop seeing the player the moment someone stands on a moving surface, when a boarding zombie plays its climb animation perfectly and then snaps back outside, when a scripted animation on a mover stutters, drifts behind it, or ends in the wrong place, when riders aboard hold a valid accepted goal and still refuse to walk, when the console floods with navmesh errors around a platform, when a use prompt on a mover only appears after walking away and coming back or vanishes between two neighbouring triggers, when a vehicle path hangs the server at load (`EXE_ERR_SERVER_TIMEOUT`, black screen), or when porting TranZit's bus / a train / an elevator. Distinct from bo3-zombies-ai (navmesh, spawners and traversals on static geometry) and bo3-mapping (brushwork and sealing) — this is the moving-carrier craft and its silent failures.
---

# Moving carriers in BO3 zombies

A mover is **not one entity** — it's a `script_vehicle` that follows a path, a collision volume you stand on, and a separate `moving_platform_enabled` `script_brushmodel` that is the only thing AI can walk on. Almost every bug in this craft comes from expecting one object to do all three jobs. Look exact KVP spellings and asset names up in **t7kb** (`search` then `get`) and confirm them against the raw install; this skill is the division of labour and the traps around it.

For AI that never leaves the ground see **bo3-zombies-ai**; for the brushwork itself see **bo3-mapping**; for reading the resulting build errors see **bo3-debugging**.

## The three parts, and why one entity can't do all of it

| Part | Entity | Provides |
|---|---|---|
| the mover | `script_vehicle` with `vehicletype` | follows the `info_vehicle_node` path |
| collision | the vehicle's xmodel, usually a **collmap** | what players physically stand on |
| AI surface | `script_brushmodel` with `moving_platform_enabled` `1`, linked to the vehicle | the navmesh zombies walk on |

The pairing is by **`script_string`**, not `target`: the vehicle's `script_string` must equal the brushmodel's `targetname`. `target` is reserved for the path's first node. The stock template does `brushes = GetEntArray(self.script_string, "targetname")` then `brush EnableLinkTo(); brush LinkTo(self, "tag_origin")` — note `tag_origin` with no offset **teleports** the brush onto the vehicle, which is usually what you want since it auto-aligns the AI surface to the collision.

## `AttachPath` + `StartPath`, never `DrivePath`

Symptom: the vehicle drifts off, runs in a straight line forever, wanders in circles, or cuts straight to the last node and parks there.

Cause is in the shipped docs: `DrivePath( [node index] , [allow free drive] )` — *"Starts the vehicle driving this path and uses the vehicle physics, **not locked to the spline**"*. Its first argument is a node **index**, not a start node. On a `type=plane` asset (see below) "not locked to the spline" means it flies *at* the path rather than following it.

Use the spline-locked pair instead:

```gsc
self.drivepath = 0;
self AttachPath( n_start );      // AttachPath( <node> ) - "Attaches this vehicle to the given path"
self StartPath();
self SetSpeed( speed, accel );
```

`vehicle_shared.gsc` picks between them on a flag — `if ( IS_TRUE( self.drivepath ) ) DrivePath(...) else StartPath()` — and BO2's bus set `self.drivepath = 0` explicitly. **Both BO2's TranZit bus (`attachpath` + `startpath`) and BO3's Origins tank (`attachpath`) use the locked pair**; only community platform prefabs reach for `DrivePath`, which is why copying one leads you astray.

## A closed node cycle hangs the server

**Verified the hard way in a real session.** Making the last `info_vehicle_node` `target` the first one — the obvious way to loop a route — produces a black screen at load and `Com_ERROR: EXE_ERR_SERVER_TIMEOUT`, with **no script output at all** in `console_mp.log`. With `SPLINE_NODE` set it is reliably fatal.

So a BO3 vehicle path must be an **open chain**. To loop a route, keep the chain open and catch the engine's own path-end notify (`vehicle_shared.gsc`):

```gsc
while ( true )
{
    self AttachPath( n_start );
    self StartPath();
    self SetSpeed( speed, accel );

    self waittill( "reached_end_node" );

    self SetSpeed( 0, accel );      // no path left: a type=plane vehicle free-drifts otherwise
    WAIT_SERVER_FRAME;              // never wait 0 here - see below
}
```

**Lay the route out so the relay is invisible. Verified working geometry:** run the final leg *through* the start node and put the end node a little way **past** it, on the same heading. `AttachPath` snaps the vehicle onto the spline, so the re-attach has to happen where the vehicle already is — overshooting the start gives `reached_end_node` room to fire while the vehicle is passing over it, and the snap is then zero-distance. No stop, no teleport, it just carries on. A square route that starts at `(0 -652)` and comes back down the same axis ends its last node at `(0 -796)`: 144 units past the start, never actually reached.

Two more things that bite while building the route:

- **An end node on the vehicle's *spawn* position is degenerate**: `reached_end_node` fires at load, the loop restarts forever, and the vehicle only pivots without travelling.
- **`wait 0` does not yield a frame in GSC.** A dwell of zero in that loop spins without releasing the VM and freezes the game. Use `WAIT_SERVER_FRAME` (`shared.gsh`) unconditionally, then any real dwell on top. BO2's per-node loop uses `waittillframeend` for the same reason.

`SPLINE_NODE` `1` belongs on every node once you're on `AttachPath` — it's what smooths the corners. Only turn it off to isolate a problem, and remember that spline plus a *closed* chain is the fatal combination, not spline itself.

BO2 could afford a cyclic route because its `follow_path()` waits on `reached_node` **per node** and simply never exits while `nextpoint` stays defined — don't port that shape to BO3.

## Zombies path on the linked brushmodel, never on the vehicle

**Verified in-game with `ai_showNavMesh 1`:** both the vehicle and the linked brushmodel generate a navmesh, but zombies only ever move on the **brushmodel's**. The vehicle's is inert. This works while the platform is moving.

So the `moving_platform_enabled` brush is the load-bearing piece — deleting it to "simplify" removes AI from the mover entirely. The collmap is the more dispensable of the two, since a solid brushmodel already collides.

Symptom that trips people first: **the moment a player stands on a `moving_platform_enabled` brush, every zombie freezes**. That is a known, unresolved BO3 behaviour with the plain `script_brushmodel` + `MoveTo` approach; driving a real `script_vehicle` instead is what fixes it.

**Nothing connects the ground navmesh to the mover's**, and nothing ever will — a moving surface can't be stitched into baked navmesh. Zombies therefore can't *walk on*; they need a scripted boarding step (next section). What they can do, once aboard, is path around on the linked brush.

Three consequences of that island being separate:

- **`GetClosestPointOnNavMesh` answers from the static mesh only** (**measured**). It cannot see the mover's island, so using it to validate or snap a goal on a deck rejects perfectly good points. Don't gate a rider's goal on it.
- **Overlapping `moving_platform_enabled` brushes generate competing meshes** (**measured**), and the console fills with navmesh errors at over a hundred a second. Where the mover's walkable surface is one volume, model it as **one convex brush** — the flood stops dead.
- **The mesh has to fit an AI.** `bin/default_navmesh_settings.json` is the generator's own contract: `characterHeight 72`, `minCharacterWidth 31`, `simplification.minCorridorWidth 30`, `maxStepHeight 18`. A gangway or a bus aisle narrower than ~31 units silently produces no mesh at all, which reads as "the platform doesn't work" rather than "the corridor is too tight".

**Negative result, worth having before you attempt it: Treyarch never pathfinds an AI on a moving vehicle.** `vehicleriders_shared` puts every rider in `PathMode( "dont move" )` for the whole ride, and nothing in the shipped scripts walks an actor around on a mover in motion. On a real port, riders holding a valid accepted goal on a moving deck frequently just stood still, with every readable state healthy (`pathmode move allowed`, `scripted 0`, `enemy 1`, goal accepted, `ignoreall 0`) and no difference when the vehicle stopped. Budget for the tank's model instead — park the rider (`setgoalpos( self.origin )` with a generous radius) and script any movement between authored, vehicle-local points — rather than for making free pathing work.

For the *parked* case the tank does connect the two worlds, but by cutting the ground rather than extending itself: it carries a linked, `notsolid` `navmesh_cutter` entity (`enablelinkto()` + `linkto`) that `disconnectpaths()` when it stops and `connectpaths()` when it leaves, so AI can't walk through where the hull now is. (From the decompiled `zm_tomb_tank.gsc` in t7kb, reliability 0.95 — Zombies Chronicles scripts aren't in a raw mod-tools install.)

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

Everything downstream follows from that: the tear/climb anims are `animscripted` anchored on a `gettagorigin( self.attachtag )` lookup, with `animmode( "noclip" )` while passing through geometry and `animmode( "gravity" )` restored after. The handoff back to navmesh is `unlink()` then a fresh goal. BO2 re-reads that tag on **every loop iteration** — do not port that part. In BO3 the link is already what holds the anim in the moving frame, and re-anchoring on top of it stutters (see the two anim sections below).

**A `zbarrier` really does survive `LinkTo`** — boards and all, `SetMovingPlatformEnabled(1)` on the barrier plus a linked parent. **Verified in-game.** Community reports claiming zbarriers can't be moved because their state models carry their own origins are wrong; don't rebuild barriers out of `script_model`s on that basis.

**Debugging a link: measure the offset in the parent's local space, not world space.** `WorldToLocalCoords` is rotation-invariant; a world-space delta also changes when the parent merely turns, which reports a false "drift" on every corner. (Cost me a false alarm on a link that was working fine.)

Two more link facts, both **measured**, both cheap to lose a session to. **Re-issuing `LinkTo` on an entity the engine already considers linked does nothing** — a second call with a different offset is silently ignored, so a re-anchor has to `Unlink()` first. And **a `LinkTo` does not move the entity until the next server tick**, so anything sampled on the same frame reads the *pre-link* placement; a probe that skips a `WAIT_SERVER_FRAME` reports the link's own work as drift.

## Don't hand-roll the anchor — `animation::play( anim, mover, tag )` already is the boarding sequence

Reaching for `AnimScripted` directly on a mover reinvents `animation_shared`, and reinventing it is how the traps below get discovered one at a time. `animation::play( animation, <entity>, <tag> )` takes an **entity and a tag** rather than a transform, and `animation_shared::_play` is worth reading once because it *is* the correct shape:

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

**Measured, at the cost of an afternoon:** re-reading the tag every server frame and resuming the anim through `AnimScripted`'s `animationTime` argument *does* hold the alignment — and it visibly stutters, because correcting an anchor in a loop looks exactly like correcting an anchor in a loop. When an anim drifts off the back of a mover, the missing piece is the **link**, not a finer correction. The one shipped case that re-anchors per frame (this repo's zipline, **bo3-scripting**) has no parent to link to; a mover always does.

Two shipped details that save a false trail. Treyarch's own comment on that teleport-then-link — *"LinkTo was not working correctly with animation and always positioning the object as if it was linked to the tag_origin. Moving the object to the tag position fixes this."* — so the `ForceTeleport` onto the tag **before** linking is deliberate, not redundant. And `waittillmatch( animation, "end" )` needs the anim to carry an `end` notetrack: a ported anim that lost its notetracks hangs there forever, and `wait GetAnimLength( anim )` is the fallback (**bo3-scripting** owns that trap).

BO3 also ships the full AI-boards-a-vehicle flow around it, in `vehicleriders_shared.gsc`: `animation::reach` walks the AI to the start, `animation::play` runs the enter anim on the align tag, a threaded `RideAnim` plays with unlink disabled, and the rider gets `PathMode( "dont move" )` for the ride. Read it before designing a boarding sequence from scratch.

## An anim never moves a linked entity — and `AnimScripted`'s `origin` is where it STARTS

Two independent facts, and confusing either one produces an animation that looks perfect and lands in the wrong place.

**Measured:** sampling a boarding zombie's position *in the vehicle's own frame* for the whole climb-in anim reported a **peak displacement of 0** while the body was plainly climbing through the window and ending up inside. `AnimScripted` parks the entity on the anchor it is handed; the **rendered pose** follows that anchor, the **entity** follows its link, and the two are independent. Nothing the animation does will ever relocate a linked actor — which is also why a stale anchor "leaves the zombie behind" visually while its collision never moved at all.

And the anchor is the **start**, not an origin for the animation's coordinate space — the shipped docs for its siblings say so plainly (`GetStartOrigin`: *"Get the starting origin for an animation, in world coordinates, given its current position, and angles"*). The end is therefore only ever start-plus-travel. **Corollary that costs a day if you miss it: translating an export is a no-op for playback.** Shifting every `OFFSET` so the clip "ends at (0,0,0)" changes the travel by nothing, so it changes where the anim plays by nothing — see **bo3-animation**.

So the destination is computed, never authored into the file:

- **The anim ends on a tag** (the normal case, and why the tank needs no fixup — *its tags are the destinations*): pass the tag transform to `GetStartOrigin( v_org, v_ang, anim )` / `GetStartAngles(...)` and you get where the entity must begin for the clip to land there. `vehicleriders_shared::can_get_in` is the canonical use — tag in, boarding spot out, then `FindPath` to it; `animation::reach` `force_goal`s to the same point.
- **The anim ends somewhere else** (a port whose joints sit outside the vehicle — BO2's window anims are anchored on the window plane): read the travel with `GetMoveDelta( anim, 0, 1, self )` and project it on the parent's frame **at the moment of the move**, after `StopAnimScripted`. Deriving it beats measuring a constant per animation, because a re-export then carries its own arrival.

Either way this is the *destination*, not the anchor — and unlike the anchor it must be composed **at the moment of the move**, because by then the mover has travelled. Composing it before a 3.5-second anim cost a 325-unit miss: a second of animation is ~110 units of bus, plus its rotation. `zm_tomb_tank::tank_get_jump_down_offset` builds its arrival the same way, from a `tank_offset` field authored on a map struct.

## A use prompt on a mover has to be a map-placed trigger

**A player holds one use trigger at a time**, so two overlapping volumes cancel and someone standing between two of them gets **no prompt at all**. BO3's answer for barriers is a **per-player** trigger — `zm_unitrigger::unitrigger_force_per_player_triggers( stub, true )` in `_zm_blockers.gsc` — which is exactly why its shipped barrier radius (`94.21`) can be generous without neighbouring barriers fighting. That escape is closed on a mover: unitriggers register at a fixed origin (`register_static_unitrigger`) and there is nothing in `_zm_unitrigger.gsc` to follow a parent.

Which leaves shaping the volumes so they never touch — Radiant's job, not a radius guessed in script. BO2 does precisely that: its bus rebuild trigger is **placed in the map** and picked up by `script_noteworthy` `"rebuild"`, then wired with `enablelinkto()` / `linkto( bus )` / `setmovingplatformenabled( 1 )`. Note the **bare** `linkto` with no tag and no offsets, which is what preserves the transform the mapper authored.

Getting the prompt to appear is its own trap, all three **measured**: `SetInvisibleToAll()` does **not** come back with a later `SetVisibleToAll()` — once hidden it stays hidden; `TriggerEnable` does **not** re-evaluate the prompt of a player already standing inside the volume, so the hint only returns after leaving and re-entering. The shipped form is per-player and per-frame — `blockertrigger_update_prompt` is one `SetInvisibleToPlayer( player, !can_use )` plus one `SetHintString`, re-run for every player on every update. Note also that stock never gates a barrier prompt on whether boards are missing (`blockerstub_update_prompt` only asks whether the *player* can use it); "nothing left to repair" is handled by **deleting** the trigger.

## `moving_platform_enabled` and `DYNAMICPATH` are different jobs — they never co-occur

Both are real, both are engine-side, and mixing them up is easy because both sound like "make pathing work on this".

- **`moving_platform_enabled`** `1` — on a `script_brushmodel`/`script_model`. "AI can stand on this and be carried." Shipped on Treyarch's own elevator prefabs and MP traversal prefabs.
- **`DYNAMICPATH`** `1` — note the **uppercase, singular** spelling. "Recut the navmesh when this moves or disappears." Shipped on `zm_giant`'s debris/doors and the MP bomb-site prefabs.

Across every entity in the shipped `map_source/` that carries either, **not one carries both**. Obstacle versus floor. A moving platform does **not** want `DYNAMICPATH` — it reconnects paths unnecessarily.

**Verified in-game, and worth knowing because it is the one remedy the community offers:** ticking `DYNAMICPATH` on a bus's `moving_platform_enabled` brush made zombies stop seeing a player aboard *entirely* — strictly worse than without it, and removing it restored the previous behaviour. The advice to pair the two comes from a Discord thread that is itself unresolved (t7kb, 0.25); treat it as ruled out rather than untried.

Two spelling traps that cost real time: the KVP is `DYNAMICPATH`, so a case-sensitive search for `dynamicpaths` finds nothing and you conclude it doesn't exist. And `moving_platform` (no `_enabled`) exists too — but as a **value** (`vehicletype`/`targetname`), not a key.

## The two stock vehicle behaviours that fight a platform

Radiant sets both by default on a `script_vehicle`, and both actively sabotage a platform zombies must reach. Neither is worth fighting through KVPs, because KVP values arrive as strings — kill them from script instead, using the `endon`s Treyarch built in:

```gsc
self notify( "kill_badplace_forever" );
self notify( "kill_disconnect_paths_forever" );
```

- **`script_badplace`** — threads `_vehicle_bad_place()`, whose own Treyarch comment is *"make ai run way from vehicle"*. It paints badplace volumes 200–500 units ahead depending on speed, which AI avoid. The trigger is `isdefined()`, so setting it to `0` still starts the thread.
- **`script_disconnectpaths`** — becomes `vehicle.disconnectPathOnStop`. When velocity drops to ~0 it disconnects the navmesh around the vehicle and reconnects on movement. The opt-out is `isdefined(...) && !...`, so **omitting the KVP does not opt out** — the default disconnects.

BO2's bus carried neither, and cut paths surgically instead: explicit `disconnectpaths()` / `connectpaths()` calls on dedicated blocker brushes (its cow-catcher blocker, its path blockers), never on the bus.

## `#using_animtree` needs a zone entry, or the server dies with no error

**Verified from `console_mp.log`.** Adding `#using_animtree("generic")` + `UseAnimTree(#animtree)` (both required by the stock platform setup, via the vehicle blackboard) without the matching zone line kills the server during load: black screen, `Com_ERROR: EXE_ERR_SERVER_TIMEOUT`, and **not one line of your script's output** — the VM dies before reaching any print.

```
rawfile,animtrees/generic.atr
```

The link succeeds regardless, so a green build proves nothing here — same class of trap as an unknown function name, which also links clean and only fails at runtime. Zone a vehicle asset as `vehicle,<name>` plus its `xmodel,<name>`.

The diagnostic that actually works: if a feature script produces **zero** prints, it isn't a logic bug — the VM never got there. Grep `console_mp.log` for your prefix before touching the code.

## The shipped platform assets are collmap-only and `type=plane`

If a community prefab pack put `t7_moving_platforms.gdt` in your `source_data/`, you get `moving_platform_32x32` / `_64x64` / `_128x128` and `hovering_platform_128x128` ready to use — no APE authoring for a first pass. Three things to know:

- Their `type` is **`plane`** (the hovering variant is `helicopter`). None is a ground vehicle, which is exactly why `DrivePath` misbehaves so badly with them.
- Their xmodel is **collision only** — a `CollisionMap` pointing at a `clip_full` brush in `share/raw/collmaps/`. It renders nothing, so the platform you stand on is invisible. That's the same technique BO2 used for the bus, and why the bus needed no clip brushes.
- The prefab that ships with them may declare `vehicletype` `moving_platform` — a name **not defined** in the GDT. Use a dimensioned variant.

To size one to a real bus, duplicate a GDT entry and swap its `CollisionMap` for a bus-shaped collmap rather than authoring a vehicle from scratch.

## Traversals can't move

BO3 exposes only `LinkTraversal( <node> )` and `UnlinkTraversal( <node> )` — *"Creates / Destroys a user edge connecting two path nodes"*. You can enable and disable a traversal; there is no API to reposition one, and the geometry is compiled. Community reports add that a traversal needs a static brush underneath to work at all.

So a traversal on a moving carrier is a dead end, and BO2's bus uses none: boarding is `linkto` plus scripted jump anims. Treat the Origins tank's mantle traversal as a stationary-only mechanism unless you've confirmed otherwise.

## Don't invent

The KVP spellings, notify names, and function semantics above were read off the shipped install (`share/raw/scripts/shared/vehicle_shared.gsc`, `animation_shared.gsc`, `vehicleriders_shared.gsc`, `docs_modtools/bo3_scriptapifunctions.htm`, `bin/default_navmesh_settings.json`, `map_source/_prefabs/`) or observed in a real session, and the items marked *verified* were reproduced in-game. Everything else about vehicles is easy to guess wrong because the community prefabs disagree with the shipped code — check `vehicle_shared.gsc` and the API docs before asserting a flag, notify, or KVP exists, and prefer what BO2's bus and `zm_tomb_tank` actually do over what a prefab pack does.
