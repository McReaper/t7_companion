Deep-dive detail for **t7kb:moving-platforms** (`plugin/skills/moving-platforms/SKILL.md`) — the full mechanics behind points that skill only summarizes. Read the parent skill first; come here for the working code, the measured geometry, and the shipped GDT names.

Contents:

- [A closed node cycle hangs the server](#a-closed-node-cycle-hangs-the-server)
- [A node's `speed` reads back in inches per second](#a-nodes-speed-reads-back-in-inches-per-second)
- [Per-node stops: node KVPs only act inside `paths()`, which a zombies vehicle never runs](#per-node-stops-node-kvps-only-act-inside-paths-which-a-zombies-vehicle-never-runs)
- [Two stock vehicle defaults that fight a platform](#two-stock-vehicle-defaults-that-fight-a-platform)
- [The shipped platform assets are collmap-only and `type=plane`](#the-shipped-platform-assets-are-collmap-only-and-typeplane)
- [Getting a use prompt to actually appear](#getting-a-use-prompt-to-actually-appear)
- [The parked-case navmesh cutter (Origins tank)](#the-parked-case-navmesh-cutter-origins-tank)
- [When an anim's destination isn't a tag](#when-an-anims-destination-isnt-a-tag)
- [One use trigger at a time](#one-use-trigger-at-a-time)
- [Traversals can't move](#traversals-cant-move)

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

**Close the lap in space, not in the chain.** Put the end node **exactly on the start node**, arriving on the start's own heading: `AttachPath` snaps the vehicle onto the spline, so re-attaching where the vehicle already stands is a zero-distance snap. **Verified:** a terminus 480 units from the start teleported the bus at every lap; the same route with a sixth node coincident with `bus_start` reads 0 units and runs seamlessly.

**Keep every later leg off the spawn point.** An earlier version of this file recommended running the final leg *through* the start and ending it past it — a square from `(0 -652)` whose last node sat at `(0 -796)`. That puts the vehicle's spawn ON the last leg, and the next session found the vehicle starting 90° off, turned onto that leg's heading; the snap disappeared once no later leg touched the spawn. It also left the overshoot inside the look-ahead window (parent skill), so the two causes were tangled — but the end-on-the-start layout above has neither problem.

Two more things that bite while building the route:

- **An end node on the vehicle's *spawn* position is degenerate**: `reached_end_node` fires at load, the loop restarts forever, and the vehicle only pivots without travelling.
- **`wait 0` does not yield a frame in GSC.** A dwell of zero in that loop spins without releasing the VM and freezes the game. Use `WAIT_SERVER_FRAME` (`shared.gsh`) unconditionally, then any real dwell on top. BO2's per-node loop uses `waittillframeend` for the same reason.

`SPLINE_NODE` `1` smooths the corners and is safe on every node of an **open** chain — this port runs that way. It is not required, though: Treyarch's `template.map` sets it on 8 of its 102 vehicle nodes. Spline plus a *closed* chain is the fatal combination, not spline itself.

BO2 could afford a cyclic route because its `follow_path()` waits on `reached_node` **per node** and simply never exits while `nextpoint` stays defined — don't port that shape to BO3.

## A node's `speed` reads back in inches per second

The KVP is authored in **mph** (`bin/t7.def.json`), and `SetSpeed` takes mph — but `n_node.speed` read from script comes back already converted to **inches per second**. **Measured:** a node at `speed 19` logged `334`, which is 19 × 17.6. Passing it straight to `SetSpeed` asks for 334 mph, and only a `SetVehMaxSpeed` cap stops the vehicle bolting. Convert with the shipped constant, `MPH_TO_INCHES_PER_SEC` (`shared.gsh`, which `_amws.gsc` uses for the same conversion):

```gsc
n_mph = n_node.speed / MPH_TO_INCHES_PER_SEC;
```

No shipped script reads `.speed` off a vehicle node at all, so guard it with `isdefined` and a fallback, the way `_elevator.gsc` reads `path_point.speed`.

## Per-node stops: node KVPs only act inside `paths()`, which a zombies vehicle never runs

`vehicle_shared::paths()` implements stop-and-go natively off node KVPs — `script_wait` (`pause_path()` then a timed wait), `script_waittill`, `script_flag_wait`, `script_notify` (notifies the vehicle **and** `level`), and `script_noteworthy` `"brake"`/`"resumespeed"`. None of it runs for a map-placed zombies vehicle: `paths()` is threaded by `get_on_path()`, which `vehicle::init()` calls, and `vehicle::init` is called only from MP (`_globallogic_vehicle.gsc`) and gadget scripts. Author `script_wait` on a node in a zombies map and nothing happens.

The Origins tank shows the zombies shape: `attachpath` + `startpath` and **its own** `follow_path()`, which walks the chain on `waittill("reached_node", node)` and dispatches the node's KVPs itself. Do the same — a stop is then a `script_noteworthy` you test for, a `SetSpeed(0, …)`, a dwell, and a `SetSpeed` back up. Resist calling `get_on_path()` yourself to get the stock handling: it threads `paths()` onto a vehicle `init()` never prepared, which is a crash risk (inference — not tried). **Measured, too:** `isphysicsvehicle` is false on a map-placed `type "4 wheel"` bus, so the `SetBrake` that `get_on_path` issues for physics vehicles would not apply anyway.

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

## One use trigger at a time

**A player holds one use trigger at a time**, so two overlapping volumes cancel and someone standing between them gets **no prompt at all**. BO3's per-player escape for barriers — `zm_unitrigger::unitrigger_force_per_player_triggers` — doesn't reach a mover: unitriggers register at a fixed origin, and there is nothing in `_zm_unitrigger.gsc` to follow a parent.

So shape the volumes so they never touch — Radiant's job, not a radius guessed in script — the way BO2's bus rebuild trigger does: **placed in the map**, wired with `enablelinkto()` / `linkto( bus )` with no tag or offset (preserving the mapper's transform) / `setmovingplatformenabled( 1 )`.

Getting the prompt to actually appear is its own trap: `SetInvisibleToAll()` and `TriggerEnable` don't behave the way their names suggest for a player already standing inside the volume. The full three-part mechanism, plus how stock signals "nothing left to repair," is in `references/route-loops-and-assets.md`.

## Traversals can't move

BO3 exposes only `LinkTraversal( <node> )` and `UnlinkTraversal( <node> )` — *"Creates / Destroys a user edge connecting two path nodes"*. You can enable and disable a traversal; there is no API to reposition one, and the geometry is compiled. Community reports add that a traversal needs a static brush underneath to work at all.

So a traversal on a moving carrier is a dead end, and BO2's bus uses none: boarding is `linkto` plus scripted jump anims. Treat the Origins tank's mantle traversal as a stationary-only mechanism unless you've confirmed otherwise.
