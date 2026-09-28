---
name: fx-editing
description: How to build, edit, and play Black Ops 3 particle FX (.efx, iwfx format) in Radiant's FX Editor — model vs sprite elements and their rotation (angleVelRoll vs the sprite rotGraph "Rotation" curve, inert unless it ramps 0→1), the Effect-category material FX needs (effect_lit_emissive_blend, effectMap not diffuseMap), a cloned stock material's junk (desaturationAmount, colorObjMin/Max, textureAtlasColumnCount), the two-sprites-flipped-180° both-sides trick, wagon-wheel aliasing at 60fps, playing FX via PlayFXOnTag, zoning it, and porting FX from an older CoD (Kobra not Greyhound, for BO1 .efx). Use when a model won't spin or spins jerky, an FX texture renders stretched/cut/discolored, a material throws "type mismatch"/"Can't find material"/"Unknown editor elem" at link, need a rotor/prop spinning with no script, or wiring an .efx onto a vehicle/tag. Distinct from t7kb:atmosphere (wiring/playing an existing effect for mood) — this is the .efx build/edit craft (elements, rotation, materials).
---

# Editing and playing BO3 FX (.efx)

BO3 effects are **`.efx` files in the `iwfx` text format** (header `iwfx 2` or `iwfx 3`), authored in **Radiant's FX Editor**. They are **not JSON** — that's a common confusion because *scriptbundles* (scenes, vehicles, killstreaks) **are** JSON. An `.efx` is a brace-block of un-quoted `key value;` lines plus curve blocks. Look up exact field names/APE material fields in **t7kb** (`t7kb:search` then `t7kb:get`) and against the raw install; this skill is the craft and the traps, most learned the hard way porting a BO1 huey rotor into BO3.

## Getting an FX out of an older CoD: Kobra, not Greyhound

