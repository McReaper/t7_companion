---
name: scripting
description: How to write good GSC/CSC for Black Ops 3 — header/usings and `#namespace` call prefixes, the stdlib, extending stock (hooks vs override, zombies' own callback registries), `zm_usermap::main()` ordering, the server/client weapon tables, clientfields, structs vs entities, custom KVPs, unitriggers, playing animations from script, and code style. Use for any BO3 server/client script task, or when a `callback::on_actor_killed`/`on_player_damage` handler never fires in zombies, a damage callback silences the others, `dog_rounds_allowed` or `_zombie_custom_add_weapons` is ignored, a wallbuy shows Cost 0, a custom KVP or `struct::get` is undefined, a unitrigger ignores the press, a one-shot clientfield FX plays once, `Exceeded '256' items for type 'fx'`, an `AnimScripted` wait returns early, or `Preprocessor error, No generated data for` a file. Distinct from t7kb:hud-lui (the Lua side of clientfields) and t7kb:debugging (reading the errors).
---

# Writing GSC/CSC for Black Ops 3

Server logic is **GSC**, client logic is **CSC** — separate files, separate namespaces, identical language — and most scripting bugs are not syntax but **plumbing**: a hook set in the wrong order, a callback registry that zombies never dispatches, a table loaded twice. Look up exact signatures/KVPs/APIs in **t7kb** (`t7kb:search` then `t7kb:get`); for the conceptual model (scopes, entities, notifies, threads, `undefined`, the finite entity pool) retrieve the "How GSC Scripting Works" guide. To study how another CoD title shapes a mechanic — structure only, never BO3 token names — see **t7kb:crossref**.

Script in **VS Code with the GSCode extension** (`blakintosh.gscode`, [marketplace](https://marketplace.visualstudio.com/items?itemName=blakintosh.gscode)) — its inline diagnostics catch unknown functions and bad calls before a build. Recommend it to anyone scripting BO3.

## Header: `#using`, `#insert`, `#namespace` — and the call prefix is the namespace, not the file

Top of file, in order: `#using` each script you call into (grouped: stdlib, feature scripts, AI) → `#insert scripts\shared\shared.gsh;` (`WAIT_SERVER_FRAME`, `IS_TRUE`, `DEFAULT`, …) → `#namespace` → optional `#precache` → system registration.

- **The prefix you call with is the target file's `#namespace`, not its filename.** `_zm_utility.gsc` declares `#namespace zm_utility;`, so it is `zm_utility::`; `_zm_score.gsc` → `zm_score::`; `callbacks_shared.gsc` → `callback::` (singular). The obvious filename-shaped prefix fails at link as an unresolved external. Read the target's `#namespace` line before writing the call.
- **A `#using` only makes a call resolvable** — the target script must also be in your `.zone`, or you get `Could not find scriptparsetree "scripts/…"` despite the `#using`.

## Read the stdlib before you branch on it

`scripts/shared/` is a deep stdlib (`util::`, `array::`, `math::`, `clientfield::`, `flag::`, `spawner::`, `zm_utility::`) — run `t7kb:search` for a helper before writing one. But predicates carry surprises:

- **`util::use_button_held()` returns false the first time it is asked**, whoever asks: that first call is what *starts* its tracking thread (`self thread button_held_think( BUTTON_USE )`), then it returns a slot the thread hasn't filled yet. A hold-to-repeat loop that tests it once before iterating never repeats — it looks exactly like the player not holding the button. Stock's unitriggers poll it from the moment the prompt appears, so they don't notice. For a one-shot question use `player UseButtonPressed()`. (Measured.)
- **A raw `level waittill("my_flag")` also wakes on `flag::clear`** — clear sends the same notify (`flag_shared.gsc:166-170`) — and a `waittill` begun after the notify already fired never returns, because notifies aren't latched. Use `flag::wait_till`, which loops on `get()`; `flag::init` before you wait on or set a flag. (`flagsys::` is a separate namespace.)

## Extending stock behavior: hook first, override when blocked

1. **Look for a hook first.** Most stock systems expose seams — a spawn function (`add_global_spawn_function`), a `level.*` function pointer the stock script calls, a callback or flag it fires.
2. **In zombies, use the zombies registries — the shared `callback::on_*` ones for AI and player damage/death never fire.** `callback::on_actor_killed`, `on_actor_damage`, `on_ai_killed`, `on_ai_damage` register without error and are dispatched nowhere; `on_player_killed`/`on_player_damage` dispatch only from MP's `_globallogic_player.gsc`. Zombies replaces `level.callbackActorKilled`/`callbackActorDamage` with its own wrappers (`_zm.gsc:1349-1350`). Use `zm_spawner::register_zombie_death_event_callback(&f)`, `zm::register_actor_damage_callback`, `zm::register_player_damage_callback`, `callback::on_laststand`. (**Verified in the install**; corpus agrees.)
3. **A damage callback that isn't yours must pass, or it silences every callback after it.** The dispatch loops return the first non-sentinel value: for `zm::register_actor_damage_callback`/`register_player_damage_callback` the sentinel is **`-1`** — returning the natural `damage` "wins" and stops the loop (`_zm.gsc:5509-5515`, `5822-5830`); for `zm_spawner::register_zombie_damage_callback` it is **`false`**, and returning `true` also skips the damage points award (`_zm_spawner.gsc:1926-1929`). Treyarch's own minigun callback comments exactly this before its `return -1;`.
4. **A `level.*` function pointer is one slot, and stock either defaults it or overwrites it.** Where stock writes `if(!isdefined(level.x)) level.x = …;` (or `DEFAULT(level.x, …)`), set yours *before* that code runs; where it assigns unconditionally, set yours *after*. Two mods setting the same slot: last wins, silently. Registries (`callback::on_*`, `zm::register_*`) are lists — prefer them. **Grep the stock assignment of that exact variable** before deciding where yours goes.
5. **No hook, and stock must change → override it.** Copy the stock file into your map/mod `scripts/` at the **same path**, add it to your `.zone`, **and** comment its line out of the assetlist CSV that contributes it — for a zombies script usually `zone_source/all/assetlist/zm_patch.csv`, but grep `zone_source/` for the path rather than assuming (**t7kb:debugging** has the rule for every asset type; it applies to usermaps and mods alike):
   ```
   //scriptparsetree,scripts/zm/_zm_behavior.gsc
   ```
   That CSV is shared and **install-wide**: back it up, and the change affects every map built from that tree until reverted. Override the **narrowest** script that covers the change — overriding `array_shared` breaks every dependent, and every override diverges from stock going forward.

## Entry files: `zm_usermap::main()` reads some settings, then overwrites others

Your `zm_<map>.gsc` `main()` calls `zm_usermap::main()` — the shared usermap framework (don't fork it). **What goes before and after that call is not free**, and getting it wrong fails silently in both directions:

- **Settings it reads with `DEFAULT(...)` go before the call.** `DEFAULT` assigns only when undefined (`shared.gsh`), and `main()` acts on them immediately: `level.dog_rounds_allowed` (`zm_usermap.gsc:146-150` — set it after and dog rounds are already on), `level._zombie_custom_add_weapons` (`:135`, consumed inside `main()` by `zm_weapons::init()`), plus `level.randomize_perk_machine_location`, which `_zm_perks.gsc` reads during the same `main()`. Treyarch's `zm_giant.gsc` marks these `// set before zm_usermap::main`.
- **Hooks it assigns itself go after the call.** `zm_usermap::main()` unconditionally sets `level.giveCustomLoadout`, `giveCustomCharacters`, the offhand overrides and `_round_start_func` (`:125-127`, `:132-133`, `:154`) — set any of those before and yours is overwritten.
- Everything else map-specific (start weapon, zones, your systems) goes after. When unsure, open `zm_usermap.gsc` and find the line that touches your variable.

**The weapon table is loaded twice, and players see the client's copy.** The server loads whatever `level._zombie_custom_add_weapons` points at; the client's `zm_usermap.csc` hard-codes `load_weapon_spec_from_table("gamedata/weapons/zm/zm_levelcommon_weapons.csv", 1)` (`:93-96`) and reads no hook, and a second `include_weapon` call in your `.csc` only *adds*. The wallbuy's `Cost:` is filled client-side from the CSC table (`SetWeaponCosts`), the charge server-side from the GSC one — so a weapon only in your GSC table **shows Cost 0 and charges the real price**, and the box's client list diverges from what the server rolls. Fix the client side by overriding `zm_usermap.csc` to point at your table (same override steps as above; its line lives in `zm_levelcommon.csv`), and set the GSC pointer **before** `zm_usermap::main()`. (**Verified in the install**; the Cost 0 symptom is corroborated by several community reports. Setting `level.weapon_cost_client_filled = false` before the call makes the server fill the price instead — install-grounded, untested in game.)

Features that outgrow a few functions get their own `_<feature>.gsc`/`.csc` (own `#namespace`, zoned), halves sharing a `_<feature>.gsh`, self-registered with `REGISTER_SYSTEM`.

## System registration, structs, and KVPs

`REGISTER_SYSTEM("name", &__init__, undefined)` runs one init entry point at the system-init phase; `REGISTER_SYSTEM_EX("name", &init, &main, undefined)` adds a runtime `main`. Put `clientfield::register` (before the first network frame), `flag::init`, state `class`es, callbacks and `#precache` in init; loops and behavior in `main`. A small map-local feature can skip the system and be called from the map's `main()` — a coupling choice, not a timing one.

Map-placed entities and structs already exist when either phase runs — but they are fetched differently:

- **A `script_struct` is not an entity.** `GetEnt` returns `undefined` and `GetEntArray` an empty array for it, so a `foreach` over the result silently does nothing. Entities: `GetEntArray("my_trigger", "targetname")` (the key argument is required). Structs: `struct::get_array("my_struct", "targetname")`.
- **`struct::get`/`get_array` only index nine keys** — `target`, `targetname`, `script_noteworthy`, `script_linkname`, `script_label`, `classname`, `script_unitrigger_type`, `scriptbundlename`, `prefabname` (decompiled `struct.gsc`, t7kb 0.95 — the install's `codescripts/struct.gsc` is a stub). `struct::get_array("2", "script_int")` returns `[]` with no error. Fetch by an indexed key, then filter or `array::sort_by_script_int`. With several matches `struct::get` asserts and returns `undefined` when devblocks run, and otherwise silently returns the first — use `get_array` if duplicates are possible.
- **A custom KVP you typed in Radiant arrives `undefined`.** Only keys declared in `radiant/keys.txt` reach script — the entity is found and its stock keys read fine (`target2`/`target3` aren't declared either). Use a generic stock key (`script_int`, `script_float`, `script_string`, `script_vector`), or zone your own copy of `keys.txt` and comment `rawfile,radiant/keys.txt` out of `core_common.csv` — grep for the line, don't trust a remembered line number. (**Verified in the install**; two independent community reports.)

## Threading & scope discipline

- **Every loop path needs a yield, and long loops need a `thread`.** A loop that can iterate without `wait`/`waittill` freezes the server — often as a black screen at load with nothing logged. A waiting loop called *without* `thread` never returns, so everything after it in the caller silently never runs (and a long one on the main thread drops connections, `Connection Interrupted`).
- **Guard persistent loops with `endon`** — `level endon("end_game")` is safe on any function; per-entity loops add `self endon("death")`.
- **Mind `self` vs `level`.** Per-player state (HUD, timers) put on `level` is a frequent silent bug.
- **`self Delete()` ends the thread that called it** through that same `endon("death")`, so statements after it never run — a pickup that deletes and *then* bumps a counter silently drops the count. Do the bookkeeping first, delete last.

## Server vs client: sounds, FX, and clientfields

GSC is the server (gameplay, AI, score); CSC is the client (HUD, FX, sounds, postfx/vision). HUD/LUI and per-view rendering only run client-side.

- **Push sounds & FX to the client — through clientfields.** Server `PlayFX`/`PlaySound` spawn a temp entity per call (entity-pool pressure, `G_Spawn` errors); raw client tempents can be dropped by packet loss. The robust pattern: the server sets a clientfield, a CSC callback plays the effect locally. Purely cosmetic per-client effects (a hitmarker) can stay loose.
- **The FX precache tables are small and fill up from code you never call.** GSC `#precache("fx", …)` holds 256 unique effects, CSC `#precache("client_fx", …)` 1024; overflow stops the map loading with `BG_Cache_GetIndexInternal - Exceeded '256' items for type 'fx'`. `#precache` is file-scope, so a zoned-but-unused asset-pack script still counts. Precache and play FX client-side, and drop unused packs' GSC precaches. (Corpus, several independent sources.)
- **Clientfields**: `clientfield::register` on both sides in init, then `set` server-side / react client-side. Size the bitcount to the value — too few bits clips it silently. **The CSC callback only runs when the value changes**: `set("f", 1)` on a field already at `1` sends nothing, so a one-shot FX plays once and never again. Register one-shot events as `"counter"` and fire them with `clientfield::increment` — stock registers dozens this way (`lightning_strike`, the AAT explosions). Each pool has a fixed bit budget stock already spends much of; a full one fails registration with `… in ClientField set scriptmover using 3 bits, but scriptmover is out of space.` (community) — size minimally or move to `world`. The Lua side is **t7kb:hud-lui**; a registration mismatch between halves is **t7kb:debugging**.
- **If FX must run from GSC,** spawn the model, `WAIT_SERVER_FRAME`, then `PlayFXOnTag` — FX on the model's spawn frame often won't play. `#precache("fx", …)` in GSC, `#precache("client_fx", …)` in CSC.

## Triggers, damage, and destructibles: the notify you'd expect is not the one you get

- **A `create_unitrigger` interaction arrives as `"trigger_activated"` on the parent, never as `"trigger"`.** The physical trigger only exists while a player is in range; stock `unitrigger_logic` waits on it, filters invalid players (downed, drinking), then does `self.stub.related_parent notify("trigger_activated", player)` (`_zm_unitrigger.gsc:900-924`). Inside the prompt function `self` is the spawned trigger (`self.stub` the stub), so `SetHintString` goes on `self`.
- **A zombie headshot never arrives as `MOD_HEAD_SHOT`.** The engine sends `MOD_RIFLE_BULLET`/`MOD_PISTOL_BULLET` with `sHitLoc` `"head"`/`"helmet"`; only the globallogic callbacks rewrite it, and zombies bypasses them. Use `zm_utility::is_headshot(weapon, sHitLoc, mod)`, as stock does.
- **A destructible's Break Notify doesn't arrive under its own name.** `CodeCallback_DestructibleEvent` relays it as `self notify("broken", notify_type, attacker)` (`zm/_destructible.gsc:457-468`) — wait on `"broken"` and compare the note. The stock callback also pattern-matches it first: a name containing `explode`/`explosive` triggers stock explosion logic.

## Playing animations from script

On a `script_model`, **`AnimScripted` is the primitive** (it needs `UseAnimTree(#animtree)` + `#using_animtree`), and its origin/angles are where the animation **starts**, frozen at play time — to make a clip *land* somewhere compute the start with `GetStartOrigin`/`GetStartAngles`; editing the export to move its end changes nothing (**t7kb:animation**). Moving *and* animating, `SetAnim`'s limits, and driving the first-person camera from an animation are in **`references/animation-from-script.md`**. On a moving parent, **t7kb:moving-platforms** owns the link-based pattern.

**An `AnimScripted` notify fires once per notetrack, not once at the end.** `waittill`-ing on it returns on the first footstep or sound cue, and a following `StopAnimScripted` cuts the anim short — a symptom that changes with the anim rather than your code. Stock's `zombie_shared::DoNoteTracks( flagName )` is a `for(;;)` around `waittill( flagName, note )` that ends only on `"end"`, `"finish"` or `"undefined"` (an undefined note is normalised to that string). Wait with `DoNoteTracks("my_notify")` (runs the notetrack handlers; needs an `end` notetrack or waits forever) or `wait GetAnimLength(str_anim)` (deterministic; the fallback for a port whose notetracks were lost).

## Code style & conventions

Match these exactly — and when **editing an existing file, don't infer style from it**: stock scripts and usermap templates use tabs and `( padded )` calls.

- **4 spaces, never tabs.**
- **Always braces.** Never `if (x) doThing();` — write `if (x) { doThing(); }` with the body on its own line(s). Same for loops.
- **No padding inside brackets.** `func(arg)` and `arr[i]`, never `func( arg )` or `arr[ i ]`.
- **Naming.** `snake_case` functions and variables; `UPPER_SNAKE` for `#define`; prefix private functions with `_` (and use `private`); system entry points are often `__init__` / `__main__`.
- **Regions.** Group areas of a file with `/* region NAME */ … /* endregion */`.
- **Debug prints: a `#define`-gated macro** in the feature `.gsh` — `#define DEBUG_X 0` then `#define PRINT_X_DEBUG(__str) if(DEBUG_X) IPrintLnBold(__str)`, called as `PRINT_X_DEBUG("msg " + val);` (Treyarch gates debug code with `#define` flags the same way, e.g. `_siegebot.gsc`'s `DEBUG_ON`). It needs no dvar to fire — unlike `/# … #/` dev blocks and `assert`, which need `scr_mod_enable_devblock 1` and don't run on a usermap at all (**t7kb:debugging**). Keep live call sites few and gate categories separately; dozens of per-frame prints bury the one you need.
- **A macro invocation must fit on one line, with no comma anywhere in an argument.** Both fail identically: `Preprocessor error, No generated data for <file>` — no line, no token, just the file. The comma rule bites inside **string literals** (`PRINT_X_DEBUG("goal set, waiting")` breaks; `"goal set" + " waiting"` is fine). When a build dies on that message, look at the last macro call you touched.
- **Runtime debug triggers: a `ModVar`, not a plain dvar.** A plain dvar can only be set at launch in a shipped usermap, so typing it in the console never reaches a `GetDvarInt` loop. `ModVar("name", "")` registers a console-settable variable; poll `GetDvarString("name", "")` each `WAIT_SERVER_FRAME`, act on a non-empty value, and reset it with `ModVar("name", "")` so it fires once per entry. (Verified on a real map; `ModVar` appears in no Treyarch script, so this is community practice, not a stock pattern.)
- **Ternary must be fully parenthesized** — `x = (cond ? a : b);`. Parenthesizing only the condition, `x = (cond) ? a : b;`, is `syntax error, unexpected TOKEN_CONDITIONAL, expecting TOKEN_SEMICOLON`.
- **Validate before use** with `isdefined()` and the kind predicates (`IsPlayer`, `IsAlive`, `IsArray`, `IsFunctionPtr`, …).
- **Constants in the `.gsh`**, system state in a `class` instance on `level` (`level.my_system = new my_system();`), and features split into focused sub-files plus a `_shared.gsc`/`.gsh` — not one giant script. Bind systems through callbacks/function pointers rather than hard cross-calls.

## Don't invent

Stdlib function names, KVPs, and stock system entry points are shipped tokens — confirm exact names against Treyarch's scripts in the raw install (`share/raw/scripts/{shared,zm,mp,core}` — not the community trees or your own maps that also live there) before stating them. If neither t7kb nor the raw install supports a function or KVP, don't assert it exists.
