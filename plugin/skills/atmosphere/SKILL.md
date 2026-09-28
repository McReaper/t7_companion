---
name: atmosphere
description: How to make a Black Ops 3 map sound and feel right — sound aliases and snd_convert, ambient rooms and reverb (`ambient_package` triggers, the `.szc` MapFile), sun/sky/SSI and reflection probes, visionsets for runtime color grading, the four fog sections (world/lit/sun/atmospheric) and their two placeable volumes, weather (rain/snow/lightning), and exploders. Use when an alias plays silently or not at all, the link log says `wav is not 48k sample rate` / `wav is not 16 bit sample depth` / `'FileSpec' cannot contain a space` / `no files for filespec`, ambient-room reverb never switches between spaces or `ambientgeometry.json` is empty, fog or the sky/sun looks wrong or ignores an edit, a visionset asserts or won't apply, or rain/snow leaks indoors or doesn't play. Distinct from t7kb:fx-editing (building the `.efx` particle effect itself) — this is wiring and playing existing sound, light, fog and effects for mood.
---

# Atmosphere: sound, lighting, fog & FX in Black Ops 3

**Atmosphere fails quietly by design** — an alias that never plays, a reverb that never switches, a sun edit that never shows — because every piece is baked or converted at a build step you don't watch. The craft is knowing which build step owns each piece and where it reports. Look up exact CSV columns, KVPs, and error strings in **t7kb** (`t7kb:search` then `t7kb:get`). Ground on files dated with the install, not the community packs that commonly overwrite `share/raw/sound/` and the Radiant templates.

## Sound: aliases are the unit, not WAV files

