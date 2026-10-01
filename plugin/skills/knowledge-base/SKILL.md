---
name: knowledge-base
description: How to search the t7kb knowledge base for Black Ops 3 / BO3 / Treyarch mod-tools questions, and the querying/reliability/grounding method every other bo3-* skill builds on. Use as the general entry point for topics without a dedicated craft skill (perks, wonder weapons, easter eggs, community packs, general lore/mechanics). For GSC/CSC, debugging, Radiant mapping, building/compiling, assets/porting, animation, cross-generation animation retargeting, zombies AI, moving platforms and vehicles that carry players or AI (buses, trains, elevators, tanks), HUD/Lua, particle-FX editing, atmosphere, localized/translated on-screen text, or cross-referencing other Call of Duty titles (GSC dumps / porting sources / gsc-tool) specifically, prefer t7kb:scripting, t7kb:debugging, t7kb:mapping, t7kb:compiling, t7kb:assets, t7kb:animation, t7kb:anim-retarget, t7kb:zombies-ai, t7kb:moving-platforms, t7kb:hud-lui, t7kb:fx-editing, t7kb:atmosphere, t7kb:localization, or t7kb:crossref instead.
---

# Answering BO3 modding questions with t7kb

You have a local knowledge base of the Black Ops 3 modding community via the **t7kb** MCP server — tools `t7kb:search` (hybrid keyword + semantic) and `t7kb:get` (full document by `doc_id`). For any non-trivial BO3 modding question, query it before answering from memory; the corpus is the authority on what BO3 modding actually contains, your training data is not.

_If the `t7kb` tools aren't available, the knowledge base isn't installed — run `/t7kb:setup` first._

_If you're working under a BO3 mod-tools root (has `raw/`, `share_raw/`, `usermaps/`, or `mods/`) and there's no `AGENTS.md`/`CLAUDE.md` at that root yet, offer to drop one in (`/t7kb:setup` step 3 fetches the primer and, for Claude Code, a `CLAUDE.md` that imports it) — one file at the root covers every map/mod under it, and a per-map/mod file still layers on top for that project's own conventions._

## Query it well

- **Search broad, then narrow.** Issue several short, differently-phrased `t7kb:search` queries (symptom-side, mechanism-side, exact-jargon-side). The full-text index is conjunctive, so a single phrasing misses the long tail.
- **Narrow by source when you know the kind of answer.** `source` takes a group — `api` (a function's exact signature and parameters), `scripts` (how Treyarch's own code does it), `docs` (official docs, asset and entity schemas), `wiki` and `forums` (tutorials), `discord`, `video` — or several (`wiki,forums`). Unfiltered, Discord threads (70% of the corpus) can fill the whole top 10: `clientfield register set lua` returns no API page until `source: api`, where `RegisterClientField` is third. Search unfiltered first for a symptom or an error message, where the threads are often the answer.
- **Read full bodies.** `t7kb:get` the top `doc_id`s — don't answer from snippets. A long document (a whole script, GDT or transcript) comes a page at a time, ending with the `offset` for the next part. To reach the passage the search matched, pass `find` with a phrase from its snippet: the page then starts at it, wherever it sits — rather than paging through everything.
- **Weigh reliability.** Each result carries a `reliability` score. On conflict, prefer higher-reliability sources and surface the disagreement when it matters.
- **Cite.** When a claim comes from the corpus, name the `source` + `url`.

## Verify shipped tokens against ground truth

The corpus is a starting point, not the final authority. For anything Treyarch **shipped** — exact function names, entity KVPs, asset fields, error strings, file paths — confirm against the raw mod-tools install (the game's own files under the BO3 root) before stating it as fact. Decompiled and community sources can be paraphrased or subtly wrong; the shipped files are ground truth. Drop any claim you can't ground.

Check the project's `AGENTS.md`/`CLAUDE.md` first for a "Raw mod-tools root" fact (`/t7kb:setup` records it there when found) — if it's there, use that path directly instead of searching for the install.

### When the raw install isn't available

The raw mod-tools install is the preferred ground truth, but it may be absent (not installed, on another machine, or a headless run). Do not silently fall back to low-reliability sources:

- **Detect and disclose.** If you cannot locate the install, say so in your answer, and mark any shipped-token claim (function name, KVP, asset field, error string, path) as corroborated by community sources only — not verified against shipped files.
- **Last-resort web supplement.** When the corpus is thin and the install is unavailable, a targeted web search may fill gaps. Rank it strictly below the corpus and the install, never as ground truth. Prefer higher-reliability sources (e.g. UGX, resolved/accepted threads) over random posts, keep the "may be paraphrased or subtly wrong" caution, and state in the answer what was verified versus merely corroborated.
- **Ordering.** Always: corpus → raw install → web. Drop any claim you cannot ground in at least one of these.

## Community packs are t7kb lookups, not engine behaviour

Perk, box, Pack-a-Punch, trap and craftable packs (Harry Bo21's `hb21_*`, Sphynx's `spx_*`, and the like) install as `include,` zone lines plus copied scripts; their rules — unique trap letters, box numbering — belong to the pack, not to BO3. Answer from the pack's own docs in t7kb, and don't generalise a pack's rule into a claim about stock code. Their scripts often sit inside the install (`share/raw/scripts/Sphynx/`, …) — that doesn't make them ground truth either.

## When the corpus or a skill turns out wrong

If a session proves a t7kb skill wrong, stale, or missing a trap that cost real time, finish the user's task first, then offer once to send the fix upstream — **t7kb:contribute** is how.

## Don't invent

BO3 has its own vocabulary. Cross-game intuitions (other CoD titles, generic engine terms) are usually wrong here. If neither t7kb nor the raw install supports a function, KVP, or concept, don't assert it exists.
