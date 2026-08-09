---
name: bo3-moving-platforms
description: How to build something in Black Ops 3 zombies that moves and carries things — buses, trains, elevators, tanks, moving platforms — using a `script_vehicle` on `info_vehicle_node` paths plus a `moving_platform_enabled` `script_brushmodel`, and how to make zbarriers, clips, triggers and zombies ride along with `LinkTo`. Covers the `AttachPath`-vs-`DrivePath` split, the `moving_platform_enabled` / `DYNAMICPATH` / `script_disconnectpaths` / `script_badplace` KVP semantics, and why AI pathing on a mover only works on the linked brushmodel. Use when a platform drifts instead of following its nodes, teleports or freezes at the end of its path, sinks into the floor, when zombies freeze or stop seeing the player the moment someone stands on a moving surface, when a boarding zombie plays its climb animation perfectly and then snaps back outside, when a use prompt on a mover only appears after walking away and coming back or vanishes between two neighbouring triggers, when a vehicle path hangs the server at load (`EXE_ERR_SERVER_TIMEOUT`, black screen), or when porting TranZit's bus / a train / an elevator. Distinct from bo3-zombies-ai (navmesh, spawners and traversals on static geometry) and bo3-mapping (brushwork and sealing) — this is the moving-carrier craft and its silent failures.
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

**Nothing connects the ground navmesh to the mover's**, and nothing ever will — a moving surface can't be stitched into baked navmesh. Zombies therefore can't *walk on*; they need a scripted boarding step (next section). What they can do, once aboard, is path around on the linked brush.

Symptom that trips people first: **the moment a player stands on a `moving_platform_enabled` brush, every zombie freezes**. That is a known, unresolved BO3 behaviour with the plain `script_brushmodel` + `MoveTo` approach; driving a real `script_vehicle` instead is what fixes it.

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

Everything downstream follows from that: the tear/climb anims are `animscripted` anchored on a **live tag lookup** (`self.attachent gettagorigin( self.attachtag )`, re-read every loop iteration) so they play correctly in a moving frame, with `animmode( "noclip" )` while passing through geometry and `animmode( "gravity" )` restored after. The handoff back to navmesh is `unlink()` then a fresh goal.

**A `zbarrier` really does survive `LinkTo`** — boards and all, `SetMovingPlatformEnabled(1)` on the barrier plus a linked parent. **Verified in-game.** Community reports claiming zbarriers can't be moved because their state models carry their own origins are wrong; don't rebuild barriers out of `script_model`s on that basis.

**Debugging a link: measure the offset in the parent's local space, not world space.** `WorldToLocalCoords` is rotation-invariant; a world-space delta also changes when the parent merely turns, which reports a false "drift" on every corner. (Cost me a false alarm on a link that was working fine.)

Two more link facts, both **measured**, both cheap to lose a session to. **Re-issuing `LinkTo` on an entity the engine already considers linked does nothing** — a second call with a different offset is silently ignored, so a re-anchor has to `Unlink()` first. And **a `LinkTo` does not move the entity until the next server tick**, so anything sampled on the same frame reads the *pre-link* placement; a probe that skips a `WAIT_SERVER_FRAME` reports the link's own work as drift.

## An anim never moves a linked entity — the anchor holds it and the travel is in the bones

The trap that eats a port of BO2's boarding sequence, and it is invisible because the animation looks perfect.

**Measured:** sampling a boarding zombie's position *in the vehicle's own frame* for the whole climb-in anim reported a **peak displacement of 0** while the body was plainly climbing through the window and ending up inside. `AnimScripted` parks the entity on the anchor it is handed; the **rendered pose** follows that anchor, the **entity** follows its link, and the two are independent. Nothing the animation does will ever relocate a linked actor — which is also why a stale anchor "leaves the zombie behind" visually while its collision never moved at all.

So on a mover the relocation is explicit, it happens **after** `StopAnimScripted`, and the destination is a **local offset projected on the parent's frame, re-read at the moment of the move**. Composing the world point up front instead cost a 325-unit miss: a second of animation is ~110 units of bus, plus its rotation.

```gsc
v_org = vehicle GetTagOrigin( str_tag );          // read HERE, not before the anim
v_ang = vehicle GetTagAngles( str_tag );
v_dest = v_org + VectorScale( AnglesToForward( v_ang ), v_delta[0] )
               + VectorScale( AnglesToRight( v_ang ),   v_delta[1] )
               + VectorScale( AnglesToUp( v_ang ),      v_delta[2] );
```

That is the shape Origins' tank uses for its own arrival points — `zm_tomb_tank::tank_get_jump_down_offset` composes exactly this from a `tank_offset` field **authored on a map struct**. Which is the real lesson: the tank never needs a fixup because **its tags are the destinations**. `climb_tag` is five lines — `linkto` / `animscripted` / `donotetracks` / `unlink` / `setgoalpos( self.origin )` — with no `AnimMode`, `OrientMode`, `PathMode` or teleport anywhere in the file; the link does the moving and the anim only dresses it. Ported anims whose tags land *outside* the vehicle (BO2's window joints sit on the window plane) have to build the destination, and `GetMoveDelta` on an un-flattened twin export is where the reach comes from when the played anim's root was pinned flat to preserve its lateral anchoring.

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

The KVP spellings, notify names, and function semantics above were read off the shipped install (`share/raw/scripts/shared/vehicle_shared.gsc`, `docs_modtools/bo3_scriptapifunctions.htm`, `map_source/_prefabs/`) or observed in a real session, and the items marked *verified* were reproduced in-game. Everything else about vehicles is easy to guess wrong because the community prefabs disagree with the shipped code — check `vehicle_shared.gsc` and the API docs before asserting a flag, notify, or KVP exists, and prefer what BO2's bus and `zm_tomb_tank` actually do over what a prefab pack does.
