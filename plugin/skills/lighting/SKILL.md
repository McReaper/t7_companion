---
name: lighting
description: How to light a Black Ops 3 map in Radiant — the map's own SSI and its exposure range (evmin/evmax), light stops set against that exposure, spot and omni shaping (falloffdistance, far_edge, fov_outer, superellipse, cut_on), PROBE_ONLY fills, reflection probes as boxes snapped to each space (size_min/size_max, blends, override, subtract), the global probe and the volume_sun's target, shadowed-light budgets, Radiant's bake and the LED. Use when lighting looks flat, washed out, blotchy or too dark, probing looks dirty or bleeds between rooms, a ceiling shows hot disks, exposure pumps from room to room, Radiant shows BOUNDING SUN VOLUME DOES NOT SUPPORT ACTIVE LIGHTING STATE or N of N probes need raytracing, or a bake logs Probe at … out of range or Too many shadowing omni lights. Distinct from t7kb:atmosphere (fog, sound, visionsets, weather, exploders) and t7kb:mapping (the geometry itself, sealing, caulk) — this is the light on it, sun through a wall's foot or corner included.
---

# Lighting a Black Ops 3 map

**Lighting is exposure first, lights second.** A light's `stops` means nothing until the map's exposure is fixed: the same light reads blown out under one SSI and black under another, and stock SSIs lock theirs anywhere from 0.5 to 13. The bounce is the other half: a surface takes its indirect light and reflections from the reflection probe covering it, baked (official, `Reflection_Probes.pdf`), and a surface no local probe covers falls back to the global probe, which by default floats at the sun volume's centre and sees mostly sky. Look up exact KVPs, defaults and error strings in **t7kb** (`t7kb:search` then `t7kb:get`); Treyarch's own guides are the `docs_modtools` PDFs (`source` `docs`) and Verk's "Lighting Tutorial" parts 1–5 (`source` `video`). Geometry, sealing and sun leaks through walls are **t7kb:mapping**; fog, visionsets and exploders are **t7kb:atmosphere**; the build's light stage is **t7kb:compiling**.

## Lock the exposure in the map's own SSI before placing a light

The template's `volume_sun` uses `default_day`, whose exposure runs from `evmin` 1 to `evmax` 16 (`source_data/ssi.gdt`): auto exposure then re-exposes each room on its own, and no stops value means the same from one room to the next. Copy a stock SSI into the map's own GDT and narrow the range: `t7kb:gdt_edit` with `copy_from` (`default_day`, `mp_sector`…) into `source_data/<map>.gdt`, then `evmin`/`evmax` about a stop apart (`deffiles/ssi.awi` holds `evmin`, `evmax`, `evcmp` and `bounceCount`). Treyarch's rule for MP: exposure always between 8 and 12, an interior about a stop darker than the exterior, light everything relative to that exposure (official, `Multiplayer_Lighting_Workflow.pdf`).

| Stock SSI | evmin–evmax | sun stops | mood |
|---|---|---|---|
| `default_day` | 1–16 | 14 | the template's: unlocked |
| `mp_sector` | 11.5–13 | 13.3 | daylight |
| `default_sunset` | 8–12.5 | 11 | dusk |
| `zm_zod` | 4–6 | −2.5 | night, sun off |
| `zm_factory` | 0.5–3.5 | −2 | night (zm_giant narrows it to 3–3.5 with `evauto`, `evmin`, `evmax` on the worldspawn) |

Then set the `volume_sun`'s `ssi1` (and `ssi`) to it. **A new SSI is unknown to Radiant until `gdtdb /update`**: the 3D view prints `BOUNDING SUN VOLUME DOES NOT SUPPORT ACTIVE LIGHTING STATE` and keeps the old exposure (verified); the build's first pass runs gdtdb, or run `gdtdb.exe /update` from the install's `gdtdb` folder. `bounceCount` 4 to 6: never under 4, and more costs bake time for little (Verk, t7kb 0.30). Two "I changed it and nothing happened" traps (Treyarch's SunSky doc, per the corpus):

