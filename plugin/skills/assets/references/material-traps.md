# Material traps: emissive, double-sided, and packed alpha

Referenced from `plugin/skills/assets/SKILL.md` — read that file first. Three material problems that look like something else, each verified porting a BO2 vehicle.

## Emissive lives in `colorMap00`, and the slot names are traps

A glowing part (screens, eyes, indicator lights) needs a **`lit_emissive*`** material type — there are ~59 of them, so pick the one that also carries whatever else the material needs (`lit_emissive_plus` when you also want gloss, `lit_emissive_advanced_fullspec`, `lit_emissive_transparent`, …).

The emissive map goes in **`colorMap00`**. Nothing in that name says "emissive", and it does **not** turn up if you look for slots by grepping field names ending in *map* — which is how you end up concluding, wrongly, that BO3 has no emissive slot and that the emission has to be composited into the diffuse alpha. **870 of 1362** shipped emissive materials fill it: `mtl_char_ger_zombie_eyes` sets `colorMap00 = zombie_eye`, `mtl_p7_pro_monitor_control_tower` sets it to an `_e` texture. So an older title's separate `_e` map ports **straight across**, no channel packing.

Its image asset is `sRGB3chAlpha` / `diffuseMap` like a diffuse, but **`compressionMethod` = `compressed no alpha`**.

Two neighbours named no better: **gloss is `cosinePowerMap`** (image side `Linear1ch` / `glossMap`; only the `_plus` / `_advanced` categories expose it) and **AO is `occMap`**.

`emissiveFalloff` and `emissiveIncompetence` (APE labels the latter *gameplay intensity*) tune the result — and do **not** copy them from a donor. Across the shipped emissive materials `emissiveIncompetence` splits **681 / 680** between `0` and `1`: a genuine per-material choice, not a default with outliers. Same discipline as `SurfaceType` above — check the *distribution* before copying a value, because where it is 50/50 the donor tells you nothing.

## Double-sided: `nocull` is a material type, not a flag

A model whose backfaces don't render is fixed by a **`nocull` material type** — `lit_nocull`, `lit_alphatest_nocull`, `lit_transparent_nocull`, `lit_detail_nocull` and `_advanced`/`_plus` variants all ship. `doubleSidedLighting` is *not* it: that controls how backfaces are **lit**, not whether they're drawn, and setting it changes nothing visible.

Worth checking the geometry first so you know which problem you have: if every face of the material has a unique position triple (no duplicated triangles with reversed winding), the mesh is genuinely single-sided and only `nocull` can save it. Duplicating the faces in the export also works but doubles the triangles and is not reversible from the GDT.

## An alpha channel's *percentage* tells you nothing — its distribution does

Ripped diffuse maps frequently carry a packed gloss/spec mask in alpha, and a real alpha cutout looks identical if you only measure "what fraction of pixels are non-opaque". Alpha-testing a packed mask punches **black speckles** through the surface, which reads as a corrupt texture.

Map the alpha spatially instead — a coarse grid of "percent of pixels below threshold" per cell. A genuine cutout is a **compact, sharp-edged region**; a packed mask is scattered noise across the whole sheet. One bus material showed a solid rectangular transparent block over half the texture (real vents), another only 2-3% scattered (gloss mask), and the naming confirmed both — Treyarch shipped a `_opq` twin of the vented material.