**Greyhound dropped XEffect (FX) and GDT export** in recent builds — it emits models/anims/images/sounds only, **zero `.efx`**. **Kobra** (VenomModding's Greyhound fork) re-added XEffect + GDT, so **BO1/older FX come out of Kobra**, under `.../black_ops_1/fx/**.efx`. If a `find` for `*.efx` in your extraction is empty, you ripped with Greyhound — re-rip that title with Kobra. (Same split noted in **t7kb:assets**/**t7kb:animation**: Kobra for FX and GDTs.)

BO1 `.efx` are `iwfx 2` — **identical field schema to stock BO3 `iwfx 2`** (BO3 ships both — ~8,400 `iwfx 2` and ~1,500 `iwfx 3`), so a copied BO1 effect loads in BO3 **as-is**, no format conversion. Copy it into your own folder under `share/raw/fx/` (e.g. `share/raw/fx/_mymap/…`); the only work is importing the models/materials it references (below).

## Element types and — the big one — how each ROTATES

An `.efx` is a list of elements (`name "wraith_looping_defN"`). Two visual types matter here:

- **Model element** — `model { "xmodel_name" }`. Rotates via the **geometry angular velocity** `angleVelPitch/Yaw/Roll` (deg/sec). Set `angleVelRoll -8380` and the FX engine spins the model, **continuously, no script**. This is the clean way to spin a rotor/prop.
- **(Oriented/camera) sprite** — a billboard. **`angleVelRoll` does NOTHING on a sprite.** A sprite's spin is the **"Rotation" field in the FX Editor = the `rotGraph`** in the file. And the trap that eats hours:

> **A sprite `rotGraph` with a FLAT curve does not rotate — no matter how big the scalar.** `rotGraph 0.14 { {0 0.5}{1 0.5} … }` holds a fixed angle for the whole particle life. To actually spin, the **curve must ramp** (`{0 0.0}{1 1.0}`) so rotation progresses over the lifetime, and the **scalar is radians** — use an exact multiple of `2π` (`12.566371` = 4π = 2 revs per life) so the looping respawn is **seamless** (ends where it started). A non-multiple, or a short `Life`/`Looping` cycle on a *visibly asymmetric* sprite, reads as stutter.

So on a compound rotor FX you'll see BOTH: a spinning **model** disc (angleVelRoll) plus **sprite** overlays whose spin lives in the ramped rotGraph.

## Spawn/Life and the respawn-reset stutter

`Method Looping` + `Life 100` + `Interval 100` = the element dies and respawns every 100 ms. That's fine for a **static** element (nothing to reset), but a **spinning** one restarts its rotation each respawn → stutter, unless the per-life rotation is a whole number of turns (see above) **or** you make it persistent (`One Shot`, spawn count 1, huge `Life`). A pre-blurred disc that doesn't rotate never stutters — which is exactly why real rotors are shipped as a **static blurred disc**, not spinning sharp blades (see the math below).

## Texture atlas: wrong column count = stretched / cut image

A sprite/model material can be a **texture atlas** (multiple frames in one image). `textureAtlasColumnCount N` divides the texture into N columns; each element samples one frame via its `atlasIndex`. **If the count is wrong the image renders stretched and cut** — e.g. a single 512×256 texture at `columnCount 4` shows a 1/4-width sliver stretched to fill; at `1` it maps whole; a 512×256 that actually *is* two 256×256 frames needs `2`. Match the count to the real frame layout — **look at the PNG** (two discs side-by-side → 2 columns).

## FX materials: Effect category, and the junk a clone drags in

A model/sprite FX visual needs an **Effect** material, not a lit Geometry one:

- `materialCategory` **Effect**, `materialType` **`effect_lit_emissive_blend`** (or `…_nocull` to render both faces), `sort` **`effect - auto sort`**, `surfaceType` **`<none>`**.
- Its color image asset must have **`semantic` = `effectMap`** (a diffuse image's `diffuseMap` triggers a **"type mismatch" warning** on the Color Map in APE and can render wrong) and be **RGBA** (alpha for the blend).
- **Keep the compression `compressed high color`** (the stock fx-sprite format). Do **not** switch it to `uncompressed`: a sprite goes through the **compute-sprite path**, which builds a **combo texture** from all sprite textures and requires them to be a compatible/compressed type — an uncompressed one fails the link with `Some textures are the wrong type for creating fx combo texture used in the compute sprite and decal rendering`. If you genuinely need uncompressed, tick **"Don't use compute sprites"** on the element instead. (Blocky green/blue/red discoloration on a blur is almost always the **material color settings** — `colorObjMin/Max` tints, `desaturationAmount` — not the compression; fix those first, below.)

**If you author the material by cloning a stock fx material, it drags that material's settings** and they will bite:
- **`desaturationAmount 1`** (a `_desat` source) → your texture renders **grayscale, no color**. Set 0.
- **`colorObjMin/Max`** color tints (e.g. `1 1 0.5` yellow) → **tints your effect**. Set both to `1 1 1 1`.
- **`textureAtlasColumnCount`** carried from the donor (often 4) → the stretched/cut bug above.
Neutralize all three. (These aren't visible in the FX Editor's element panel — they're on the **material in APE/GDT**.)

## Both-sides visibility: flip one, don't reach for nocull first

To see a flat rotor/disc FX from **above and below**, the shipped trick is **two identical sprite elements, one rotated 180°** on an in-plane axis (`spawnAnglePitch 180`, *not* Roll — Roll just spins it in-plane). It is **not** done via the material culling or a different atlas frame. (nocull makes a *single* sprite two-faced but the shipped rotor uses the flip.)

## The wagon-wheel reality (why blur, not spinning blades)

You cannot show fast, sharp, *distinct* spinning blades at 60 fps without stroboscopic aliasing. A 2-blade rotor has 180° symmetry; per-frame rotation `A = angleVelDelta / fps`, and the disc **freezes/reverses** whenever `A ≈ 180°·k`. `10000°/s @ 60fps = 166.7°/frame ≈ 180°` → looks frozen. Pick `A` far from 180°·k (≈90–120°/frame, `≈5400–7200°/s`) if you must show blades — but the robust, ship-correct answer is a **pre-blurred disc texture** (the motion is painted in) with little or no rotation. BO1's vehicle def proves it: `rotorMainIdleFx` = sharp `fx_prop_huey_main_blade` (slow/idle), `rotorMainRunningFx` = the **blur** `fx_prop_huey_hub_blur` (running).

## Playing an FX from GSC (on a moving entity)

Vehicles play rotor FX automatically via the vehicle def (`rotorMain*Fx`), but a **script_model** heli/prop needs it played by hand:

```gsc
#precache("fx", "_mymap/fx_huey_main_blade_full");      // GSC; a CSC file precaches with "client_fx"
// init:
level._effect["heli_rotor_main"] = "_mymap/fx_huey_main_blade_full";
// play (fx rides the tag on the moving heli; looping fx loops until stopped):
main_rotor_fx = PlayFXOnTag(level._effect["heli_rotor_main"], heli, "main_rotor_jnt");
// cleanup: if (isdefined(main_rotor_fx)) { main_rotor_fx Delete(); }
```

Zone it with `fx,_mymap/fx_huey_main_blade_full` — that pulls the effect and its referenced models/materials. `PlayFXOnTag` attaches to the tag, so on a scripted/animated heli the rotor FX follows the body and crash for free.

## The Radiant FX Editor ↔ disk `.efx` sync trap

The FX Editor **loads its own copy and re-saves the whole `.efx`** (often upgrading `iwfx 2`→`3`). So: edits you make to the `.efx` **on disk are invisible to an open editor**, and **saving from the editor overwrites your disk edits**. Don't fight it — split the work: **element properties (rotation, spawn angle, life, atlas index) in the FX Editor**; **materials/images in APE/GDT** (picked up on re-link, not live in the FX viewport). If you must hand-edit the `.efx`, do it with the **editor closed**, and preserve its **CRLF** line endings.

## Don't invent

iwfx field names, APE material `materialType`/`semantic` values, and the FX-play API are shipped tokens — confirm against the raw install and a working stock `.efx`/material before asserting them. Where a value here is an empirical finding on one port (the rotGraph-ramp requirement, the 2π-multiple seamless loop, a specific artifact→setting link), reproduce the check rather than trusting the remembered number.
