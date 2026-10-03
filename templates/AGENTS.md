# AGENTS.md — Black Ops 3 modding workspace

> Drop this file at the root of your BO3 mod-tools install (the folder holding `usermaps/`, `mods/`, `share/`) — one copy covers every map and mod underneath, and a per-project `AGENTS.md` layers on top. AGENTS.md-aware agents (Claude Code via a `CLAUDE.md` that imports it, Codex, OpenCode, recent Cursor) read it. Editors with their own rules file (Copilot, Windsurf, Cline, Kiro): paste this there; see the t7kb README. Edit freely.

This is a Black Ops III (Treyarch mod tools) modding workspace. The **t7kb** MCP server gives you: a local knowledge base of the BO3 modding community (`search`, hybrid keyword + semantic; `get`, a full document by `doc_id`) covering wikis, forums, Discord, decompiled engine scripts, YouTube tutorials and the mod-tools schema files; the `gdt_*` tools, to read, check and edit this install's GDTs; and `build`, to compile, light and link a map or mod without the Launcher.

_If the `t7kb` tools aren't available, it isn't installed yet — see the t7kb README._

## Use t7kb for BO3 questions

For any non-trivial BO3 modding question — GSC/CSC, Radiant mapping, zombies, assets, FX, audio, lighting, compile/linker errors — query t7kb **before** answering from memory. The corpus is the authority on what BO3 modding contains; your training data is not. This holds mid-task too, not only for a question or an error: before you assert how the pipeline behaves (what the linker packs, how a zone, assetlist or GDT works) or ask the user to confirm it, search t7kb and load the skill that owns it.

## Query it well

- **Search broad, then narrow.** Issue several short, differently-phrased `search` queries (symptom-side, mechanism-side, exact-jargon-side). The full-text index is conjunctive, so one phrasing misses the long tail.
- **Narrow by source when you know the kind of answer.** `source` takes a group — `api` (exact function signatures), `scripts` (Treyarch's own code), `docs` (official docs, asset and entity schemas), `wiki` and `forums` (tutorials), `discord`, `video` — or several (`wiki,forums`). Unfiltered, Discord threads (70% of the corpus) can fill the whole top 10. For a symptom or error message, search unfiltered first: the threads are often the answer.
- **Read full bodies.** `get` the top `doc_id`s; don't answer from snippets. A long document comes a page at a time, ending with the `offset` for the next part. Pass `find` with a phrase from the snippet to start the page at the matched passage.
- **Weigh reliability.** Each result has a `reliability` score. On conflict prefer higher-reliability sources, and surface the disagreement when it matters.
- **Cite.** Name the `source` + `url` for any claim from the kb.

## Use the GDT and build tools instead of editing and building by hand

- **Edit GDTs through `gdt_edit`, not as text.** It validates against the install's `.awi` schema and material techsets, refuses stock GDTs, and is a **dry run unless you pass `write`**: show the user the changes and issues, then write. `gdt_get` / `gdt_find` read an asset, `gdt_schema` lists what an asset type (or material type's texture slots) accepts, `gdt_refs` lists what uses an asset before you rename or delete it.
- **`gdt_check` a GDT before building it.** It reports what would otherwise surface one link error at a time: missing or wrong-typed references, exports and textures missing on disk, duplicates, a surface type left on `<error>`.
- **Build with `build`, then read its report.** It runs gdtdb → cod2map → light → linker and returns each stage's status and first actionable error; `stages: link` alone suffices after a script or asset change. The linker exits non-zero on mere warnings, so `link` can report failure on a fast file that built fine: the verdict is `zone_source/all/assetinfo/<map>.errorlog` (and `<map>.csv`), not the exit code. `build` and `gdt_edit` are the only tools that change files.

## Craft essentials (BO3)

Verify specifics in t7kb, but default to these:

- **Reuse the shared stdlib.** `scripts/shared/` has helpers (`util`, `array`, `math`, `clientfield`, `flag`, `spawner`, …); check t7kb for an existing function before writing one. (**t7kb:scripting**.)
- **Hook before you override; never edit stock scripts in place.** Stock systems expose seams (spawn functions, `level.*` function pointers, callbacks). To replace a stock script, copy it into your map/mod and comment its line out of the assetlist CSV that contributes it, in usermaps and mods alike. Load **t7kb:scripting** and **t7kb:compiling** for the method.
- **Thread long logic and guard it with `endon`.** Un-threaded long `wait` loops freeze the game / drop connections; persistent threads need `self endon("death")` or `level endon("end_game")`. Mind `self` vs `level` scope. (**t7kb:scripting**.)
- **Errors: make them visible first.** In Launcher → dvars set `developer 2` and `logfile 1`, reproduce, and read the exact message before theorizing. Then load **t7kb:debugging** (compile vs linker vs unresolved-external vs runtime, line numbers, the S.R.E. call stack, the dvar/devgui toolkit) rather than diagnosing from here.
- **Read the log file, don't ask for a screenshot.** `logfile` writes the game console to **`console_mp.log`** at the `fs_game` root: `mods/<modname>/console_mp.log` for a mod, the **BO3 root** for a usermap (check both, take the newest). Map reloads in one process append sessions to the same file, so cut to the last `Game Initialization` before grepping. A run that never set `logfile` (any headless launch) leaves the *previous* run's file untouched, so check the `logfile opened on` header and the mtime. Grep for `script error` / `Call stack` / `Could not find`. A hard crash with no error: `crashes.log` at the BO3 root.

## Code style (GSC/CSC) — match exactly, don't copy the file you're editing

Stock scripts and usermap templates predate these conventions (tabs, `( padded )` calls); **do not mirror them.** Load **t7kb:scripting** before writing or editing any GSC/CSC. Without skills, these five apply:

- **4 spaces, never tabs.**
- **No padding inside brackets** — `func(arg)`, `arr[i]`, `if (x)`.
- **Always braces**, body on its own line — never `if (x) doThing();`.
- **Naming** — `snake_case` functions/vars, `UPPER_SNAKE` for `#define`, `_`-prefix + `private` keyword for file-local helpers.
- **Tunables in a `#insert`ed `.gsh`** — not magic numbers, not config dvars.

## Verify shipped tokens against ground truth

For anything Treyarch **shipped** — function names, entity KVPs, asset fields, error strings, file paths — confirm against the **raw mod-tools install** (the game's own files under your BO3 root) before stating it as fact. Decompiled and community sources can be paraphrased or subtly wrong. If the install is unavailable, say so and mark such claims as corroborated by community sources only. A targeted web search is a last resort, ranked below the kb and the install (prefer UGX and resolved threads). Order: kb → raw install → web. Drop any claim you can't ground in one of them.

## Don't invent

BO3 has its own vocabulary; cross-game intuitions (other CoD titles, Unity/Unreal terms) are usually wrong here. If neither t7kb nor the raw install supports a function, KVP or concept, don't assert it exists.
