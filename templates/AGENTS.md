# AGENTS.md — Black Ops 3 modding workspace

> Drop this file at the root of your BO3 map/mod project. Any AGENTS.md-aware agent (Claude Code, Codex, OpenCode, recent Cursor) reads it and will use the knowledge base below. Editors that use their own rules file instead (Copilot, Windsurf, Cline, Kiro) — paste this content there; see the t7kb README. Edit freely for your project.

This is a Black Ops III (Treyarch mod tools) modding workspace. You have a local knowledge base of the BO3 modding community available through the **t7kb** MCP server — tools `search` (hybrid keyword + semantic) and `get` (full document by `doc_id`). It covers community wikis, forums, Discord, decompiled engine scripts, YouTube tutorials, and the mod-tools schema files.

_If the `t7kb` tools aren't available, the knowledge base isn't installed yet — see the t7kb README to install it and register the MCP server._

## Use t7kb for BO3 questions

For any non-trivial BO3 modding question — GSC/CSC scripting, Radiant mapping, zombies mechanics, assets, FX, audio, lighting, compile/linker errors — query t7kb **before** answering from memory. The corpus is the authority on what BO3 modding actually contains; your training data is not.

## Query it well

- **Search broad, then narrow.** Issue several short, differently-phrased `search` queries (symptom-side, mechanism-side, exact-jargon-side). The full-text index is conjunctive, so one phrasing misses the long tail.
- **Read full bodies.** `get` the top `doc_id`s — don't answer from snippets.
- **Weigh reliability.** Each result carries a `reliability` score. On conflict, prefer higher-reliability sources, and surface the disagreement when it matters.
- **Cite.** When a claim comes from the kb, name the `source` + `url` so the user can verify.

## Craft essentials (BO3)

Durable conventions that hold regardless of the specific task — verify the specifics in t7kb, but default to these:

- **Reuse the shared stdlib.** `scripts/shared/` has deep helpers (`util`, `array`, `math`, `clientfield`, `flag`, `spawner`, …) — check t7kb for an existing function before writing one.
- **Don't edit stock scripts in place.** Many can't be overridden from the map/mod folder; hook instead (spawn functions, `level.*` function pointers, callbacks) rather than forking.
- **Thread long logic and guard it with `endon`.** Un-threaded long `wait` loops freeze the game / drop connections; persistent threads need `self endon("death")` or `level endon("end_game")`. Mind `self` vs `level` scope.
- **Errors: make them visible first, then load the skill.** In Launcher → dvars set `dev 2` and `logfile 1`, reproduce, and read the exact message before theorizing. The method for working it — telling compile vs linker vs unresolved-external vs runtime apart, getting real line numbers, the S.R.E. call stack, the dvar/devgui toolkit — is the **bo3-debugging** skill: **load it rather than diagnosing from this bullet** whenever it is available.
- **Read the log file, don't ask for a screenshot.** `logfile` writes the whole game console to **`console_mp.log`** at the `fs_game` root — `mods/<modname>/console_mp.log` for a mod, the **BO3 root** for a usermap (check both, take the newest; it's rewritten each session). Grep it for `script error` / `Call stack` / `Could not find`. Hard crash with no error: `crashes.log` at the BO3 root.

## Code style (GSC/CSC) — match exactly, don't copy the file you're editing

The stock scripts and usermap templates predate these conventions (tabs, `( padded )` calls) — **do not mirror them.** The full craft (hooks vs override, threading/scope, clientfields, init/main, entry files) is in the **bo3-scripting** skill: **load it before writing or editing any GSC/CSC** (and its siblings for mapping, HUD/Lua, assets, zombies AI, debugging). When skills aren't available, these five are the floor and still apply:

- **4 spaces, never tabs.**
- **No padding inside brackets** — `func(arg)`, `arr[i]`, `if (x)`; never `func( arg )` or `arr[ i ]`.
- **Always braces**, body on its own line — never `if (x) doThing();`.
- **Naming** — `snake_case` functions/vars, `UPPER_SNAKE` for `#define`, `_`-prefix + `private` keyword for file-local helpers.
- **Tunables in a `#insert`ed `.gsh`** — not magic numbers, not config dvars.

## Verify shipped tokens against ground truth

The corpus is a starting point, not the final authority. For anything Treyarch **shipped** — exact function names, entity KVPs, asset fields, error strings, file paths — confirm against the **raw mod-tools install** (the game's own files under your BO3 root) before stating it as fact. Decompiled and community sources can be paraphrased or subtly wrong; the shipped files are ground truth. Drop any claim you can't ground in either the corpus or the raw install.

### When the raw install isn't available

The raw mod-tools install is the preferred ground truth, but it may be absent (not installed, on another machine, or a headless run). Do not silently fall back to low-reliability sources:

- **Detect and disclose.** If you cannot locate the install, say so in your answer, and mark any shipped-token claim (function name, KVP, asset field, error string, path) as corroborated by community sources only — not verified against shipped files.
- **Last-resort web supplement.** When the kb is thin and the install is unavailable, a targeted web search may fill gaps. Rank it strictly below the kb and the install, never as ground truth. Prefer higher-reliability sources (e.g. UGX, resolved/accepted threads) over random posts, keep the "may be paraphrased or subtly wrong" caution, and state in the answer what was verified versus merely corroborated.
- **Ordering.** Always: kb → raw install → web. Drop any claim you cannot ground in at least one of these.

## Don't invent

BO3 has its own vocabulary. Cross-game intuitions (other CoD titles, generic engine/Unity/Unreal terms) are usually wrong here. If neither t7kb nor the raw install supports a function, KVP, or concept, do not assert it exists.
