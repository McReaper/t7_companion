---
name: bo3-scripting
description: How to write good GSC/CSC for Black Ops 3 — header/usings, the standard library, extending stock behavior (hooks vs override), threading/scope, clientfields, init-vs-main, usermap-vs-map entry files, and code-style conventions. Use for any BO3 server- or client-script task (gameplay logic, custom systems, perks/weapons) and for how to structure/format a script. The clientfield bridge to LUI/Lua HUD work is covered here on the GSC/CSC side; for the Lua/LUI authoring side itself, see bo3-hud-lui.
---

# Writing GSC/CSC for Black Ops 3

Server logic is **GSC**, client logic is **CSC** — separate files, separate namespaces, identical language. This skill is the craft; look up exact signatures/KVPs/APIs in **t7kb** (`search` then `get`), and for the conceptual model (scopes, entities, notifies, threads, `undefined`, the finite entity pool, cooperative scheduling) retrieve the "How GSC Scripting Works" guide. t7kb also indexes real, well-structured mod code — retrieve a worked example to see the conventions below applied in practice.

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

When there is **no** hook and you must change stock behavior, you **can and sometimes should override**: copy the stock file into your mod/map `scripts/` at the **same path**, add it to your **`.zone`**, and the engine loads your version instead of the shared one. Caveats: some scripts override only from a **mod**, not a map folder (a common "my copy is ignored"); override the **narrowest** script (overriding low-level shared like `array_shared` breaks its dependents); and an override diverges from stock, so reach for a hook first.

## `init` vs `main` (REGISTER_SYSTEM_EX)

`REGISTER_SYSTEM_EX("name", &init, &main, undefined)` runs `init` then `main`. Split responsibilities:

- **`init`** — everything that must *exist before runtime*: `clientfield::register` (must happen here, before the first network frame), `flag::init`, instantiate the system's state `class`, register callbacks / spawn functions, `#precache` setup.
- **`main`** — *runtime*: wait for the game to start, then the loops, spawns, and behavior.

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

## Code style & conventions

Match these exactly — and when **editing an existing file, don't infer style from it**: the stock scripts and usermap templates use tabs and `( padded )` calls, and mirroring them is the single most common way these rules get ignored. The first two are the most-violated.

- **4 spaces, never tabs.**
- **Always braces.** Never `if (x) doThing();` — write `if (x) { doThing(); }` with the body on its own line(s). Same for loops.
- **No padding inside brackets.** Write `func(arg)` and `arr[i]`, never `func( arg )` or `arr[ i ]` — no space after `(`/`[` or before `)`/`]`.
- **Naming.** `snake_case` for functions and variables; `UPPER_SNAKE` for `#define` constants; **prefix private functions with `_`** (and use the `private` keyword); registered system entry points are often `__init__` / `__main__`.
- **Regions.** Group distinct areas of a file with `/* region NAME */ … /* endregion */`.
- **Debug in dev blocks.** Wrap debug/dev-only code in `/# … #/` — it's compiled out of release. A `#define DEBUG_X 0` flag + `PRINT_DEBUG_X` macro is the alternative when you want a runtime-toggleable print you can ship.
- **IoC over hard calls.** Bind systems by registering callbacks / function pointers (e.g. an optional subsystem hooking a round-state event) rather than calling across them directly — less coupling.
- **Validate before use.** `isdefined()` is the baseline against `undefined`; use the specific predicates (`IsPlayer`, `IsAlive`, `IsArray`, `IsEntity`, `IsFunctionPtr`, …) to check *kind/state*, not just existence.
- **Constants in the `.gsh`**, `#insert`ed — one place to tune.
- **System state in a `class` instance** on `level` (`level.my_system = new my_system();`), not scattered `level.foo_*` fields.
- **`flag::init("name")`** before you wait on or set a flag.
- **Split a feature into focused sub-files** (e.g. logic / audio / fx) + a `_shared.gsc`/`.gsh` for cross-file state and constants, rather than one giant script.

## Don't invent

Stdlib function names, KVPs, and stock system entry points are shipped tokens — confirm exact names against the raw mod-tools install before stating them as fact. If neither t7kb nor the raw install supports a specific function or KVP, don't assert it exists.
