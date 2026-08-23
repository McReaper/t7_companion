---
name: bo3-atmosphere
description: How to make a Black Ops 3 map sound and feel right — sound aliases, ambient rooms and reverb, sun/sky/SSI and reflection probes, visionsets for runtime color grading, the four fog sections (world/lit/sun/atmospheric) and their two placeable volumes, weather (rain/snow/lightning), and exploders. Use when an alias plays silently or not at all, ambient-room reverb doesn't switch between spaces, fog or the sky/sun looks wrong, a visionset color-grade won't apply, or weather FX (rain/snow) leaks indoors or doesn't play. Distinct from bo3-fx (owns building/editing the `.efx` particle effect itself — elements, rotation, materials): this is wiring and playing an existing effect for mood — precache, occlusion, ambience, fog, and light.
---

# Atmosphere: sound, lighting, fog & FX in Black Ops 3

This skill covers the audiovisual-polish layer of mapping — everything that makes a space *feel* right rather than just work. Look up exact CSV columns, KVPs, and error strings in **t7kb** (`t7kb:search` then `t7kb:get`); this skill is the craft and the gotchas around it.

## Sound: aliases are the unit, not WAV files

A **sound alias** is a named entry in a CSV under `<game>/share/raw/sound/aliases/` — code/script/triggers reference the alias name, never a WAV path directly, so the underlying file can change without touching callers. Add a new alias CSV as an `ALIAS` source in the map's `.szc`, with `Name` matching the CSV's base filename. Lean on a **template alias** (`share/raw/sound/templates/`) for the boilerplate columns (bus, volume, curves, limiting) and only override what you need per-alias — the column list is huge (~80 fields) and most modders only ever touch a handful (`FileSpec`, `VolMin`/`VolMax`, `Template`, `Looping`, `Subtitle`). **Every source WAV must be 48 kHz, signed 16-bit PCM** (export via Audacity) — the single most-repeated requirement across every audio how-to, and the most common reason an alias silently fails to play.

Sound variants aren't a naming convention you invent — they're gated through **sound contexts**: `ringoff_plr` selects an indoor/outdoor/underwater variant, `water` selects under/over-water, both resolved automatically from the listener's context (this is also what drives a weapon's indoor/outdoor decay tail, not a manual column).

**Gotcha:** `user_aliases.csv` is the example file the mod tools ship — it gets **overwritten on mod-tools updates**, so anything you add there is eventually lost. Create your own CSV named after your mod/map (same header row, same folder) and add it as its own `Sources` entry in the `.szc` instead.

**Aliases are rebuilt at the *link* step** — there's no separate sound-zone build. Edit the CSV, re-**link** the map, and the new/changed aliases are in the fastfile; a script-only relink picks them up too.

**`Storage` (loaded / streamed / primed) matters for timing.** A `streamed` one-shot can silently fail to fire when triggered at a precise instant (e.g. the first frame of a scripted sequence) — the stream isn't ready yet. For short one-shots that *must* play on cue, set `Storage` to **`loaded`** (held in memory, fires instantly); keep long loops/ambience `streamed`. If a callback provably runs (debug print fires) but you hear nothing, suspect the alias — WAV format (48 kHz/16-bit PCM) first, then `Storage`.

**Don't reinvent what's already loaded**: roughly 6,600 sound aliases and ~1,150 ZM FX / ~470 MP FX ship usable without declaring anything — run `t7kb:search` for an existing alias/FX before authoring a new one.

**Ambient rooms** (looping ambience + reverb per space) are defined in an ambients CSV (`share/raw/sound/ambients/`) and placed via a `trigger_multiple` in Radiant with `targetname: ambient_room`, `script_ambientroom: <name>`, and `CLIENTSIDE_TRIGGER` checked — size the trigger to match the room. `script_ambientpriority` breaks ties on overlapping triggers. Zombies' stock `_zm_audio.csc` already drives ambient-room switching (e.g. forcing a room during last stand); community setups (e.g. Ardivee's `_ambient_room.csc`) hook the same pattern for custom per-area ambience.

## Sun, sky & SSI: the primary light source

Before reflection probes or visionsets, a map needs its **Sun/Sky/Illumination (SSI)** setup: a `volume_sun` entity plus an `ssi` APE asset carrying sun direction, color, and up to 4 lighting "stops" (states) — the SSI **must be baked**, and `shadowSplitDistance` tunes shadow-cascade quality/performance. The skybox is a separate asset chain layered on top: sky image → material → model, then assigned into the SSI's SkyBox Model slot; `skyRotation`/`skyStops` control the sky's static alignment relative to the sun (this is alignment, not a live rotation — see Weather & skybox below for actually animating one).

## Lighting & reflections

