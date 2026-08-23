---
name: bo3-localization
description: How to get translated on-screen text into Black Ops 3 — `.str` files under `share/raw/<lang>/localizedstrings/`, the `<STRFILE>_<REFERENCE>` naming rule that decides the name GSC passes, `#precache("triggerstring")` vs `#precache("string")`, `SetHintString(&"REF")`, `localize,` zone entries, `linkerflag,noloc`, and the per-language linker pass that writes `en_<map>.ff` / `fr_<map>.ff`. Use when a trigger prompt or HUD string prints its raw reference name (`MY_HINT_NAME`) instead of the words, when adding or translating a language for a map or mod, when one language is stale after a rebuild, or when authoring any player-visible text. Distinct from bo3-hud-lui (Lua/LUI widgets, and the `^1`-`^9` color codes inside a string) and bo3-compiling (the build pipeline as a whole) — this is the string-asset craft and its silent failures.
---

# Localizing text in Black Ops 3

**A localized string is an asset with a compound name, not a literal you type.** The name GSC passes is assembled from the `.str` filename *and* the `REFERENCE` inside it, and every failure in this craft is silent — the game falls back to drawing the reference name on screen, with a clean link and an empty `.errorlog`. Look exact shipped refs, KVPs, and column names up in **t7kb** (`t7kb:search` then `t7kb:get`); confirm anything Treyarch shipped against the raw mod-tools install.

## The reference is the filename joined to the `REFERENCE` line

A `.str` file holds one or more entries:

```
FILENOTES "Teddy bear easter egg (zm_michael)"

REFERENCE PICKUP
LANG_ENGLISH "^3[{+activate}]^7 Pick up the teddy bear"

ENDMARKER
```

**The name you pass is `<FILENAME>_<REFERENCE>`, uppercased — not the `REFERENCE` on its own:**

| File | `REFERENCE` | Name GSC passes |
|---|---|---|
| `zipline.str` | `USE` | `&"ZIPLINE_USE"` |
| `room_of_thanks.str` | `PLAY` | `&"ROOM_OF_THANKS_PLAY"` |
| `meteor.str` | `ACTIVATE` | `&"METEOR_ACTIVATE"` |

Get it wrong and **the game draws the raw reference name where the words should be** — no link error, empty `.errorlog`, nothing naming the culprit. That silent fallback makes the name the first thing to check on any prompt showing gibberish, ahead of the zone file and ahead of the precache.

It also means the `REFERENCE` should stay **short**, because the filename already namespaces it: `zm_michael_teddy.str` wants `REFERENCE PICKUP`, giving `ZM_MICHAEL_TEDDY_PICKUP`. Writing `REFERENCE TEDDY_HUNT_PICKUP` there yields `ZM_MICHAEL_TEDDY_TEDDY_HUNT_PICKUP`, which is what a doubled-looking name in a log is telling you.

Keep the resolved name in the feature's `.gsh` as a `*_LOCALIZED` constant so it is written once, and use it macro-style in both places — the `&MACRO` form compiles:

```gsc
#define TEDDY_HINT_LOCALIZED "ZM_MICHAEL_TEDDY_PICKUP"

#precache("triggerstring", TEDDY_HINT_LOCALIZED);
trigger SetHintString(&TEDDY_HINT_LOCALIZED);
```

## A trigger hint needs `triggerstring`, not `string`

A trigger's hint reaches the client as a **precached trigger-string index**, which is why it has a precache type of its own. The GSC that calls `SetHintString(&"<NAME>")` must also carry `#precache("triggerstring", "<NAME>")`. `#precache("string", "<NAME>")` is the separate form for other UI text; `_zm_craftables.gsc` registers some refs under both, and is the shipped pattern to copy for a hint. (`_zm_utility.gsc`'s un-precached `&"ZOMBIE_NAVCARD_PICKUP"` is a BO2-era leftover — don't read it as proof the precache is optional.)

## Each language is a separate file and a separate linker pass

Languages are **parallel trees**, not several `LANG_` lines in one file — the same filename and the same `REFERENCE` repeated per language:

```
share/raw/english/localizedstrings/zm_michael_teddy.str   → LANG_ENGLISH "..."
share/raw/french/localizedstrings/zm_michael_teddy.str    → LANG_FRENCH  "..."
```

Write them UTF-8; accented text (`Arrêter`) is stored as UTF-8 in the shipped files.

One linker pass covers one language: it writes the language-neutral `<map>.ff` **and** that language's `<lang>_<map>.ff`, leaving every other language's fastfile at its previous content. So a map translated into French after an English-only build keeps serving stale French text until you link with `-language french` too — link once per language you ship. See **bo3-compiling** for the pass itself and the rest of what it rewrites.

## A `localize,` zone entry is not what makes a string resolve

Reaching for a missing `localize,<file>` entry to explain an unresolved string is a wrong turn. **A usermap build sweeps `share/raw/<lang>/localizedstrings/` wholesale** — verified on a real build, a map with no `localize` line anywhere in its `.zone` or its included `.zpkg`s still pulls its `.str` files into the build deps. Some maps carry the entry and it is harmless, but it is not the failing link; check the name first.

`linkerflag,noloc` is the opposite lever: it skips generating the per-language loc files (`en_`, `fr_`, …) entirely, saving a fastfile slot when nothing in the map or mod is localized. Don't reach for it if you rely on localized strings anywhere. Notifies are unaffected either way.

## Diagnosing a string that won't resolve

Work in this order — it is cheapest-first, and the first check catches most of them:

1. **Name.** Does `<FILENAME>_<REFERENCE>` match the string GSC passes, character for character? Fix here before touching anything else.
2. **Precache.** For a trigger hint, is `#precache("triggerstring", "<NAME>")` present in the calling script?
3. **Language pass.** Was the map linked for the language you're playing in? A stale `<lang>_<map>.ff` shows the previous build's text, not the reference name.
4. **The file reached the build.** `usermaps/<map>/zone_source/all/assetinfo/<map>.deps` lists every source file consumed — grep it for your `.str` path. Absent means the file isn't where the linker sweeps.

## Don't invent

Reference names, precache types, and `linkerflag` values are shipped tokens. Confirm a `REFERENCE` against the actual `.str` file and a stock ref against `share/raw/scripts/` before asserting either exists — a plausible-looking localized name that nothing defines fails exactly like a typo, and looks identical on screen.