A **sound alias** is a named row in a CSV under `share/raw/sound/aliases/` — script and triggers reference the alias name, never a WAV path. Add your CSV as an `ALIAS` source in the map's `.szc`, `Name` matching the CSV's base filename. Rather than filling ~80 columns by hand, copy a **stock row** (from t7kb's alias dumps — the CSVs under `share/raw/sound/aliases/` on a modded install are usually pack files) whose behaviour already matches (a `BUS_MUSIC`/`2d`/`streamed` row for music, a `BUS_FX`/`3d`/`loaded` row for a one-shot) and change `Name` and `FileSpec`; a `Template` column names a template alias for the boilerplate (` missing template alias '` in the log means that template isn't loaded).

- **`FileSpec` resolves from `sound_assets/`, not from the CSV's folder** — written with backslashes like the shipped rows, so `mus\zm\nacht\mus_undone.wav` means `sound_assets/mus/zm/nacht/mus_undone.wav`. Pointing it under `share/raw/sound/` is the usual cause of `no files for filespec`. No spaces in `Name` or the path (`'FileSpec' cannot contain a space`), and no absolute path (`Cannot use rooted path as filespec`).
- **The WAV format is enforced — and reported, if you read the link log.** snd_convert rejects anything but 48 kHz, 16-bit, PCM, mono or stereo, with literal messages: `wav is not 48k sample rate`, `wav is not 16 bit sample depth`, `wav is not format 1 or 0xFFFE` (float/ADPCM), `wav is not one or two channels`. Grep the link output for `wav is not` before assuming anything else. (**Verified** from `snd_convert.exe`'s own strings.) `Missing source checksum` and `Object reference not set to an instance of an object` are reported harmless (community).
- **Aliases are rebuilt at the *link* step** — no separate sound build; a script-only relink picks up CSV edits.
- **`Storage` matters for timing.** A `streamed` one-shot can silently fail to fire at a precise instant (the first frame of a scripted sequence) because the stream isn't ready; make must-play one-shots `loaded`, keep long loops/ambience `streamed`.
- **`user_aliases.csv` is overwritten on mod-tools updates** — make your own CSV (same header) as its own `.szc` source.
- **Variants go through sound contexts**, not naming: `ringoff_plr` (indoor/outdoor/underwater) and `water` select the variant, with the context value set by the ambient room the listener is in (community, Ardivee's wiki 0.70).
- Thousands of aliases and ~1,150 ZM / ~470 MP FX already ship (corpus lists) — `t7kb:search` before authoring a new one. Localized voice lines go under `soundloc_assets/` (**t7kb:localization**).

## Ambient rooms: `ambient_package` triggers, baked by snd_convert — not `ambient_room`

Per-space ambience and reverb is **baked**: snd_convert reads the `.map` named in the `.szc`'s `MapFile`, writes `usermaps/<map>/sound/zone/<map>.ambientgeometry.json`, and the engine switches rooms itself at runtime (calling `CodeCallback_SoundSetAmbientState`). Each of those links fails silently:

1. **The triggers are `trigger_multiple` with `targetname` `ambient_package`**, plus `script_ambientroom` `<room>` (a row in your `AMBIENT` CSV source), `script_ambientpriority` for overlaps, and `CLIENTSIDE_TRIGGER` checked. That is what Treyarch's own `zm_giant_audio.map` prefab uses (29 of them) and what snd_convert's parser looks for — the string `ambient_room` does not appear in it at all.
2. **`"MapFile"` in the `.szc` must name the map** (relative to `map_source`, backslashes doubled: `"zm\\<map>.map"`). The stock usermap templates ship it **empty**, so nothing is baked and only the `DefaultRoom` ever plays.
3. **Triggers inside a `misc_prefab` are invisible to it** — the parser doesn't follow prefabs (Treyarch's `zm_giant.map` itself holds zero ambient triggers; they live in the audio prefab). Put the triggers in the map `MapFile` names.
4. **Verify the bake**: `ambientgeometry.json` must have a non-empty `"Triggers"` array. snd_convert is reported flaky (several links before it appears); ` invalid hull on trigger ` is its brush error.

`targetname ambient_room` + `CLIENTSIDE_TRIGGER` is a **different, community** mechanism: it only works with Ardivee's `_ambient_room.csc` (which finds the triggers client-side and calls `forceambientroom`), plus that script's zone lines. Pick one system; mixing the two recipes is why rooms "never switch". (**Verified in the install**: `snd_convert.exe` strings, the zm_giant prefab, the stock `.szc` templates.)

## Sun, sky & SSI: the primary light source

A **`volume_sun`** holds up to four light states — its `ssi1`–`ssi4` KVPs (Radiant groups them "Light State 1–4") — plus `shadowSplitDistance` (default 2000) for the shadow cascade. Each **`ssi`** APE asset carries sun `pitch`/`yaw`/`colorSRGB` and `stops` (sun brightness). The skybox is a separate chain (image → material → model) assigned in the SSI; **`skyRotation` (alignment) and `skyStops` (brightness) are fields on the sky *material*, not the SSI** (`deffiles/material.awi`, `ssi.awi`).

Two "I changed it and nothing happened" traps:

- **`ssiN_runtime_override` wins in game, the baked `ssiN` wins in the probes.** With an override set, you see the override live, but reflection probes keep the baked SSI unless `override_reflections` is on (`t7.def.json`). Edit the one that's actually showing.
- **`pitch`, `yaw`, `skyStops` and the bounce part of `stops` only change after a rebake** (Light stage, **t7kb:compiling**) — a relink alone shows the old sun (Treyarch's SunSky doc, per the corpus).

## Lighting & reflections

**Reflection probes** give bounce light and reflections: with a probe selected, **Alt+Left-click** snaps its box to geometry, **Alt+Right-click** snaps its 6 reflection planes to walls (**W** auto-picks all six). Probes blend via `blend_maxs`/`blend_mins`; a smaller, denser, or less-occluded probe wins over a larger overlapping one.

**Visionsets are a full runtime color-grading system.** `visionset_mgr::register_info` / `activate` / `deactivate` switch grading live — a GobbleGum wash, a black-and-white death, a zone's mood. Activation is driven from GSC, but the GSC `register_info` needs its matching CSC `register_visionset_info`, or `activate` does nothing visible. **Register during the first frame**: `visionset_mgr_shared.gsc:32` asserts *"All info registration in the visionset_mgr system must occur during the first frame while the system is initializing"* — so no `wait` before it — and it also asserts on a reused name or a priority already taken within that type. A `.vision` file (`vkTT` temperature, `vkTS` saturation, `vkTC` tint) lives under `share/raw/vision/` and zones via `rawfile,vision/<file>.vision`.

**Exploders** toggle a placed light/FX combo from script — set up in Radiant's Exploder Manager, then `exploder::exploder("name")` / `kill_exploder`.

## Fog: one asset, four sections, only two volumes

A `fog` GDT asset has **four independently-toggleable sections** — **world fog** (`worldfog`: `fogcolor`, `basedist`, `halfdist`, `baseheight`/`halfheight`, `fogopacity`), **lit fog** (`litfog`: volumetric, catches light, needs a light with the **volumetric** flag inside the volume), **sun fog** (`sunfog`: tint biased toward the sun) and **atmospheric fog** (`atmospherefog`: Rayleigh/Mie haze). Only the first two have a Radiant volume (`volume_worldfog`, `volume_litfog`); sun and atmospheric fog are extra sections of the same asset and apply wherever the referencing volume does — enable them by ticking the section in APE, not by placing anything — and check the volume's own flags too: `volume_worldfog` carries `ENABLE_SUN_FOG` (default on) and `DISABLE_FOG`, `volume_litfog` carries `ENABLE_SUN` (`t7.def.json`), so a volume can switch sun fog off even with the section ticked.

For scripted fog changes, `SetExpFog(startDist, halfwayDist, r, g, b, transitionTime)` takes all six arguments (older references showing five are wrong; stock `_art.gsc` calls it with six).

## FX for atmosphere: precache the effect, don't just place it

Building or editing the `.efx` itself is **t7kb:fx-editing**'s craft. Wiring one up: a common "it doesn't play" is a **missing precache** rather than a broken asset — `#precache("fx", …)` in GSC, `#precache("client_fx", …)` in CSC (stock `ctf.csc`). **The `_outdoor` FX techset (meant to cull weather indoors) is reported not to work in the released mod tools** — use outdoor occlusion volumes instead. The **blood-splatter** screen effect is off by default and needs a `blood.csc` override.

## Weather & skybox

Weather isn't one system, and none of it runs off a generic `level.weather_*` variable:

- **Snow**: stock attaches a player FX — `zm_usermap.csc`/`zm_giant.csc` precache `dlc0/factory/fx_snow_player_os_factory` as `level._effect["player_snow"]`. The CSC loop calling `PlayFX` on a timer (`falling_snow` on `on_localplayer_spawned`) is a community recipe (Ardivee's wiki, t7kb 0.70), not a stock script.
- **Rain** is four pieces: a player-attached FX tag (`PlayFxOnTag` + `SetFXOutdoor`, the latter community-reported, not in any stock script), a **Weathergrime Volume** (impact-splash decals), volume decals with a raindrop material (`t7_decal_raindrops`), and an outdoor occlusion volume to keep it off interiors.
- **Dynamic intensity** needs a clientfield you register yourself — community tutorials use a 2-bit `weather_intensity`; it is not a stock token.
- **Lightning sky-flash** is stock: a `vsky` KVP on WorldSpawn driven by `SetUkkoScriptIndex`, timed by the `lightning_strike` counter clientfield — read `zm_giant.gsc`/`.csc` for the working pattern.
- To switch the lighting state for a weather setup, call `level util::set_lighting_state(…)` from your own map script after `zm_usermap::main()` rather than forking `zm_usermap.gsc` as some tutorials do (**t7kb:scripting**).

**Rotating skyboxes are not a built-in live feature** — `skyRotation` is a static alignment. Community "rotating world" scripts rotate a sky brush/model themselves; treat any "just set X" claim about live sky rotation as unverified.

## Don't invent

CSV column names, KVPs, and dvars named here are shipped tokens — confirm them against files dated with the raw install (`share/raw/scripts`, `bin/t7.def.json`, `deffiles/*.awi`, Treyarch's `map_source/_prefabs/`), not against alias CSVs, sound templates, or Radiant templates a community pack replaced. If neither t7kb nor the raw install supports an alias column, FX behavior, or lighting field, don't assert it exists.
