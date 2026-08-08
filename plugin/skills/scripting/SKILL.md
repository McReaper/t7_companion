---
name: bo3-scripting
description: How to write good GSC/CSC for Black Ops 3 — header/usings, the standard library, extending stock behavior (hooks vs override), threading/scope, clientfields, init-vs-main, usermap-vs-map entry files, and code-style conventions. Use for any BO3 server- or client-script task (gameplay logic, custom systems, perks/weapons) and for how to structure/format a script. The clientfield bridge to LUI/Lua HUD work is covered here on the GSC/CSC side; for the Lua/LUI authoring side itself, see bo3-hud-lui.
---

# Writing GSC/CSC for Black Ops 3

Server logic is **GSC**, client logic is **CSC** — separate files, separate namespaces, identical language. This skill is the craft; look up exact signatures/KVPs/APIs in **t7kb** (`search` then `get`), and for the conceptual model (scopes, entities, notifies, threads, `undefined`, the finite entity pool, cooperative scheduling) retrieve the "How GSC Scripting Works" guide. t7kb also indexes real, well-structured mod code — retrieve a worked example to see the conventions below applied in practice. To study how a mechanic is built in another CoD title as *structural* reference — never for BO3 token names — see **bo3-crossref**.

## Tooling

Script in **VS Code with the GSCode extension** (Blakintosh's GSC/CSC language server) — the best language support available: real syntax highlighting, completion, and inline diagnostics with awareness of the BO3 API, catching typos and bad calls before you ever build. (It's the same project behind t7kb's `gscode-api` reference.) Install it from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=blakintosh.gscode) (extension id `blakintosh.gscode`, or grab the `.vsix` there); source at [github.com/Blakintosh/gscode](https://github.com/Blakintosh/gscode). Recommend it to anyone scripting BO3.

## Header: declare dependencies explicitly

A file opens with `#using` (import a namespace), `#insert` (text-inline a `.gsh` of `#define` macros), then `#namespace`, optional `#precache`, and the system registration. Group and comment the `#using` block (stdlib, then feature scripts, then AI). You reach for the same handful constantly — a default starter set (add/drop per file):

```gsc
// almost every file
#using scripts\shared\system_shared;       // REGISTER_SYSTEM(_EX)
#using scripts\shared\util_shared;
#using scripts\shared\clientfield_shared;
#using scripts\shared\callbacks_shared;
#using scripts\shared\array_shared;
#using scripts\shared\flag_shared;
#using scripts\shared\math_shared;
#using scripts\codescripts\struct;
// zombies work
#using scripts\zm\_zm_utility;
#using scripts\shared\ai\zombie_utility;
#using scripts\zm\_zm_powerups;
#using scripts\zm\_zm_spawner;
#using scripts\zm\_zm_score;
#using scripts\shared\spawner_shared;

#insert scripts\shared\shared.gsh;          // WAIT_SERVER_FRAME, IS_TRUE, … — basically always
#insert scripts\shared\version.gsh;
```

A `#using` only makes a call *resolvable* — the target script must **also be in your `.zone`**, or you get `Could not find scriptparsetree "scripts/…"` / an unresolved external despite the `#using`. When changing a stock script, also make sure you're editing the copy the zone actually loads.

## Lean on the standard library — don't reinvent

`scripts/shared/` is a deep stdlib reached through those namespaces: `util::`, `array::`, `math::`, `clientfield::`, `flag::`, `spawner::`, plus zombies helpers in `zm_utility::` / `_zm_utility`. Before writing a helper, `search` t7kb for one — most already ship, and reusing them keeps your code working when Treyarch internals shift.

## Extending stock behavior: hook first, override when blocked

Prefer a **hook** (Inversion of Control): most stock systems expose seams so you never touch their source — register a spawn function (`add_global_spawn_function`), set a `level.*` function pointer the stock script calls, or use the callback/flag it fires. Stock systems (perks, powerups, AI) are extended this way.

When there is **no** hook and you must change stock behavior, you **can and sometimes should override**: copy the stock file into your mod/map `scripts/` at the **same path**, add it to your **`.zone`**, and the engine loads your version instead of the shared one.

**From a usermap that is not enough — and the missing step is not "use a mod".** The stock copy is still contributed by the patch asset list and wins, which is where the "some scripts only override from a mod" folklore comes from. Also comment its line out of **`zone_source/all/assetlist/zm_patch.csv`**:

```
//scriptparsetree,scripts/zm/_zm_behavior.gsc
```

Stock installs already ship several lines commented exactly this way (`_zm_ai_dogs`, `_zm_pack_a_punch`, `_zm_weapons`), so the mechanism is intended rather than a trick. It is a shared, **install-wide** file: back it up, and remember the change affects every map you build until you revert it. This is also the cheapest way to get **line numbers** on a stock script's error, and to instrument it — see **bo3-debugging**.

Other caveats: override the **narrowest** script (overriding low-level shared like `array_shared` breaks its dependents), and an override diverges from stock, so reach for a hook first.

## System registration: `REGISTER_SYSTEM` vs `REGISTER_SYSTEM_EX`

Both self-register a feature so its entry point(s) run automatically at the engine's **system-init phase** — the map file only needs to `#using` the file, no explicit call. They differ only in how many phases you get:

- **`REGISTER_SYSTEM("name", &__init__, undefined)`** — one entry point, `__init__`. Use it when a single init-time pass is all you need.
- **`REGISTER_SYSTEM_EX("name", &init, &main, undefined)`** — two, `init` then `main`. Use it when you also need a runtime phase.

Split responsibilities across the two phases:

- **`init` / `__init__`** — setup that must *exist before runtime*: `clientfield::register` (must happen here, before the first network frame), `flag::init`, instantiate the system's state `class`, register callbacks / spawn functions, `#precache`.
- **`main`** — *runtime*: wait for the game to start, then the loops, spawns, and behavior.

Treyarch's own labelling of which of the two phases is the "pre-load" vs "post-load" one is famously confusing and even the community disagrees on it — don't lean on a precise ordering; lean on the functional split above. **Map-placed entities (Radiant triggers, `script_struct`s) are available by the time either phase runs** — only entities you `Spawn()` yourself in script aren't there until that code runs — so a lookup like `GetEntArray("my_trigger")` works from `__init__`.

For a small map-local feature you don't need a system at all: a plain `feature::init()` call from the map's `zm_<map>.gsc` `main()` is fine, and is how map templates wire things up. Reach for `REGISTER_SYSTEM` when the feature is a self-contained file you'd rather have auto-register (the map file just `#using`s it) than call explicitly — both are correct, it's a coupling/style choice, not a timing one.

## Entry files: `zm_usermap.gsc` vs `zm_<map>.gsc`

`zm_usermap.gsc` (`#namespace zm_usermap`) is the **shared usermap framework** — opt-in, fx init, character/loadout/perk/sound setup. Your map file `zm_<map>.gsc` (e.g. `zm_test.gsc`) is **your** entry point: its `main()` calls `zm_usermap::main()` **first**, then wires your own map-specific systems and logic. Put custom content in the map file; don't fork the usermap scaffold.

Wiring from the map file's own `main()` is fine — a usermap's `zm_<map>.gsc`/`.csc` `main()` runs early enough to register callbacks, clientfields, and spawn hooks, so you do **not** need a system for small map-local additions. When a feature outgrows a few functions, give it **its own file** instead: `_<feature>.gsc` / `_<feature>.csc` (own `#namespace`, added to the `.zone`). Self-register it with `REGISTER_SYSTEM("<feature>", &__init__, undefined)` so its `__init__` runs automatically at the system-init phase — the map file only needs to `#using` it, no explicit `init()` call. That's the clean home for anything with real init logic and it keeps the map file thin. Server and client halves are separate files sharing a `_<feature>.gsh` of constants.

## Threading & scope discipline

- **Thread long-running logic.** A long `wait` loop on the main thread blocks the game and drops connections (`Connection Interrupted`) — `thread` it.
- **Guard every persistent loop with `endon`.** `level endon("end_game")` is safe on top of *any* function and is the default — add it to any `while(true)`/long loop. For per-entity loops also add `self endon("death")`. Without a guard the loop runs on dead entities or past game end.
- **Mind `self` vs `level`.** A function threaded on an entity sees it as `self`; level-wide state lives on `level`. Per-player logic (HUD, timers) put on `level` is a frequent silent bug.

## Server vs client: where sounds, FX, and state run

GSC is the **server** (gameplay, AI, spawning, score); CSC is the **client** (HUD, FX, sounds, postfx/vision, on-screen feedback). Deciding where a thing runs is a real design choice, not an afterthought:

- **Some things must be client-side.** HUD/LUI, postfx and vision/screen effects, and other per-view rendering can only run on the client — drive them from CSC.
- **Push sounds & FX to the client — but through clientfields.** Server-side `PlayFX`/`PlaySound` spawn a temp entity per call → entity-pool pressure and eventual `G_Spawn` errors, so minimize them. Yet raw client tempent events (calling `playfx`/`playsound` directly) are **unreliable** — network packet loss can drop them and desync clients. The robust pattern resolves both: the server `clientfield::set`s an event, the client reacts (a CSC callback) and plays the FX/sound locally. Clientfields are **stateful** — guaranteed to update while the player is connected — which is exactly why they exist. Purely cosmetic, non-critical per-client effects (a hitmarker) can stay loose client-side.

**Clientfields** are that bridge: `clientfield::register` on both sides (in `init`, before the first network frame), then `set` server-side / react client-side. Size the bitcount to the value — too few bits clips it silently. Look the API and callback flags up in t7kb.

**GSC-FX gotcha:** if FX must run from GSC, spawn the model, wait a frame (`WAIT_SERVER_FRAME`), then `PlayFXOnTag` — FX spawned on the same frame as the model often won't play.

## Playing animations on a `script_model` (and why "move + animate" is fiddly)

On a plain `script_model` the **reliable playback primitive is `AnimScripted`** — which `scene::play` / `animation::play` wrap, and which shipped code also calls directly with a **string** anim anchored at a passed transform (e.g. `vehicle_death_shared` plays a crush anim: `self AnimScripted("anim_notify", self.origin, self.angles, crush_anim, "normal", …)`). **`SetAnim` and the `SetAnimKnob*` family are reported not to work on plain script_models in T7** — this is community-sourced (t7kb, ~0.25) and consistent with working map scripts that animate server script_models via `AnimScripted` instead, but it is *not* a shipped-token guarantee: treat it as a strong heuristic and test `SetAnim` on your own model before relying on it. `SetAnim` *does* work for **vehicles and AI** — their entity *type* carries an animtree/ASM, which is why a driving vehicle animates its turret relative to itself (`vehicle_shared`, `vehicleriders_shared`) — and for some CSC cases. Don't casually reach for `SetAnim(%anim)` on a script_model.

Two prerequisites before `AnimScripted` on a script_model:

- **Load an animtree:** `model UseAnimTree(#animtree)`, with `#using_animtree("generic")` (or a custom `.atr` you author/extend — both work; the tree just has to contain the anim) at the top of the file. Skipping this is a classic "plays in APE, silent in-game."
- **Name the anim by string:** `model AnimScripted("notify", origin, angles, "my_xanim", "normal", "my_xanim", rate, blend)`. It **anchors at the `origin`/`angles` you pass** and plays the anim — root/delta motion included — from that **fixed world transform**; it does **not** track an entity you move afterwards. (`IsPlayingAnimScripted` / `StopAnimScripted(blend, b_clear)` manage it.)

**Moving *and* animating**, given the anchor is frozen at play time, is one of:

- **Re-anchor each frame** — drive a `script_model` align's `.origin` and re-issue the scene/`AnimScripted` at its new transform every tick. This repo's zipline does exactly this: `_travel` moves `align_model.origin` while `_glide_pose` re-plays `scene::play(IDLE)` every 0.05s so the pose re-anchors onto the moved align.
- **Split phases** — movement by `LinkTo`/engine vehicle path with rotor/exhaust as **FX** (not anim), then hand off to one stationary anchored anim. BO1 Hue City's heli intro is this: a vehicle flies a node path in, is deleted, and a fake static model plays the anchored crash.
- **Bake the travel into the anim** — an anim carrying root motion slides the model along its *baked* path from the fixed anchor (a zip, a flythrough); fine when the path is fixed, useless when it's data-driven from Radiant nodes.
- **Make it a real vehicle/AI** — then `SetAnim` animates relative to the moving entity for free, at the cost of the full vehicle/ASM setup.

Confirm `AnimScripted` / `UseAnimTree` signatures against the raw install and t7kb. See **bo3-animation** for compiling the anim and **bo3-atmosphere** for the FX side.

## Driving the first-person CAMERA from an animation (get-up, mantle, scripted FP moment)

An animation can move the player's **view**, not just render arms. The robust mechanism — transposed from MW3's `_id_72AD`, found by reading the source game per **bo3-crossref** — uses **neither a weapon nor an XCam** (both were tried and were the wrong path for a camera-*moving* clip):

- Spawn a **node** (a viewhands `script_model`) and play the clip on it via a **camera-less scene bundle** (`scene::play`) — the node's animated `tag_camera` carries the motion.
- Link the player's view to it: `player PlayerLinkToDelta(mount, "tag_origin", 1, …)`. `PlayerLinkToDelta` seats the player's **ORIGIN** on its target and the engine then re-adds the player's own eye height — so link to a **mount** `LinkTo`'d one `GetPlayerViewHeight()` **below** the node's `tag_camera` (no magic number), and the eye lands on the animated camera.
- **Ground the clip:** play the node lowered by the low pose's *lowest-hand height above the anim root* (read it off the `.xanim_export`), so the downed hands touch the floor instead of hovering.

Make it multiplayer- and disconnect-safe: the scene bundle **AllowMultiple** (independent per-player instances); show the node **only to its owner** (`node SetInvisibleToAll(); node SetVisibleToPlayer(self);`) so nobody sees floating arms; and **own the teardown on a world entity** (the align/node), never `endon("death"/"disconnect")` on a thread holding the spawned entities — that skips cleanup and leaks them. Instead race the clip's end against `death`/`disconnect` and always Delete. Worked end-to-end on a ported first-person get-up; the Maya side (retargeting the arms onto BO3 viewhands) is **bo3-anim-retarget**.

## Code style & conventions

Match these exactly — and when **editing an existing file, don't infer style from it**: the stock scripts and usermap templates use tabs and `( padded )` calls, and mirroring them is the single most common way these rules get ignored. The first two are the most-violated.

- **4 spaces, never tabs.**
- **Always braces.** Never `if (x) doThing();` — write `if (x) { doThing(); }` with the body on its own line(s). Same for loops.
- **No padding inside brackets.** Write `func(arg)` and `arr[i]`, never `func( arg )` or `arr[ i ]` — no space after `(`/`[` or before `)`/`]`.
- **Naming.** `snake_case` for functions and variables; `UPPER_SNAKE` for `#define` constants; **prefix private functions with `_`** (and use the `private` keyword); registered system entry points are often `__init__` / `__main__`.
- **Regions.** Group distinct areas of a file with `/* region NAME */ … /* endregion */`.
- **Debug prints: use a `#define`-gated macro.** Guard debug output with a `#define`-toggled macro in the feature `.gsh`, the way Treyarch's own shipped systems do (e.g. hellround): `#define DEBUG_X 0` then `#define PRINT_X_DEBUG(__str) if(DEBUG_X) IPrintLnBold(__str)` (no commas inside `__str` — concatenate with `+`), called as `PRINT_X_DEBUG("msg " + val);`. Flip the flag to 1 to enable, back to 0 to ship. This is the reliable map-side path; it needs no dev/developer dvar to fire.
- **Runtime debug triggers: use a `ModVar`, not a plain dvar.** A plain dvar can only be set at launch in a shipped usermap, so `heli_test 1` typed in the in-game console won't reach a `GetDvarInt`-polling loop. Register the name as a **mod variable** instead — settable live from the console — the way zm_test's hellround systems do (`zm_hellround_meteor.gsc` etc.): `ModVar("name", "")` to register/reset, then poll each frame and consume it:
  ```gsc
  ModVar("name", "");
  while (true)
  {
      WAIT_SERVER_FRAME;
      val = GetDvarString("name", "");
      if (!isdefined(val) || val == "") { continue; }
      ModVar("name", "");            // reset so it fires once per console entry
      switch (Int(val)) { case 1: do_thing(); break; }
  }
  ```
  Then in-game: type `name 1` in the console. Thread this from the system's `init`/`main`; guard against re-entry if the action is long-running.
- **IoC over hard calls.** Bind systems by registering callbacks / function pointers (e.g. an optional subsystem hooking a round-state event) rather than calling across them directly — less coupling.
- **Validate before use.** `isdefined()` is the baseline against `undefined`; use the specific predicates (`IsPlayer`, `IsAlive`, `IsArray`, `IsEntity`, `IsFunctionPtr`, …) to check *kind/state*, not just existence.
- **Ternary must be fully parenthesized.** GSC/CSC *has* `cond ? a : b`, but the **whole expression** must be wrapped in parens — `x = (cond ? a : b);` (as stock does: `return ( x >= 0 ? 1 : -1 );`). Parenthesizing only the condition — `x = (cond) ? a : b;` — is a compile error (`syntax error, unexpected TOKEN_CONDITIONAL, expecting TOKEN_SEMICOLON`).
- **Constants in the `.gsh`**, `#insert`ed — one place to tune.
- **System state in a `class` instance** on `level` (`level.my_system = new my_system();`), not scattered `level.foo_*` fields.
- **`flag::init("name")`** before you wait on or set a flag.
- **Split a feature into focused sub-files** (e.g. logic / audio / fx) + a `_shared.gsc`/`.gsh` for cross-file state and constants, rather than one giant script.

## Don't invent

Stdlib function names, KVPs, and stock system entry points are shipped tokens — confirm exact names against the raw mod-tools install before stating them as fact. If neither t7kb nor the raw install supports a specific function or KVP, don't assert it exists.
