---
name: bo3-zombies-ai
description: How to work with zombies/character AI in Black Ops 3 — behavior trees (`.ai_bt`/`.ai_asm`/`.ai_am`/`.ai_ast`), spawners and risers, custom traversals, navmesh, custom zombie variants, and engine flags a scripted zombie must manage (`maxsightdistsqrd`, `SetPlayerCollision`, `ForceTeleport`, `PathMode`). This is in-game AI (zombie/character behavior, pathing, spawning) — not LLM/agent AI, don't conflate the two. Use for custom zombie types/variants, spawner logic, or AI not behaving/pathing/attacking correctly: zombies replaying an idle "dance", `assert fail` on a riser's `script_string`, zombies losing sight of a player they can plainly see, a scripted zombie that blocks the player like invisible geometry until it dies, or one parked at melee range that neither swings nor closes. Distinct from bo3-moving-platforms (AI carried by a vehicle/train/elevator and boarding it) — this is AI on static geometry. Navmesh generation and pathing quirks stay here, not in bo3-mapping's brush/geometry craft.
---

# Zombie / character AI in Black Ops 3

Custom AI behavior is driven by four file types working together, not one — look up exact function names/KVPs in **t7kb** (`t7kb:search` then `t7kb:get`), but the file relationship below is the part that trips people up.

## The four files that drive AI behavior

`.ai_bt` (behavior tree), `.ai_asm` (anim state machine), `.ai_am` (anim table), `.ai_ast` (**AnimState Tree** — a blackboard-attribute + selector table that resolves an `animation_selector` to an `_animation_alias`, not just "an anim state") — under `share/raw/behavior`, `animstatemachines`, `animtables`, `animstates` respectively. ASM vs AST is the single most important distinction to keep straight here: they tell the AI *what to do* (bt) and *how to animate it* (asm/am/ast). Adding a custom zombie variant or a custom traversal usually means touching more than one of these — a fix that only edits the behavior tree but not the matching anim table (or vice versa) is a common half-fix.

## Behavior tree gotcha: your usermap copy can be stale, not the one from `share/raw`

If zombies spawn but just stand there replaying an idle animation ("the zombie dance"), the map GSC is usually missing a community-pack `#using` for its behavior module plus that module's scriptparsetree in the `.zone` — add both. Separately: when debugging a broken behavior (e.g. a ported special zombie not reacting to a weapon), don't assume the `.ai_bt`/`.ai_asm`/`.ai_am`/`.ai_ast` sitting in your usermap folder is correct — pull the canonical one from `share/raw` first; a stale usermap copy missing the checks for your case (e.g. a specific wonder weapon) is a frequent cause, and the fix is adding the missing checks to *all four* files, not just one.

## Custom traversals need a behavior tree entry — the animation existing isn't enough

A traversal animation being visible in a prefab does **not** mean AI will use it: the traversal must be added to the behavior tree, or the AI won't take it (this trips people up for non-standard AI like dogs, not just zombies). Procedural traversals (the AI measures distance and jumps to match) are a distinct mechanism from fixed custom traversals and need their own behavior tree setup. Traversal names follow a literal convention in the anim tables, e.g. `JUMP_UP_36_DOWN_52` — match the name to hook up the right entry.

## Spawning zombies: it's mostly Radiant KVPs, not a GSC call

The actual spawn recipe is entity setup, not scripting: a spawner entity `actor/spawner_zm_factory_zombie` with `script_noteworthy = zombie_spawner`, `count = 9999`, `coop_count = {9999,0,0,0}`, and `script_forcespawn = 1`. **Risers** are a separate entity — a `script/struct` with `targetname = <zone>_spawners`, `script_noteworthy = riser_location`, `script_string = find_flesh`. Fallers/crawlers and special types (margwa/nova/quad) have their own KVPs and devraw prefabs — check the prefab itself or t7kb if a type-specific KVP isn't in the usual list. On the script side, `zombie_utility::spawn_zombie(spawner, ...)` is the runtime call that actually spawns against a placed spawner; to run custom logic on every spawn **without forking anything**, hook it (`spawner::add_archetype_spawn_function("zombie", &my_spawn_function)`) — same hook-first philosophy as `bo3-scripting`.

