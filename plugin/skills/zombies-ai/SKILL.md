---
name: zombies-ai
description: How to fix and build zombies/character AI in Black Ops 3 — when zombies spawn but stand still replaying an idle "dance" and never attack, a dog round never ends because nothing spawns, `assert fail` on a riser's `script_string`, zombies are blind to a player in plain sight, a scripted zombie blocks the player like invisible geometry, or one parks at melee range and neither swings nor closes. Covers behavior trees and anim tables (`.ai_bt`/`.ai_asm`/`.ai_am`/`.ai_ast`), spawners, risers and dog rounds, custom traversals, navmesh, custom zombie variants and AAT immunity, and the engine flags a scripted zombie must manage (`maxsightdistsqrd`, `SetPlayerCollision`, `ForceTeleport`, `PathMode`). In-game AI (zombie behavior, pathing, spawning), not LLM/agent AI. Distinct from t7kb:moving-platforms (AI riding or boarding a mover) and t7kb:mapping (brushwork, zones) — this is AI on static ground, navmesh included.
---

# Zombie / character AI in Black Ops 3

Custom AI behavior is driven by four file types working together, not one — look up exact function names/KVPs in **t7kb** (`t7kb:search` then `t7kb:get`), but the file relationship below is the part that trips people up.

## The four files that drive AI behavior

| File | Asset type (zone line) | Folder under `share/raw/` | Job |
|---|---|---|---|
| `.ai_bt` | `behaviortree` | `behavior/` | *what* the AI decides to do |
| `.ai_asm` | `animstatemachine` | `animstatemachines/` | states → substates → an `animation_selector` |
| `.ai_ast` | `animselectortable` | `animtables/` | resolves a selector, by blackboard attributes, to an `_animation_alias` |
| `.ai_am` | `animmappingtable` | `animtables/` | maps an alias to the actual xanim |

There is no `animstates/` folder — `.ai_ast` sits beside the `.ai_am` in `animtables/` (with `ast_definitions.json`), and it is a **selector table**, not "an anim state". ASM vs AST is the distinction that trips people: the state machine names the state, the selector table picks the animation for it. Adding a zombie variant or a custom traversal usually touches more than one of these — editing the behavior tree but not the matching table (or vice versa) is the classic half-fix — and a usermap override copy of any of them needs its own zone line of the type above (**t7kb:debugging** for the assetlist override).

## Behavior tree gotcha: your usermap copy can be stale, not the one from `share/raw`

If zombies spawn but just stand there replaying an idle animation ("the zombie dance"), the map GSC is usually missing a community-pack `#using` for its behavior module plus that module's scriptparsetree in the `.zone` — add both. Separately: when debugging a broken behavior (e.g. a ported special zombie not reacting to a weapon), don't assume the `.ai_bt`/`.ai_asm`/`.ai_am`/`.ai_ast` sitting in your usermap folder is correct — pull the canonical one from `share/raw` first; a stale usermap copy missing the checks for your case (e.g. a specific wonder weapon) is a frequent cause, and the fix is adding the missing checks to *all four* files, not just one.

## Custom traversals need a behavior tree entry — the animation existing isn't enough

A traversal animation being visible in a prefab does **not** mean AI will use it: the traversal must be added to the behavior tree, or the AI won't take it (this trips people up for non-standard AI like dogs, not just zombies). Procedural traversals (the AI measures distance and jumps to match) are a distinct mechanism from fixed custom traversals and need their own behavior tree setup. Traversal names follow a literal convention in the anim tables, e.g. `JUMP_UP_36_DOWN_52` — match the name to hook up the right entry.

## Spawning zombies: it's mostly Radiant KVPs, not a GSC call

The actual spawn recipe is entity setup, not scripting: a spawner entity `actor/spawner_zm_factory_zombie` with `script_noteworthy = zombie_spawner`, `count = 9999`, `coop_count = {9999,0,0,0}`, and `script_forcespawn = 1`. **Risers** are a separate entity — a `script/struct` with `targetname = <zone>_spawners`, `script_noteworthy = riser_location`, `script_string = find_flesh`. Fallers/crawlers and special types (margwa/nova/quad) have their own KVPs and devraw prefabs — check the prefab itself or t7kb if a type-specific KVP isn't in the usual list. On the script side, `zombie_utility::spawn_zombie(spawner, ...)` is the runtime call that actually spawns against a placed spawner; to run custom logic on every spawn **without forking anything**, hook it (`spawner::add_archetype_spawn_function("zombie", &my_spawn_function)`) — same hook-first philosophy as `t7kb:scripting`.

