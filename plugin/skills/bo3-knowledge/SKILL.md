---
name: bo3-knowledge
description: How to search the t7kb knowledge base for Black Ops 3 / BO3 / Treyarch mod-tools questions, and the querying/reliability/grounding method every other bo3-* skill builds on. Use as the general entry point for topics without a dedicated craft skill (perks, wonder weapons, easter eggs, community packs, general lore/mechanics). For GSC/CSC, debugging, Radiant mapping, building/compiling, assets/porting, zombies AI, HUD/Lua, or atmosphere specifically, prefer bo3-scripting, bo3-debugging, bo3-mapping, bo3-compiling, bo3-assets, bo3-zombies-ai, bo3-hud-lui, or bo3-atmosphere instead.
---

# Answering BO3 modding questions with t7kb

You have a local knowledge base of the Black Ops 3 modding community via the **t7kb** MCP server — tools `search` (hybrid keyword + semantic) and `get` (full document by `doc_id`). For any non-trivial BO3 modding question, query it before answering from memory; the corpus is the authority on what BO3 modding actually contains, your training data is not.

_If the `t7kb` tools aren't available, the knowledge base isn't installed — run `/t7kb:setup` first._

_If you're working under a BO3 mod-tools root (has `raw/`, `share_raw/`, `usermaps/`, or `mods/`) and there's no `AGENTS.md`/`CLAUDE.md` at that root yet, offer to drop one in (`/t7kb:setup` step 3 fetches the primer and, for Claude Code, a `CLAUDE.md` that imports it) — one file at the root covers every map/mod under it, and a per-map/mod file still layers on top for that project's own conventions._

## Query it well

- **Search broad, then narrow.** Issue several short, differently-phrased `search` queries (symptom-side, mechanism-side, exact-jargon-side). The full-text index is conjunctive, so a single phrasing misses the long tail.
- **Read full bodies.** `get` the top `doc_id`s — don't answer from snippets.
- **Weigh reliability.** Each result carries a `reliability` score. On conflict, prefer higher-reliability sources and surface the disagreement when it matters.
- **Cite.** When a claim comes from the kb, name the `source` + `url`.

## Verify shipped tokens against ground truth

The corpus is a starting point, not the final authority. For anything Treyarch **shipped** — exact function names, entity KVPs, asset fields, error strings, file paths — confirm against the raw mod-tools install (the game's own files under the BO3 root) before stating it as fact. Decompiled and community sources can be paraphrased or subtly wrong; the shipped files are ground truth. Drop any claim you can't ground.

Check the project's `AGENTS.md`/`CLAUDE.md` first for a "Raw mod-tools root" fact (`/t7kb:setup` records it there when found) — if it's there, use that path directly instead of searching for the install.

### When the raw install isn't available

The raw mod-tools install is the preferred ground truth, but it may be absent (not installed, on another machine, or a headless run). Do not silently fall back to low-reliability sources:

- **Detect and disclose.** If you cannot locate the install, say so in your answer, and mark any shipped-token claim (function name, KVP, asset field, error string, path) as corroborated by community sources only — not verified against shipped files.
- **Last-resort web supplement.** When the kb is thin and the install is unavailable, a targeted web search may fill gaps. Rank it strictly below the kb and the install, never as ground truth. Prefer higher-reliability sources (e.g. UGX, resolved/accepted threads) over random posts, keep the "may be paraphrased or subtly wrong" caution, and state in the answer what was verified versus merely corroborated.
- **Ordering.** Always: kb → raw install → web. Drop any claim you cannot ground in at least one of these.

## Don't invent

BO3 has its own vocabulary. Cross-game intuitions (other CoD titles, generic engine terms) are usually wrong here. If neither t7kb nor the raw install supports a function, KVP, or concept, don't assert it exists.
