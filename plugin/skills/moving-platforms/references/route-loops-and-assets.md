Deep-dive detail for **bo3-moving-platforms** (`plugin/skills/moving-platforms/SKILL.md`) — the full mechanics behind points that skill only summarizes. Read the parent skill first; come here for the working code, the measured geometry, and the shipped GDT names.

Contents:

- [A closed node cycle hangs the server](#a-closed-node-cycle-hangs-the-server)
- [Two stock vehicle defaults that fight a platform](#two-stock-vehicle-defaults-that-fight-a-platform)
- [The shipped platform assets are collmap-only and `type=plane`](#the-shipped-platform-assets-are-collmap-only-and-typeplane)
- [Getting a use prompt to actually appear](#getting-a-use-prompt-to-actually-appear)
- [The parked-case navmesh cutter (Origins tank)](#the-parked-case-navmesh-cutter-origins-tank)
- [When an anim's destination isn't a tag](#when-an-anims-destination-isnt-a-tag)

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

## Two stock vehicle defaults that fight a platform

Radiant sets both by default on a `script_vehicle`, and both actively sabotage a platform zombies must reach. Neither is worth fighting through KVPs, because KVP values arrive as strings — kill them from script instead, using the `endon`s Treyarch built in:

```gsc
self notify( "kill_badplace_forever" );
self notify( "kill_disconnect_paths_forever" );
```

- **`script_badplace`** — threads `_vehicle_bad_place()`, whose own Treyarch comment is *"make ai run way from vehicle"*. It paints badplace volumes 200–500 units ahead depending on speed, which AI avoid. The trigger is `isdefined()`, so setting it to `0` still starts the thread.
- **`script_disconnectpaths`** — becomes `vehicle.disconnectPathOnStop`. When velocity drops to ~0 it disconnects the navmesh around the vehicle and reconnects on movement. The opt-out is `isdefined(...) && !...`, so **omitting the KVP does not opt out** — the default disconnects.

BO2's bus carried neither, and cut paths surgically instead: explicit `disconnectpaths()` / `connectpaths()` calls on dedicated blocker brushes (its cow-catcher blocker, its path blockers), never on the bus.

## The shipped platform assets are collmap-only and `type=plane`

If a community prefab pack put `t7_moving_platforms.gdt` in your `source_data/`, you get `moving_platform_32x32` / `_64x64` / `_128x128` and `hovering_platform_128x128` ready to use — no APE authoring for a first pass. Three things to know:

- Their `type` is **`plane`** (the hovering variant is `helicopter`). None is a ground vehicle, which is exactly why `DrivePath` misbehaves so badly with them.
- Their xmodel is **collision only** — a `CollisionMap` pointing at a `clip_full` brush in `share/raw/collmaps/`. It renders nothing, so the platform you stand on is invisible. That's the same technique BO2 used for the bus, and why the bus needed no clip brushes.
- The prefab that ships with them may declare `vehicletype` `moving_platform` — a name **not defined** in the GDT. Use a dimensioned variant.

To size one to a real bus, duplicate a GDT entry and swap its `CollisionMap` for a bus-shaped collmap rather than authoring a vehicle from scratch.

## Getting a use prompt to actually appear

All three points below are **measured**, on top of shaping the trigger volumes themselves (parent skill).

- `SetInvisibleToAll()` does **not** come back with a later `SetVisibleToAll()` — once hidden it stays hidden.
- `TriggerEnable` does **not** re-evaluate the prompt of a player already standing inside the volume, so the hint only returns after leaving and re-entering.
- The shipped form is per-player and per-frame — `blockertrigger_update_prompt` is one `SetInvisibleToPlayer( player, !can_use )` plus one `SetHintString`, re-run for every player on every update.

Note also that stock never gates a barrier prompt on whether boards are missing (`blockerstub_update_prompt` only asks whether the *player* can use it); "nothing left to repair" is handled by **deleting** the trigger, not by a prompt state.

## The parked-case navmesh cutter (Origins tank)

For the *parked* case, the Origins tank does connect the ground navmesh to a mover — not by extending its own island, but by cutting the ground. It carries a linked, `notsolid` `navmesh_cutter` entity (`enablelinkto()` + `linkto`) that `disconnectpaths()` when it stops and `connectpaths()` when it leaves, so AI can't walk through where the hull now is. (From the decompiled `zm_tomb_tank.gsc` in t7kb, reliability 0.95 — Zombies Chronicles scripts aren't in a raw mod-tools install.)

## When an anim's destination isn't a tag

The parent skill covers the normal case — an anim that ends on a tag, where `GetStartOrigin`/`GetStartAngles` hand you the boarding spot directly. A port whose joints sit outside the vehicle (BO2's window anims are anchored on the window plane) has no tag to land on, so the destination has to be derived instead: read the travel with `GetMoveDelta( anim, 0, 1, self )` and project it on the parent's frame **at the moment of the move**, after `StopAnimScripted`. Deriving it this way beats measuring a constant per animation, because a re-export then carries its own arrival with it.