**Fresh maps ship with `dog_rounds_allowed` on but no dog-spawner prefab** — which produces an infinite dog round with nothing spawning. Fix by adding a `dog_spawner.map` (from The Giant) stamp plus a `<zone>_spawners`-named struct, or disable dog rounds with `level.dog_rounds_allowed = false` — set **before** `zm_usermap::main()`, which reads it through `DEFAULT(...)` and enables dog rounds on the spot (`zm_usermap.gsc:146-150`); set after, it changes nothing (**t7kb:scripting** has the before/after rule).

**Water risers** are `script_parameters "in_water"` on the riser; the splash FX is gated client-side by `level.use_water_risers` (`_zm.csc`), so without it they emerge from water throwing dirt.

**Zombies with no valid target just stand still** — a solo player downed, or anyone under Zombie Blood, leaves nothing for zombies to path toward. Fix with lightweight `script_noteworthy = zombie_poi` structs for a simple case, or the full `zm_giant_cleanup_mgr` hook (`enemy_location_override`/`no_target_override`) for `dog_location`-based relocation in a more involved one.

## A riser's `script_string` must be matched by an `exterior_goal`, or the AI throws at spawn

A riser's `script_string` is not decoration: `_zm_spawner`'s spawn path copies it onto the zombie as **`find_flesh_struct_string`**, and `findNodesService` in `_zm_behavior.gsc` then walks `level.exterior_goals` looking for one whose **`script_string` is equal**. `find_flesh` is the special value meaning "no entrance, just chase" — it returns early and is always safe. **Any other value must be declared by an `exterior_goal` struct somewhere**, or `node` stays undefined and you get, in this order:

```
assert fail: <the string it wanted>
undefined is not a field object
SetGoal() unsupported goal type
```

on every zombie that rises there — while zombies from a `find_flesh` riser in the same map behave perfectly, so it looks intermittent.