**Fresh maps ship with `dog_rounds_allowed` on but no dog-spawner prefab** — an easy-to-miss gotcha that produces an infinite dog round with nothing spawning. Fix by adding a `dog_spawner.map` (from The Giant) stamp plus a `<zone>_spawners`-named struct, or just disable dog rounds outright with `level.dog_rounds_allowed = false`.

**Zombies with no valid target just stand still** — a solo player downed, or anyone under Zombie Blood, leaves nothing for zombies to path toward. Fix with lightweight `script_noteworthy = zombie_poi` structs for a simple case, or the full `zm_giant_cleanup_mgr` hook (`enemy_location_override`/`no_target_override`) for `dog_location`-based relocation in a more involved one.

## A riser's `script_string` must be matched by an `exterior_goal`, or the AI throws at spawn

A riser's `script_string` is not decoration: `_zm_spawner`'s spawn path copies it onto the zombie as **`find_flesh_struct_string`**, and `findNodesService` in `_zm_behavior.gsc` then walks `level.exterior_goals` looking for one whose **`script_string` is equal**. `find_flesh` is the special value meaning "no entrance, just chase" — it returns early and is always safe. **Any other value must be declared by an `exterior_goal` struct somewhere**, or `node` stays undefined and you get, in this order:

```
assert fail: <the string it wanted>
undefined is not a field object
SetGoal() unsupported goal type
```

on every zombie that rises there — while zombies from a `find_flesh` riser in the same map behave perfectly, which makes it look intermittent rather than deterministic. The assert names the missing string, so read it before theorising.

**The trap that produces this:** `script_string` set on the **prefab instance** (the `misc_prefab` entity) does **not** propagate to the entities inside it. A barrier prefab placed with `script_string "receiver_set_entry_a"` still has an `exterior_goal` carrying no `script_string` at all, so nothing declares that entry. Put the KVP on the `exterior_goal` **struct itself** — which for a stock prefab means taking a local copy of it rather than editing shared content.

And check for **more than one**: a map can have several risers asking for different entries, and fixing the first one you find leaves the others throwing exactly the same error, which reads as "the fix didn't work".

## Zombies see 128 units, and nothing ever puts it back

`zombie_setup_attack_properties` — the function every zombie goes through when it is released to fight — clamps sight hard, and the comment above it says why it was done rather than what it costs:

```gsc
//try to prevent always turning towards the enemy
self.maxsightdistsqrd = 128 * 128;      // _zm_spawner.gsc
```

Nothing restores it afterwards. The spawner default it overwrites is `1024 * 1024`, or `script_sightrange` when the KVP is present (`spawner_shared.gsc`), so a zombie is left with **1/64 of the sighted area** it was spawned with. Inside a house that is invisible and deliberate. It stops being invisible the moment distance is part of the design — an open street, a zombie carried away from the player on a vehicle, anything that should notice a player it can plainly see — and it reads as "the AI is broken" rather than as a tuning value, because every other state on the zombie looks healthy.

If your zombies go blind at a suspiciously round distance, restore it right after the setup call, honouring the KVP if the mapper set one:

```gsc
self.maxsightdistsqrd = (isdefined(self.script_sightrange) ? self.script_sightrange : 1024 * 1024);
```

## A zombie you hold in script stops behaving like a zombie

Scripted control (an `AnimScripted` sequence, a forced hold position) suspends the behaviours the rest of the game assumes are running, and each one fails in a way that doesn't look like it came from your script:

