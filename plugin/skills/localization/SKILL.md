---
name: localization
description: How to get translated on-screen text and audio into Black Ops 3 — `.str` files under `share/raw/{lang}/localizedstrings/`, the `STRFILE_REFERENCE` naming rule that decides the name GSC passes, `#precache("triggerstring")` vs `#precache("string")` and per-value precaches for `&&1` hints, `SetHintString(&"REF")`, when a `localize,` zone line is required, `linkerflag,noloc`, `soundloc_assets`, and the per-language linker pass that writes `en_mapname.ff` / `fr_mapname.ff`. Use when a trigger prompt, HUD label or `Engine.Localize` string prints its raw reference name (`MY_HINT_NAME`, `ZOMBIE_ROCKET_HINT`) instead of the words, a hint cost `&&1` shows blank or crashes, one language is stale after a rebuild, snd_convert reports `File is both localized and not localized`, or when adding a language or authoring player-visible text. Distinct from t7kb:hud-lui (the Lua widgets and `^1`-`^9` color codes) and t7kb:compiling (the build as a whole).
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

A trigger's hint reaches the client as a **precached trigger-string index**, which is why it has a precache type of its own. The GSC that calls `SetHintString(&"<NAME>")` must also carry `#precache("triggerstring", "<NAME>")`. `#precache("string", "<NAME>")` is the separate form for other UI text; `_zm_craftables.gsc` registers some refs under both, and is the shipped pattern to copy for a hint.

**A hint with arguments is precached once per distinct value, and the value is a quoted string.** `&&1`, `&&2`, … in the text are filled in order by `SetHintString`'s extra arguments, and Treyarch's `zm_giant.gsc` precaches each combination it uses: `#precache("triggerstring", "ZOMBIE_PERK_QUICKREVIVE", "500");` then the same with `"1500"`. Pass the cost as `"500"`, not `500` — a precache takes strings, and an unquoted int crashes (community guide, t7kb 0.70).

**Why localize at all, beyond translation: every distinct final string takes a configstring slot.** `SetHintString(&"REF", n)` with a precached ref reuses one entry; concatenating a changing value into a literal (`"Cost: " + n`) mints a new one each update until the fixed-size table overflows (community reports; one of them a stock patch). Keep dynamic parts in `&&1` arguments. The `triggerstring` table has its own ceiling too — `Exceeded '250' items for type 'triggerstring'` (community). (`_zm_utility.gsc`'s un-precached `&"ZOMBIE_NAVCARD_PICKUP"` is a BO2-era leftover — don't read it as proof the precache is optional.)

## Each language is a separate file and a separate linker pass

Languages are **parallel trees**, not several `LANG_` lines in one file — the same filename and the same `REFERENCE` repeated per language:

```
share/raw/english/localizedstrings/zm_michael_teddy.str   → LANG_ENGLISH "..."
share/raw/french/localizedstrings/zm_michael_teddy.str    → LANG_FRENCH  "..."
```

Write them UTF-8; accented text (`Arrêter`) is stored as UTF-8. A language you don't actually translate can reuse the English text by writing `#same` in its entry (community guide, t7kb 0.70).

**Localized audio is the same idea on the sound side:** the per-language WAVs go under `soundloc_assets/<lang>/`, not `sound_assets/`, and a file present in both makes snd_convert fail with `File is both localized and not localized.` (**verified** in `snd_convert.exe`; that every shipped locale folder must exist is community-reported). Aliases themselves are **t7kb:atmosphere**'s.

One linker pass covers one language: it writes the language-neutral `<map>.ff` **and** that language's `<lang>_<map>.ff`, leaving every other language's fastfile at its previous content. So a map translated into French after an English-only build keeps serving stale French text until you link with `-language french` too — link once per language you ship. See **t7kb:compiling** for the pass itself and the rest of what it rewrites.

## A `localize,` line is optional for strings GSC uses — and required for strings only Lua or a HUD asks for

**The linker pulls in a `.str` because compiled script references one of its strings**, not because it sits in the folder. Verified on a real build: a map with no `localize,` line in its `.zone` or `.zpkg`s got its `zm_michael_teddy.str` into `<map>.deps` — while the other `.str` files in the same `localizedstrings/` folder were **not** pulled in, and the linker appended `localize,zm_michael_teddy` to the generated loc zone itself. So for a hint or HUD string your GSC passes, a missing `localize,` line is not the failing link; check the name first.

The corollary is the real trap: **a string no compiled GSC references is never found that way.** Text asked for only by Lua (`Engine.Localize("…")`), by a stock HUD widget you loaded, or by an asset prints its raw reference — `ZOMBIE_ROCKET_HINT`, `ZM_CASTLE_TRAM_TOKEN_POWERUP` — with a clean link and an empty `.errorlog`. For those, `localize,<file>` **is** the fix; for a stock ref, author a `.str` whose filename reproduces its prefix (`zm_castle.str` with `REFERENCE TRAM_TOKEN_POWERUP`). (Two independent community fixes, t7kb 0.70, match the mechanism.)

`linkerflag,noloc` is the opposite lever: it skips generating the per-language loc files (`en_`, `fr_`, …) entirely, saving a fastfile slot when nothing in the map or mod is localized. Don't reach for it if you rely on localized strings anywhere. Notifies are unaffected either way.

## Diagnosing a string that won't resolve

Work in this order — it is cheapest-first, and the first check catches most of them:

1. **Name.** Does `<FILENAME>_<REFERENCE>` match the string GSC passes, character for character? Fix here before touching anything else.
2. **Precache.** For a trigger hint, is `#precache("triggerstring", "<NAME>")` present in the calling script?
3. **Language pass.** Was the map linked for the language you're playing in? A stale `<lang>_<map>.ff` shows the previous build's text, not the reference name.
4. **The file reached the build.** `usermaps/<map>/zone_source/all/assetinfo/<map>.deps` lists every source file consumed — grep it for your `.str` path. Absent means no compiled script referenced it — add a `localize,<file>` line (the Lua/HUD case above) or check the file's location.

## Don't invent

**No `.str` source under `share/raw/*/localizedstrings/` is Treyarch's** — the mod tools ship none; what's there is community dumps (Cyph3r's `zm.str`/`zombie.str`) or your own. To check a real stock reference name, grep a dump of the shipped string table (HydraX's exported `localizedstrings.str`) or the stock scripts, not those files.

Reference names, precache types, and `linkerflag` values are shipped tokens. Confirm a `REFERENCE` against the actual `.str` file and a stock ref against `share/raw/scripts/` before asserting either exists — a plausible-looking localized name that nothing defines fails exactly like a typo, and looks identical on screen.