**The trap that produces this:** `script_string` set on the **prefab instance** (the `misc_prefab` entity) does **not** propagate to the entities inside it. A barrier prefab placed with `script_string "receiver_set_entry_a"` still has an `exterior_goal` carrying no `script_string` at all, so nothing declares that entry. Put the KVP on the `exterior_goal` **struct itself** — which for a stock prefab means taking a local copy of it rather than editing shared content. (verified in the install: Treyarch's `zm_core/barricade_reciever_wood.map` struct carries only `origin`/`target`/`targetname`, and nothing substitutes a prefab-level value in. The community recipe "give the prefab a `script_string`" produces this assert.)

Check for **more than one**: a map can have several risers asking for different entries, and the others keep throwing the same error until each is fixed.

## Zombies see 128 units, and nothing ever puts it back

`zombie_setup_attack_properties` — the function every zombie goes through when it is released to fight — clamps sight hard:

```gsc
//try to prevent always turning towards the enemy
self.maxsightdistsqrd = 128 * 128;      // _zm_spawner.gsc
```

Nothing restores it afterwards. The spawner default it overwrites is `1024 * 1024`, or `script_sightrange` when the KVP is present (`spawner_shared.gsc`), so a zombie is left with **1/64 of the sighted area** it was spawned with. Inside a house that is invisible; once distance matters — an open street, a zombie carried away on a vehicle — it reads as "the AI is broken" because every other state on the zombie looks healthy.

If your zombies go blind at a suspiciously round distance, restore it right after the setup call, honouring the KVP if the mapper set one:

```gsc
self.maxsightdistsqrd = (isdefined(self.script_sightrange) ? self.script_sightrange : 1024 * 1024);
```

## A zombie you hold in script stops behaving like a zombie

Scripted control (an `AnimScripted` sequence, a forced hold position) suspends the behaviours the rest of the game assumes are running, and each fails in a way that doesn't look like it came from your script:

- **It becomes a wall.** A scripted body cannot yield, so it seals off whatever it is standing in — typically the doorway the player is trying to use — and the block only disappears when the zombie dies, which is what makes it read as level geometry. `PushPlayer( true )` cannot save you: it is shipped **commented out** in `zombie_setup_attack_properties` (*"push the player out of the way so they use traversals in the house."*), and pushing requires a body with behaviour left to yield with. Drop `self SetPlayerCollision( 0 )` for the length of the sequence and restore `1` the instant it ends — stock uses the same flag on a ragdolling zombie. Leaving it off makes a body that should be solid non-solid.
- **A hold position exactly at melee range is a dead band.** Melee is gated on `DistanceSquared(...) > ZM_MELEE_DIST_SQ` (`zombie.gsc`, `ZM_MELEE_DIST` `64` in `zombie.gsh`/`skeleton.gsh`), so a zombie parked *at* 64 units neither closes nor swings, and jitters between the two. Park it well inside — half that is comfortable.
- **Holding it still is `SetPathMode`, and the state is readable.** `self SetPathMode( "dont move" )` is how the shipped scripts park an actor for a scripted sequence; the corresponding readable state prints as `pathmode move allowed` when it is *not* parked, so you can confirm which side you are on. A rider on a **moving** platform is a different problem — a zombie with healthy state and an accepted goal still frequently just stands there — see **t7kb:moving-platforms**.
- **`ForceTeleport( position, angles, updategoalpos, resetEntity )` does more than move it.** `updategoalpos` defaults to **true**, so a teleport silently rewrites the goal you just set; `resetEntity` resets the entity's behaviours. `spawner_shared::teleport_spawned` passes reset `true` by default. Pass both explicitly when the order of teleport-then-goal matters, and note the call returns a value worth testing.

## Tuning zombie/player stats

Zombie health is **script-set, not a dvar**: `zombie_utility::set_zombie_var(zvar, value, is_float, column)` with vars `zombie_health_start` / `zombie_health_increase` / `zombie_health_increase_multiplier`. `level.zombie_health` does exist (`_zm.gsc:4097`, applied at spawn in `_zm_spawner.gsc`), but it is **recomputed from those vars every round**, so writing it directly is overwritten; set the vars. On the player side, the same function with `"player_base_health"` changes starting health. **Power-up drop rate is a zombie var too, not a dvar** — `_zm_powerups.gsc` reads no dvar at all, so no dvar exists for it. A drop arms when the players' combined `score_total` passes a threshold built from `zombie_powerup_drop_increment` (`2000`, multiplied by 1.14 after every drop), capped by `zombie_powerup_drop_max_per_round` (`4`). Override those after `_zm_powerups` inits, or set `level.custom_zombie_powerup_drop` — returning true from it replaces the stock drop.

## Navmesh & pathing

The ground **navmesh is auto-generated at compile time** (the full compile's `-navmesh` pass) — there is nothing to paint by hand for zombies to walk. A **`nav_volume`** brush entity is a different thing: it defines a 3D space "covered by a Nav Volume" (`t7.def.json`), which community reports tie to **flying** AI. So `NavVolume generation is skipped... no nav_volume brush in the level` is harmless on a ground-only map and matters once you add flying units — add a `nav_volume` covering their airspace then. (community; the ground navmesh also needs `cod2map64` run from `bin/`, **t7kb:compiling**.) Even in a single open, fully-connected area, add at least one pathnode — an area with no barriers can still confuse generation without one. To block zombies from a specific spot, prefer `DisconnectPaths` over deleting the navmesh outright — deleting can leave a disconnected mesh fragment zombies can't route around, while disconnecting the specific area keeps the rest usable. The `nav_volume` brush is covered here rather than in **t7kb:mapping** because it exists only to drive navmesh generation.

## Custom zombie variants: mostly APE, not scripting

Adding a genuinely new zombie **type** is primarily an APE data-authoring task, not a GSC one: derive an `archetype_zm_factory_zombie` → a Variant → a Spawner, then build a Character with the required parts (`_body`, `_head`, and gib pieces `_behead`/`_upclean`/`_lowclean`/`_r|larmoff`/`_r|l|blegoff`), place the spawner in Radiant with the KVPs above, and register it in the zone (`aitype,archetype_zm_<name>_zombie`). Scripting (custom spawn functions, behavior-tree edits) is for *behavior* that diverges from stock — most "add a new zombie skin/variant" requests don't need any.

**Making an AI immune to Alternate Ammo Types (AATs — Pack-a-Punch effects) is a stock hook, not a rewrite:** `aat::register_immunity( name, archetype, immune_trigger, immune_result_direct, immune_result_indirect )` (`aat_shared.gsc:303`), as `_zm_ai_dogs.gsc` does for dogs. Don't reassign `level.aat[…].validation_func` (the community recipe): it replaces the AAT's own validation for everyone.

Rigging the underlying model (extracting the factory armature, binding joints, head/jaw split) is a separate asset-pipeline task — see **t7kb:assets** for that; this section is about wiring an already-rigged model into the AI/spawner system.

## Don't invent

The `.ai_bt`/`.ai_asm`/`.ai_am`/`.ai_ast` schema and KVP names here are shipped, Treyarch-authored tokens — confirm exact function/KVP names against the raw mod-tools install before stating them as fact. If neither t7kb nor the raw install supports a specific behavior-tree entry or spawner KVP, don't assert it exists.