- **It becomes a wall.** A scripted body cannot yield, so it seals off whatever it is standing in — typically the doorway the player is trying to use — and the block only disappears when the zombie dies, which is what makes it read as level geometry. `PushPlayer( true )` cannot save you: it is shipped **commented out** in `zombie_setup_attack_properties` (*"push the player out of the way so they use traversals in the house."*), and pushing requires a body with behaviour left to yield with. Drop `self SetPlayerCollision( 0 )` for the length of the sequence and restore `1` the instant it ends — stock uses the same flag on a ragdolling zombie. Don't leave it off: a body that should be solid then isn't.
- **A hold position exactly at melee range is a dead band.** Melee is gated on `DistanceSquared(...) > ZM_MELEE_DIST_SQ` (`zombie.gsc`, `ZM_MELEE_DIST` `64` in `zombie.gsh`/`skeleton.gsh`), so a zombie parked *at* 64 units neither closes nor swings, and jitters between the two. Park it well inside — half that is comfortable.
- **Holding it still is `SetPathMode`, and the state is readable.** `self SetPathMode( "dont move" )` is how the shipped scripts park an actor for a scripted sequence; the corresponding readable state prints as `pathmode move allowed` when it is *not* parked, so you can confirm which side you are on rather than guessing. A rider on a **moving** platform is a different problem — a zombie with healthy state and an accepted goal still frequently just stands there — see **bo3-moving-platforms**.
- **`ForceTeleport( position, angles, updategoalpos, resetEntity )` does more than move it.** `updategoalpos` defaults to **true**, so a teleport silently rewrites the goal you just set; `resetEntity` resets the entity's behaviours. `spawner_shared::teleport_spawned` passes reset `true` by default. Pass both explicitly when the order of teleport-then-goal matters, and note the call returns a value worth testing.

## Tuning zombie/player stats

Zombie health is **script-set, not a dvar**: `zombie_utility::set_zombie_var(zvar, value, is_float, column)` with vars `zombie_health_start` / `zombie_health_increase` / `zombie_health_increase_multiplier` (there's no single `level.zombie_health` to read or set directly). On the player side, the same function with `"player_base_health"` changes starting health. Power-up drop rates are dvar-controlled — run `t7kb:search` for the specific dvar name before assuming a hardcoded value.

## Navmesh & pathing

The navmesh is **auto-generated by the launcher at compile time** — there's no manual "nav volume" texture or brush to paint by hand, and looking for one by that literal name is a dead end. What you *do* place is a `nav_volume` brush entity; if the compiler warns `NavVolume generation is skipped... no nav_volume brush in the level`, add one (or pass `-navvolume`). Even in a single open, fully-connected area, add at least one pathnode — an area with no barriers can still confuse generation without one. To block zombies from a specific spot, prefer `DisconnectPaths` over deleting the navmesh outright — deleting can leave a disconnected mesh fragment zombies can't route around, while disconnecting the specific area keeps the rest usable. The `nav_volume` brush is the one piece of level geometry covered here rather than in **bo3-mapping**: it exists purely to drive navmesh generation, not for visible/collision geometry, so it stays with the AI craft it feeds.

## Custom zombie variants: mostly APE, not scripting

Adding a genuinely new zombie **type** is primarily an APE data-authoring task, not a GSC one: derive an `archetype_zm_factory_zombie` → a Variant → a Spawner, then build a Character with the required parts (`_body`, `_head`, and gib pieces `_behead`/`_upclean`/`_lowclean`/`_r|larmoff`/`_r|l|blegoff`), place the spawner in Radiant with the KVPs above, and register it in the zone (`aitype,archetype_zm_<name>_zombie`). Scripting (custom spawn functions, behavior-tree edits) is for *behavior* that diverges from stock — most "add a new zombie skin/variant" requests don't need any.

Rigging the underlying model (extracting the factory armature, binding joints, head/jaw split) is a separate asset-pipeline task — see **bo3-assets** for that; this section is about wiring an already-rigged model into the AI/spawner system.

## Don't invent

The `.ai_bt`/`.ai_asm`/`.ai_am`/`.ai_ast` schema and KVP names here are shipped, Treyarch-authored tokens — confirm exact function/KVP names against the raw mod-tools install before stating them as fact. If neither t7kb nor the raw install supports a specific behavior-tree entry or spawner KVP, don't assert it exists.