**Reflection probes** give both bounce lighting and reflections. Placement shortcuts matter more than manual dragging: with a probe selected, **Alt+Left-click** a surface to snap the probe box to it ("snap to geo"); **Alt+Right-click** to snap **reflection planes** to the surrounding walls (there are only 6 planes per probe — press **W** to auto-pick all six). Probes blend between each other via `blend_maxs`/`blend_mins`; a smaller, denser, or less-occluded probe wins the blend over a larger overlapping one.

**Visionsets are a full runtime color-grading system, not just a screen-FX one-off.** `visionset_mgr::register_info` / `activate` / `deactivate` (both GSC and CSC) switch grading live — this is exactly how a GobbleGum's color wash, a black-and-white death effect, or a zone's mood shift are done in stock zombies. A `.vision` file carries tint keywords (`vkTT` temperature, `vkTS` saturation, `vkTC` tint), lives under `share/raw/vision/`, and zones via `rawfile,vision/<file>.vision`. Reach for this before assuming you need a custom shader for a color-grade change.

**Exploders** toggle a placed light/FX combo on/off from script — set it up in Radiant's Exploder Manager, then call `exploder::exploder("name")` / `kill_exploder` from GSC (the standard "power on lights this room" pattern).

## Fog: one asset, four sections, only two volumes

Fog is a single `fog` GDT asset with **four independently-toggleable sections**, each with its own field set — don't confuse "four fog systems" with "four things you place in Radiant," because only two of them get their own volume:

- **World fog** (`worldfog`) — plain distance fade (`fogcolor`, `basedist`, `halfdist`, `baseheight`/`halfheight`, `fogopacity`) — the everyday "atmosphere over the whole map" fog. Placed via a **`volume_worldfog`** entity.
- **Lit fog** (`litfog`) — volumetric fog that catches light: god-rays and glow around light sources, only rendering where volumetric lighting is on (needs a light with the **volumetric** flag inside the volume). Placed via a separate **`volume_litfog`** entity.
- **Sun fog** (`sunfog`) — a directional tint biased toward the active sun, layered on top of world fog.
- **Atmospheric fog** (`atmospherefog`) — a physically-based Rayleigh/Mie haze model for sky and long-distance air.

**Sun fog and atmospheric fog don't have their own volume** — they're just extra sections inside the same `fog` asset, and apply automatically wherever the `volume_worldfog`/`volume_litfog` that references that asset applies. So in Radiant you only ever place two *volumes*; enabling sun/atmospheric is done by ticking their section in the APE asset, not by placing anything extra. All four sections are independently toggleable, so a map can combine any of them.

For runtime/scripted fog changes bypassing the asset/volume entirely, `SetExpFog(startDist, halfwayDist, r, g, b, transitionTime)` is the direct GSC call (all 6 args required — older references showing five are wrong).

## FX for atmosphere: precache the effect, don't just place it

Wiring an already-built effect onto an entity/trigger follows the official FX Quickstart flow; building or editing the `.efx` itself (elements, rotation, materials) is **bo3-fx**'s craft, not this one — hand off there for anything about the particle asset itself. For **weather-style FX (rain, snow)** make sure the effect is **precached** — a common "it doesn't play" report traces back to a missing precache rather than a broken FX asset. **The `_outdoor` FX techset (meant to cull weather FX indoors) does not work in the released mod tools** — don't rely on it to keep rain/snow out of interiors; handle that with occlusion volumes instead (see Weather below). The **blood-splatter** screen effect is **off by default** and needs a `blood.csc` override to enable.

## Weather & skybox

Weather isn't one system — each effect has its own mechanism, and none of them are driven by a generic `level.weather_*` variable:
- **Snow** is a CSC-only loop (`falling_snow`, hooked on `on_localplayer_spawned`) calling `PlayFX` on a timer — no clientfield needed for a static snowfall.
- **Rain** is four cooperating pieces: a player-attached FX tag (`PlayFxOnTag` + `SetFXOutdoor`), a **Weathergrime Volume** (drives impact-splash decals), volume decals using a raindrop material (`t7_decal_raindrops`), and an outdoor occlusion volume to keep it off indoor surfaces.
- **Dynamic intensity** (heavier/lighter over time, for either) goes through a clientfield: `weather_intensity` (2-bit, 4 levels).
- **Lightning sky-flash** is its own mechanism: a `vsky` KVP on WorldSpawn (`zm_factory_lightning_ukko`) driven by `SetUkkoScriptIndex` calls, timed via its own `lightning_strike` clientfield/counter.

**Rotating skyboxes are not a built-in live feature** — `skyRotation` (above) sets a *static* alignment, not an animation. The community "rotating world" scripts are script-driven hacks that rotate the sky brush/model over time; treat any "just set X" claim about live sky rotation as unverified until you find the actual script doing it.

## Don't invent

CSV column names, KVPs, and dvars named here are shipped tokens — confirm exact names against the raw mod-tools install before stating them as fact. If neither t7kb nor the raw install supports a specific alias column, FX behavior, or lighting field, don't assert it exists.