- **`ssiN_runtime_override` wins in game, the baked `ssiN` wins in the probes**, unless the `volume_sun`'s `override_reflections` is on (`t7.def.json`): edit the one that's showing.
- **`pitch`, `yaw`, `skyStops` and the bounce part of `stops` change only after a rebake**; a relink alone shows the old sun. `skyRotation` and `skyStops` are fields of the sky *material*, not the SSI (`deffiles/material.awi`, `ssi.awi`).

**The 3D view's `Exposure EV x (y GPU)` line is the meter.** Stuck on `evmax`, the scene is too bright for the range; stuck on `evmin`, too dark; resting inside it, the lights fit (verified).

## Set a light's stops for where its light lands

`stops` is a light's level out to `falloffdistance`; past it the light falls with the square of the distance, so the level at distance *d* is about `stops − 2·log2(d / falloffdistance)` (the key's own description, `bin/t7.def.json`; the formula follows from it). A larger `falloffdistance` spreads a light evenly (a well-lit community map's room fills use 30 to 65); `far_edge` softens where it ends at `radius`; `cut_on` starts it further out, past a lamp shade (Verk, t7kb 0.30). Defaults: `stops` 11, `radius` 100, `falloffdistance` 12, `far_edge` 0.95, and **`PRIMARY_NOSHADOWMAP` true: a placed light casts no shadow until it is set to 0** (`t7.def.json`), and lights the room behind a wall too (community, t7kb 0.25).

- **Ceiling lights point down.** An omni a few units under the ceiling burns a disk on it (verified). A `PRIMARY_SPOT` under a fixture model lights the floor and leaves the ceiling to the bounce. **A spot points straight down at `angles` `0 0 0`**, and pitch tilts it from there: Treyarch's ceiling spots carry `360 0 0` (mp_combine) or no `angles` at all (most of zm_giant's), and `90 0 0` lays the cone on its side, streaking the walls and leaving the floor dim (verified in the install and on a real map). Then: `fov_outer` up to 120 (wider draws artifacts: official, `Lighting_Parameters.pdf`), `roundness` 1, and an inner `superellipse` value well under the outer one feathers the cone's edge (`0.2 1 0.2 1`; default `0.75 1.0 0.75 1.0`).
- **Give the light a source.** `p7_light_fixture_led_ceiling_01` (a flat LED panel, 120 stock placements) or `p7_light_metal_recessed_led` (a round downlight); the fixture is the mapper's choice.
- **Fill with `PROBE_ONLY` lights, not more visible ones.** Only the probes see them: they lift the bounce on walls and ceiling with no hot spot anywhere. Treyarch's zm_giant sources hold 155 among their 577 lights, big and low (verified in the install).

A recipe that baked well in a 112-high office at `evmin` 11.5 / `evmax` 12.5 (verified in Radiant's bake): a spot under each panel every ~320 units, `stops` 16, `falloffdistance` 24, `radius` 260, shadows on; a `PROBE_ONLY` omni at each, 24 units under the ceiling, `stops` 9, `radius` 320, `falloffdistance` 64. The exposure then rests at 11.7. Change the SSI and every number moves with it.

## Probe every space with a box snapped to its walls, floor and ceiling

A `reflection_probe` is a box: `size_min` and `size_max` its reach each way from its origin, 72 by default, no radius (`t7.def.json`). It fades out between that inner box and an outer one `blend_mins`/`blend_maxs` further out, default 3 (official, `Reflection_Probes.pdf`), so a box ending on a wall's face covers the face fully. In Radiant, Alt+click snaps the selected probe's side to the face clicked.

**Probe the play space with `t7kb:map_edit`'s `probes` op first, then by hand what it leaves.** It walks the floor players reach, keeps to the zones on a map that has them, and boxes it by the rules below, prefabs included; `replace` swaps the probes already there, `global` moves the global probe (checked against hand-placed probes on two maps). Along a wall off the axes it turns a box with the wall (`angles`, as Treyarch's along zm_giant's slanted walls), since an axis-aligned box's edge would cross the wall's face. Its notes count the floor no box holds, and it leaves two things to a lighter: what players see but don't stand in (vistas, rooms behind windows: zm_test's lighter gave those big `8x` boxes), and floor reached only by jumping. For those, measure the faces (`t7kb:map_find` bounds, `t7kb:map_view` `top` with its `scale`) and write the sizes.

- **Inner boxes never overlap** (Verk, t7kb 0.30): where two must, mark the one that should win `override` (official, `Priority_Probes.pdf`) or cut the other with a `subtract` child (`Subtract_Probes.pdf`). A blend past an inner wall reaches the next room's face: keep it at the wall's thickness or less there, and longer only over open floor.
- **Cut odd shapes into several boxes**, rotate one with `angles` to fit a wall off the grid, or untick `box` for a convex multiface probe of up to 24 planes (official, `Multiface_Probes.pdf`).
- **The capture point must see the room.** GI is computed on a grid inside the box (`resolution` 4x, AO `ao_range` 8, `ao_power` and `ao_strength` 1: Treyarch's defaults, `Multiplayer_Lighting_Workflow.pdf`) from rays that must reach the centre of one of the 8 nearest probes (official). Keep the origin out of stairs and counters, and **put no probe in a space nobody reaches**: in a sealed closet the bake prints `ERROR: Probe at x y z out of range. Color … exposure …`, gone once that probe is deleted (verified).
- Leave `evcomp` at 0 (official: its artifacts aren't wanted). Treyarch starts interior probes at a probe brightness of 2 (official, `Multiplayer_Lighting_Workflow.pdf`); Verk warns that raising `brightnessAdjust` splotches the bounce and lowers it only for a space meant to stay dark: reach for lights and `PROBE_ONLY` fills first.

A space no box covers takes the global probe, which looks worse up close (official, `Lighting_Probe_Workflow.pdf`): cover every surface a player sees.

## Move the global probe off the sky

The global probe sits at the `volume_sun`'s centre, high over a whole map, unless the `volume_sun` `target`s an entity: give an `info_null` a `targetname` and set the volume's `target` to it, at an average spot inside the play space, neither buried nor high up (official, `Lighting_Probe_Workflow.pdf`). It blends in only outside every local probe's reach (`Reflection_Probes.pdf`); `global_fill_color` tints what of it is occluded.

## Keep the sun out with a sharp sun shadow, walls 8 thick or more, probes and a high bake

Sunlight along a wall's foot or through a corner is the lighting's setup, not a gap to overlap brushes over. Lower the `volume_sun`'s `shadowSplitDistance` (the sun's shadow map: smaller is sharper): the ZM Mod Level template ships 2000, zm_giant's sun has 900 and the ZM Advanced Level template 750 (verified in the install); community advice runs 700–1000 by the map's size (t7kb discord 0.25). Keep walls 8 thick at least, thicker where the sun still bleeds; give lights their shadows; probe every space (above); and build the final light `high` (`t7kb:build`'s `light`). A space open to the sky but meant to stay in shade still takes brushes all of `caulk_sun_shadow` over it (zm_giant's light prefab holds 12, 8-unit sheets over roofs among them, verified in the install).

## Count shadowed lights against the worldspawn's slices

The worldspawn's `numSpotShadowSlices` (64) and `numOmniShadowSlices` (24) bound shadowing lights; past them Radiant prints `Too many shadowing spot lights. Shadow for light at x y z is being dropped` and that light then shines through walls (the string is in `radiant_modtools.exe`; zm_giant raises the omni count to 96). The budget isn't a plain count over the map: a map with 70 shadowed spots baked without the warning (verified). `t7kb:map_edit` can't set worldspawn keys (`set` refuses worldspawn): ask the user to change one in Entity Info.

## See it baked before building it

`t7kb:map_view` with `bake` runs Radiant's own bake and shoots under real lighting; its answer's `warned` lists what the bake's console warned of (out-of-range probes, dropped shadows), folded by kind. Radiant's view lights only Radiant: the game reads the `.led` the build's light stage writes (**t7kb:compiling**), and Radiant asks to export one on close, an answer that is the user's. Bake, read the exposure line and `warned`, look from inside each space, then build.

## Don't invent

KVPs, defaults and strings named here are confirmed in the install (`bin/t7.def.json`, `deffiles/ssi.awi`, `source_data/ssi.gdt`, Treyarch's `map_source` prefabs, the Radiant binary) or carry their source. Stops that look right depend on the SSI: give numbers with the exposure they were judged under, and bake before calling a look good.
