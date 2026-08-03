---
name: bo3-anim-retarget
description: How to port an animation from another CoD generation onto a Black Ops 3 rig in Maya and get it playing in-game — two paths: full-body/biped via HumanIK retargeting, and first-person VIEWHANDS/viewmodel via `-mo` constraints (HumanIK cannot characterise an arms-only rig at all). Covers the cross-generation bind-axis mismatch, baking and export, the three mechanisms that move the first-person view, and the CoDMayaTools export bugs. Use when a ported anim binds but limbs are twisted or exploding, when first-person arms come out rotated ~90° or drift/stretch or fingers stay curled, when a first-person camera won't move, when a scripted scene errors `unable to find animation '<name>' in tree 'all_player'`, or when a CoDMayaTools export throws `notetracks[N]` / Py3 errors or the linker rejects an xcam. Distinct from bo3-animation (the export2bin/APE-xanim pipeline) and bo3-assets (model/material porting).
---

# Retargeting an animation onto the BO3 skeleton

Porting an animation from an **older CoD generation** (BO1/BO2/WAW, or any non-T7 rig) is not "import the anim onto `c_t7_ally_fb` and export." Treyarch reuses joint **names** across games, so the anim *binds* — every joint matches — but the **bind-pose orientations and local axes differ** between generations, so the rotation curves apply in the wrong frame and the result is subtly-to-badly **twisted** (a foot that folds at its middle, fingers splayed, an arm that pops to the wrong side). This skill is that retarget and its hard-won traps. For the export half (`.xanim_export` → `export2bin` → APE xanim asset) see **bo3-animation**; for the broader model/rig porting see **bo3-assets**. Confirm exact bone names against the raw install and the rig you actually ripped.

## Two paths — decide which one you're on before you start

| your source anim | target rig | method |
|---|---|---|
| **full body** (character, NPC, third-person player) | a biped BO3 rig (has `j_mainroot`, spine, legs) | **HumanIK retarget** — "Path A" below |
| **first-person arms only** (viewmodel / viewhands) | a BO3 `*_viewhands` rig | **direct `-mo` constraints** — "Path B" below. HumanIK is not an option: it cannot characterise an arms-only rig |

Both paths converge on the same **bake → export → APE** tail and share the same diagnostic method, unit gotchas, in-game traps and CoDMayaTools bugs — everything from "Bake, then unbind" onward applies to both.

## BO3 characters do NOT share one skeleton — pick one target rig and commit

*(Applies to both paths.)* The costly surprise: **different BO3 characters have completely different skeletons.** Bone counts seen on one project: a ported MWR body ≈ its own rig, the Der Eisendrache crew full-body (`c_zom_der_*_mpc_fb`) = **260** bones, the same crew's **viewbody** = **115**, the generic `c_t7_ally_fb` = **105**. Same `j_`-prefixed core names, but different extra bones *and* different bind poses. Re-characterizing on a new skin gives a visibly different joint selection — that's the tell.

The rule that ends the pain:

> **Retarget target rig == the model you render in-game == same skeleton.** Full stop.

You cannot retarget onto rig A and render the baked anim on model B if their skeletons differ (105-bone anim on a 260-bone rig, or mismatched bind poses → frozen or twisted). Consequences for planning:

- **Choose ONE rig for a group of NPCs and stick to it.** Retarget every NPC anim onto that same rig, render every NPC on that same model. Don't mix skins across skeletons.
- **The xanim asset's `model` field must be that same rig's `.xmodel_bin`** — the exact model you rigged onto, "not another one." A bare model *name* there fails to link (`GetFileAttributesEx … failed` / `xanim not found`); it must be a real bin **path** relative to `model_export\`.
- **Characterize that rig once, `Export Character Definition`, reuse it** for every anim in the group (seconds vs re-mapping ~20 bones each). *(Path A only — Path B has nothing to characterize.)*
- **Getting a loose `.xmodel_bin` into Maya as a target rig — unpack it, don't recompile-and-re-rip.** A `.xmodel_bin` is an **LZ4-compressed `.xmodel_export`** (magic `*LZ4*`, `uint32` decompressed size at +5, LZ4 **block** stream at +9), so mesh *and* skeleton are recoverable as text. Three converters can do it — `export2bin.exe` (Treyarch, in `bin\`, EXPORT→BIN only), `exportxbin.exe` (Scobalula, <https://github.com/Scobalula/exportxbin>, both directions), and **`exportx.exe`** ("ExportX", DTZxPorter, <https://dtzxporter.com/tools/exportx>, both directions). ExportX is the one that worked from the command line:
  ```
  exportx.exe -f <path>\model_LOD0.xmodel_bin -m export     # -m bin (default) goes the other way
  ```
  Verify the first lines — `NUMBONES` / `NUMVERTS` / `NUMFACES` — then import via CoDMayaTools. ExportX is a standalone exe; drop it in `bin\` next to the other two. If `exportxbin` reports `Failed to decompress binary file … return: 0`, that's the **tool**, not a bad rip — try its drag-and-drop mode (its documented primary usage) or switch to ExportX rather than re-ripping. Fall back to "compile into a map/mod and re-rip with Greyhound/Saluki as `.cast`" only when you actually want Cast (materials/images alongside the rig).

---

# Path A — full body, via HumanIK

## The shape of the job

```
old-CoD rig + its anim  ──HumanIK retarget──▶  BO3 rig (baked keys)  ──export──▶ .xanim  ──(Player anim? add to all_player.atr)──▶ in-game
```

The retarget is done in **Maya with HumanIK** (the built-in retargeter). Manual per-joint constraints are a dead end for cross-gen work (see below). Rip rigs and anims as **Cast** (SEanim/SEModel are deprecated); every modern ripper emits it.

**The order that works — do exactly this (source first, then target, then link):**

1. **Import the old-CoD (source) rig** alone — fresh Cast import is in bind pose.
2. **Characterize it** (map the joints, table below).
3. **Lock it** — *while still in bind.*
4. **Import the anim** onto the source rig (only now, after the Lock).
5. **Import the BO3 (target) rig** (also fresh = bind).
6. **Characterize it** (same mapping).
7. On the target, set **Source = the source character** → it retargets live; scrub to check.
8. **Bake** onto the target skeleton, then **export** (Select > Hierarchy).

The single invariant that makes or breaks it is **step 3 before step 4** — Lock each rig in bind *before* any animation touches it (see the trap section below). Everything else is just "build source fully, then target, then link."

## Why not manual constraints

The obvious "constrain each BO3 joint to the same-named old joint, then bake" **fails**, and knowing *why* saves hours:

- **`parentConstraint` on every joint explodes the rig.** It forces each child joint to the old rig's world **position**; since bone *lengths* differ between generations, the skeleton stretches/shatters within a few frames.
- **`orientConstraint` on all + `parentConstraint` on the root only** stops the explosion (positions now come from BO3's own bone lengths) but still **does not fix the local-axis difference** — the limbs stay mis-rotated, because copying world orientation joint-by-joint ignores that "zero rotation" means a different pose on each skeleton.

HumanIK exists precisely to solve that: it maps both skeletons to a common biped rig and resolves the axis/bind-pose difference automatically. Use it.

*(Path B has to solve this the hard way, with `-mo` — because HumanIK isn't available there at all.)*

## The one trap that wastes the most time: characterize in BIND POSE, first

HumanIK's **Lock/characterize snapshots the skeleton's current pose as its reference pose.** If the rig is already **posed by the animation** when you Lock it (e.g. the anim opens mid-action — sitting, crouched), HIK records that as "rest," and every retargeted frame is offset from a wrong reference → the same twisting you were trying to fix.

So the **order is mandatory**:

1. Import the rig **alone** (a freshly imported Cast rig is in bind pose).
2. **Characterize + Lock it NOW**, in bind.
3. *Then* import the anim onto it.

Once an anim is on the rig you **cannot reliably get back to bind** to re-characterize — the usual escapes all disappoint: `gotoBindPose` needs the skinned **mesh shape** (errors `No shape found` on a joint or group), and `doEnableNodeItems false animCurve` merely **freezes at the current frame** (which may be the sitting pose, not bind). If you locked in the wrong pose, the clean fix is to **start over from a fresh import in the right order** — faster than fighting it.

**Save each definition** (Character Controls → *Export Character Definition*, written to `…/HIKCharacterizationTool6/template/*.xml`). You reuse them for every other anim you port — *Import Character Definition* re-applies the mapping (still locked in bind) in seconds instead of re-clicking ~20 bones. They are plain XML, so you can diff two definitions to spot a bad mapping.

## Mapping CoD joints → HumanIK roles

Same for source and target (assign each role from the correct rig — the Cast plugin puts each imported rig under a **group** like `Joints` / `Joints1`, *not* a namespace, so select from the right group):

| HIK role | CoD joint |
|---|---|
| Reference | `tag_origin` |
| Hips | `j_mainroot` |
| Spine / Spine1 / Spine2 | `j_spinelower` / `j_spineupper` / `j_spine4` |
| Neck / Head | `j_neck` / `j_head` |
| Left/RightShoulder | `j_clavicle_le` / `_ri` |
| Left/RightArm | `j_shoulder_le` / `_ri` |
| Left/RightForeArm | `j_elbow_le` / `_ri` |
| Left/RightHand | `j_wrist_le` / `_ri` |
| Left/RightUpLeg | `j_hip_le` / `_ri` |
| Left/RightLeg | `j_knee_le` / `_ri` |
| Left/RightFoot | `j_ankle_le` / `_ri` |
| Left/RightToeBase | `j_ball_le` / `_ri` |

- **Extra BO3 joints left unmapped follow their parent** — `j_neck2`, face joints, twist/roll helpers: don't map them, they inherit.
- **Viewmodel/first-person rigs have no `j_head`** (you never see your own head) — map **Head → `tag_cambone`** (the camera bone, roughly where the head sits). This also feeds the head-role motion into the camera bone, which helps first-person camera transfer.
- Keep both definitions **symmetric** (same set of roles on each side) — an extra role mapped on one side only makes the retarget worse.

Set **Source = the old-CoD definition** on the BO3 (target) character to retarget live, then scrub.

## HumanIK doesn't retarget everything — fingers and camera need direct constraints

HIK only drives the **biped roles**. Fingers, and tags like `tag_camera`, are left in bind. Transfer them *after* the body retarget, with direct constraints (source → target, **no `-mo`**, so they snap exactly):

- **Finger joints → `orientConstraint`** (rotation only). Never `parentConstraint` a finger — same bone-length-stretch explosion as above.
- **`tag_camera` (and other tags) → `parentConstraint`.** A tag has no children/bone-length, so position+rotation is safe, and this gives you the *exact* cinematic camera trajectory (e.g. snapping a mis-placed BO3 camera onto the correctly-placed source camera). This is the go-to for "this one bone sits in the wrong place."

General rule you can reuse for any single misplaced bone: select the **source** bone, then the **target** bone, `orientConstraint` (skeleton bone) or `parentConstraint` (tag).

> **Wrist caveat (known cross-gen pain point):** the hand/wrist is a frequent offender even after a good HIK retarget — the wrist's local axis and roll convention differ between generations, so the hand can read rotated/rolled while the forearm is fine. If HIK leaves the wrist twisted, fix it like fingers: an `orientConstraint` from the source `j_wrist_*` onto the target `j_wrist_*` before baking, and if it's a pure roll, correct that one axis by hand. Confirm the actual bone axes on the rig you ripped rather than assuming — this is empirical, verify before relying on it.

---

# Path B — viewhands (first-person arms), via `-mo` constraints

A **viewhands** rig is arms-only — no `j_mainroot`, no spine, no legs — so **HumanIK cannot characterise it at all**: Lock only enables once the required biped bones are filled, and they never will be. Drive it with direct constraints instead.

> **Read the numbers in this section as one worked example, not as constants.** Everything below was measured on a single port: MW3 `berlin_sgt_down_recovery_vm` → `c_zom_der_dempsey_viewhands` (110 bones). The *techniques* transfer to any title pair; the specific angles, offsets and bone counts do not. Reproduce each **measurement** on your own rig pair rather than reusing the value.

**`-mo` on every constraint, and co-locate the rigs first.** The two rigs' bind axes differ by **40° (pinky) to 174° (right metacarpals), median 54°** — a plain `orientConstraint` copies the *absolute* source axis and twists every joint at rest, so `-mo` (offset captured in bind) is mandatory, not optional. And co-locate before constraining: MW3 is rooted at `tag_origin` with `tag_view` at Z=152.4 while the **BO3 root IS `tag_view`** at 0. Constrain across that gap and `-mo` bakes a ~150-unit offset into the constraint; every rotation of the source then swings the target through a huge arc and the arms fly off sideways. After co-locating, `tag_view`/`tag_ads`/`tag_cambone`/`tag_camera` match to 0.00.

**Pick the constraint type from the measured channels, not from habit.** Read the source anim's per-bone translation amplitude first, then:

| | use | because |
|---|---|---|
| root (`tag_view`), `tag_camera` | `parentConstraint -mo` | only where the residual offset is ~0, so there is no lever arm to swing |
| shoulders | `pointConstraint -mo` + `orientConstraint -mo` | MW3 drives the arms by **translating the shoulders** (140–164 units); a pointConstraint offset is not rotated by the source, so no lever arm |
| everything else | `orientConstraint -mo` | positions below the root must come from the target's own bone lengths |

Measure rather than assume: in this clip `tag_torso`'s translation amplitude was **0.00** — the `parentConstraint` it looked like it needed was pure noise.

**Finger names lie.** MW3 → BO3 is an index shift plus a rename, and the two rigs share enough short names that a by-name mapping *looks* correct while silently moving every phalanx one joint down the chain:

- `j_<finger>_<side>_0/1/2` → `j_<finger>_<side>_1/2/3` (index, mid, ring, pinky, thumb)
- `j_pinkypalm_*` → `j_pinkybase_*`, `j_ringpalm_*` → `j_ringbase_*` (the metacarpals — they *do* have counterparts)
- `j_webbing_*`, `j_sleave_reshape_*` → nothing on the BO3 side; drop them

Only ~36 of the 66 animated bones match by name. Rebuild the table from the two hierarchies (dump both hands and align them by chain depth) before trusting any of it on another title.

**Both rigs share short joint names, and the Cast anim importer will not say so loudly.** Import an anim with both rigs in the scene and every *shared* track is refused with `Unable to animate "<bone>" … name conflict in the scene`, while source-only names apply fine — leaving a half-animated rig that looks plausible. Prefix the target rig's joints (`bo3_*`) before importing, strip the prefix after baking. An xanim exported with `bo3_j_wrist_le` binds to no model. When renaming a hierarchy, sort **deepest-first and re-query each pass**: `listRelatives -ad` order is not guaranteed, and renaming a parent first invalidates every stored child path.

## The trap that actually causes "arms rotated 90° in game"

**MW3 keeps `tag_torso` permanently rotated** — a *constant* `(101.7, 8.2, 68.0)` local to `tag_ads`, identical on every frame — and authors the arms in that frame. BO3 **anchors the viewmodel on `tag_torso` and expects it at identity**. `orientConstraint -mo` copies the MW3 convention faithfully, offset included, and that constant ~100° is what renders as arms tilted off to the side while the camera looks fine.

Because the offset is *constant*, push it down into the arms with no change to the animation: after the bake, **zero `tag_torso`'s rotation, then restore the shoulders' world transforms** (sample them per frame first). Everything below the shoulders keeps its local keys and follows. Verify: torso local rotation → `(0,0,0)`, shoulder world positions **unchanged to the decimal**, shoulder local rotation moves into the same range as a working reference.

## Dead ends — measured, do not repeat

All of these looked reasonable and each made the in-game result worse:

| attempt | result |
|---|---|
| clear `tag_view` + `tag_ads` | no change, arms still offset |
| rebase per-frame onto `tag_camera` | view frozen; hands sit ~12 units below the view axis, permanently invisible |
| …and pin torso to camera | arms gone entirely |
| rebase by a constant `inv(camera @ frame 0)` | view detaches from the body |

The lesson: the world placement was never the problem, and neither was the camera. Fix the **constant torso rotation** and leave the tag chain otherwise alone.

## Arms longer than BO3, the wrist-twist chain, fingers left curled — three post-bake fixes

The base `-mo` retarget binds and plays, but three artifacts survive it. Each is a scripted post-pass — named here (`anchor_arms`, `prealign_fingers`, `distribute_wrist_twist`) so the three stay distinguishable:

- **MW3 arms are LONGER, so the hand lands wrong.** Pinning the shoulder 1:1 leaves the hand short by the length gap; pinning shoulder *and* wrist forces the span to MW3's length and **stretches the forearm → the skinned mesh deforms**. Fix (`anchor_arms`): point-constrain the shoulder to a **weighted blend of MW3's wrist and shoulder** — weight `r = BO3_arm / MW3_arm` on the shoulder, `1-r` on the wrist — which drops it onto the MW3 shoulder→wrist line at BO3-arm distance from the wrist, so the rigid BO3 arm lands its wrist on MW3's with nothing stretched. Keep the shoulder orient-constrained too; it rides off-screen.
- **MW3 has ONE wrist joint; BO3 viewhands have `j_wristtwist1..6`** that spread pronation from elbow to wrist so the sleeve doesn't pinch ("candy wrapper"). The retarget only drives `j_wrist`, leaving the chain at bind. Post-bake (`distribute_wrist_twist`), extract the wrist's **pure twist** (swing-twist quaternion decomposition — project the vector part onto the roll axis, so bend doesn't leak) and key each twist joint to **a fraction of it = its rest position along the forearm**, computed from geometry. The numbering is **not** elbow-to-wrist order, so an index-based `i/N` spreads it backwards — measure each joint's position, don't assume.
- **Fingers render curled when MW3's hand is flat.** They're orient-constrained but never prealigned, so `-mo` bakes BO3's *curled* rest pose as the baseline. Fix (`prealign_fingers`): before constraining, aim each proximal finger bone's **direction** at MW3's (bend only — leave the roll to `-mo`, or the axis-convention twist comes back).

---

# Both paths: bake, export, diagnose

## Bake, then unbind

Once the retarget looks right live:

1. **Bake** the target skeleton — select the target joint hierarchy and plot the retarget/constraints to keys:
   ```mel
   float $mn = `playbackOptions -q -min`; float $mx = `playbackOptions -q -max`;
   select -r `listRelatives -ad -type "joint" -f "Joints1"`;
   bakeResults -simulation true -t ($mn + ":" + $mx) -sampleBy 1 -disableImplicitControl true -preserveOutsideKeys false `ls -sl`;
   ```
   `-disableImplicitControl true` stops the HIK solver double-driving during the bake.
2. **Cut the drivers** so the baked keys play alone: Character Controls → **Source = None** (Path A), then `delete \`ls -type "orientConstraint" -type "parentConstraint" -type "pointConstraint"\`;`.
3. Scrub — the target should replay on its own keys, camera and fingers included (they're joints, so they baked too).

## Correcting a baked anim on an anim layer — the display gotcha that wastes time

To tweak an artifact non-destructively after bake (an arm/hand offset, a wrist), use an **additive anim layer**. The trap that will mislead you (and me): a freshly `Create Empty Layer` shows the **dense BASE keys** in the timeline/Graph Editor, so the empty layer looks like it "inherited every frame" — it hasn't.

**The timeline reflects a layer's own keys only once the joint is a MEMBER of that layer.** So: `Layers > Create Empty Layer` → select the joint → **`Add Selected Objects`** → *now* the timeline shows the layer's real (empty) keys, and your corrections land cleanly. Skipping "Add Selected Objects" is what makes the layer look polluted.

Then: make the layer **active** (highlighted), rotate the joint, `S`. A constant misalignment needs just **2 keys** (range start + end) — the additive offset holds across the dense base frames, no per-frame re-keying. (Additive preserves the base *motion* shifted by your offset; to fully replace a limb's motion over a range, use an **Override** layer instead.)

**Prerequisite:** HIK **Source = None** first (previous section). If the retarget is still live and Auto Key is on, every scrub bakes a key onto the active layer and it fills up *for real* — the tell is that deleting all keys leaves the anim still playing (HIK is still driving it).

## Export gotcha: select the HIERARCHY, not the root

CoDMayaTools exports **what's selected**. Clicking only the root joint *looks* like it grabbed the skeleton but exports **one bone** → a tiny file with `NUMPARTS 1`. Use **Select → Hierarchy** (or `select -r \`listRelatives -ad -type "joint" -f "Joints1"\`;`) before *Export XAnim*. **Verify** the export is large and `NUMPARTS` equals the joint count — a multi-MB text file, not a few hundred KB. Then convert per **bo3-animation** (Quality 0, notetracks cleared, single-arg `export2bin`/`exportxbin`, verify the `*LZ4*…55 c3` header).

## How to diagnose this class of bug

This is the method that finds an axis-convention bug on *any* title pair, and it's worth reaching for before guessing:

Load a **working anim of the same class** onto the same rig and compare **local rotations** — they encode the axis convention independently of pose and scale. Choose the reference carefully: a stock weapon idle is a *bad* one (it animates nothing but the arms), a **ported cinematic** is the right one. `t6_deathanim` (a BO2 death anim running as a BO3 viewmodel) reads `tag_torso (0,0,0)` on every frame, which is what exposed the 100° above.

`CoDMayaTools`' **`ImportXAnim` is a dead stub** — it parses a compiled xanim and `print()`s, building nothing. To get a reference anim into Maya, rip it as `.cast` with Greyhound while the map that contains it is loaded.

**Watch the units when comparing.** `.cast` is **centimetres**, `.xmodel_export`/`.xanim_export` are **inches** (game units) — CoDMayaTools scales by 2.54 on import and back on export. A distance read in Maya and the same distance read in the exported file differ by exactly 2.54; if a ratio of 2.54 shows up in a discrepancy, that is what it is, not a bug. Rotations are scale-free, which is another reason to compare those.

## Playing it in-game: things that bite

- **First-person (`int_`) vs third-person (`ch_`) variants.** A shipped IGC exports both: `int_*` is the **player** (first person — carries `tag_camera` + `tag_view`, the arms/body you see), `ch_*` is a **third-person body** with no camera (that's the *NPC beside you*, not the player). Retarget `int_` onto a BO3 **viewbody** (which has `tag_camera`); retarget `ch_` onto a full body. Set the xanim asset's **Model File** to the rig it was authored on (the viewbody for `int_`) so the `tag_camera` track survives.
- **A viewhands anim can play through a WEAPON — but that's the wrong tool for a camera-moving clip.** Give the player a weapon whose anim slots all point at your clip, then `SwitchToWeaponImmediate` — that is how the ported `t6_deathanim` runs a BO2 death animation in ZM. Clone a working `grenadeweapon` entry, swap the anim names, and stretch `raiseTime` to the clip length or the engine cuts to idle early; its other dependencies (`wpn_t7_none_view`/`_world`, `vm_ap9_ads_base_*`, `hud_us_grenade`) are all stock. The xanim asset itself is `type relative` + `useBones 0` for a viewmodel — `delta` + `useBones 1` is for world/character anims, and mixing them up is a classic cause of "right on the weapon, broken on the world model". **But for a get-up / mantle that *moves the view and travels*, the weapon path is a dead end** (a ripped weapon-viewhands model came in with broken partial skinning, and the weapon doesn't cleanly carry big camera travel) — link the player to an animated node instead (see the camera section below, GSC in **bo3-scripting**).
- **A scene bundle with a `Player` object plays through the player's animtree, not the raw xanim.** If the scene's object is `type Player` / `player 1`, the engine looks the anim up in **`all_player`** and you get `unable to find animation '<name>' in tree 'all_player'` at runtime — even though the xanim linked fine. **Fix:** add the anim's name to `share/raw/animtrees/all_player.atr` (a plain indented list) **and its generated copy** under `share/raw/animtrees/gen/animtrees/all_player.atr`. This is the same animtree override the zipline used for its `pb_zipline_*` player anims. Non-player scene objects (`Prop`) don't need this — they play the xanim directly.
- **An animated prop (ported IGC fxanim: rope, cloth, debris) uses `AnimScripted`, NOT `scene::play` or `SetAnim`.** Playing such a model's anim via a scene bundle's `MainAnim` — or via `SetAnim` on its animtree — leaves the mesh **frozen** (the model spawns, no bones move). What works is the cymbal-monkey verb: `model UseAnimTree(#animtree); model AnimScripted("note", origin, angles, %anim);` — it advances the scripted anim frame-by-frame on the model's own skeleton. The anim must still be listed in the animtree you `#using_animtree`.
- **Don't attach an IGC fxanim prop to its moving parent — play it at the shared scene origin.** A ripped fxanim (e.g. a rappel rope hanging off a heli) typically has **no root motion** (its `tag_origin`/PART 0 is static every frame) yet its *child bones* carry the full world-space sweep (verify: PART 1's `OFFSET` varies hugely across frames). Since the prop anim and the vehicle anim were authored on the **same IGC origin**, `AnimScripted`-ing the prop at that same origin makes it track the moving vehicle *for free*. `LinkTo`, the scene `AlignTargetTag`, and `scene::play` **on** the vehicle all fight this — each either froze the rope or killed the vehicle's own anim. Attach nothing; co-locate the origins.

## Moving the first-person CAMERA: three mechanisms, pick by case

Start from the finding that costs the most time, because it holds in every case: **a scene does NOT drive the player's first-person view in ZM.** The body anim plays (you can confirm it in third person), but the camera stays put. Two ground-truth reasons:

- In the shipped `scripts/shared/scene_shared.gsc`, the player view-lock calls in `reset_player` (`ShowViewModel`, `StartCameraTween`, …) are **commented out `//TODO_CODE: only supported for SP`**. The scene's first-person camera path is single-player only.
- The compiled viewbody model often **exposes no camera tag at runtime** — both `entity LinkTo(model, "tag_camera")` and `model GetTagOrigin("tag_camera")` fail/return undefined, even though the anim animates `tag_camera` and the `.XMODEL_EXPORT` lists it. So you cannot drive the camera off the model's tag either.

Verifying the anim is *not* the problem: parse the exported `.xcam_export`/`.xanim_export` and confirm `tag_camera`'s offset **varies across frames** (many distinct offsets, not one) — if it moves in the file, the camera data is fine and the wall is the engine/model, not your export.

What follows is **not** "always use an XCam" — it's that the view must be driven by something other than the scene. Three mechanisms do that; pick by what the clip has to do:

| the clip | mechanism |
|---|---|
| a cinematic on a **`Player`-object scene** | **XCam** — `PlayMainCamXCam` (CSC). The rest of this section. |
| a mostly-static **viewmodel** | play it through a **weapon**; the viewmodel's own `tag_camera` moves the view directly. Verified — but a dead end once the camera has to *travel* (see the weapon note above). |
| a **get-up / mantle that travels** | **link the player to an animated node** playing a *camera-less* scene bundle: `PlayerLinkToDelta` onto a mount `LinkTo`'d one `GetPlayerViewHeight()` **below** the node's moving `tag_camera`, so the eye lands on the animated camera. This is MW3's `_id_72AD` transposed, and the path that actually carried a traveling first-person get-up; the GSC lives in **bo3-scripting**. |

Counter-pressure worth knowing whichever you pick: every stock/ported reference keeps `tag_torso` / `tag_cambone` / `tag_camera` **static**, with all motion in the arm joints. An animated `tag_camera` works, but it is off the beaten path — so if the arms misbehave, suspect the tags before the arms.

### The XCam path

An **XCam** is a dedicated camera animation asset, played per-client with **`PlayMainCamXCam` (CSC)**. Because it's CSC/per-client, it also satisfies "every player sees the cinematic" in co-op (each client plays it on its own camera). `PlayMainCamXCam` is how the shipped campaign/MP cinematics drive their cameras.

#### Making the XCam in Maya

Work in the scene where the retargeted rig's `tag_camera` animates.

1. **Create a camera** (`Create → Cameras → Camera`).
2. **Snap it to `tag_camera`** (position + rotation): select `tag_camera` then the camera, `parentConstraint tag_camera camera1;` then delete that constraint (leaves the camera at the tag). Do **not** use `-mo` here (you want an exact snap, not the current offset).
3. **Fix the axis offset** — a Maya camera looks down its **−Z**, a CoD `tag_camera`'s forward is **+X**, so the raw-inherited orientation looks sideways/into the body. Apply a relative object-space rotation (`rotate -r -os -fo 90 0 -90 camera1;` is the usual starting point — verify by looking through the camera, adjust by 90° steps until forward is correct).
4. **Re-constrain WITH `-mo`** (`parentConstraint -mo tag_camera camera1;`) — now the corrected aim is locked and it follows the anim. (This is the one place `-mo` is right on Path A.)
5. **Bake** the camera, then **Call of Duty Tools → Export XCam** (frame range = full anim). The sub-camera name in the export is your Maya camera's name (e.g. `camera1`) — you need it to play the XCam.

#### The xcam asset + playing it

- **Asset** (`xcam.gdf` in your GDT, or APE): `filename` → the `.xcam_export`; **`use_firstperson_player` = 1** → this is what makes the **first-person body/arms render** during the XCam (the missing-hands fix lives here, not in the scene); `parent_scene` → the scene bundle that animates the body; `disableNearDof` = 1 to kill close-range blur. Zone it `xcam,<name>`.
- **Body vs camera are separate**: the XCam is only the camera (+ FP body visibility). The body animation still comes from playing the retargeted anim on a model (a `Prop`-type scene object is simplest — it plays the xanim directly, no `all_player.atr` needed).
- **Play** (CSC): `PlayMainCamXCam(localClientNum, "<xcam>", lerp, "<subcam>", "", origin, angles)` — `<subcam>` is the Maya camera name; `origin`/`angles` are the world placement (the scene's align point). `StopMainCamXCam(localClientNum)` ends it. Bridge from server logic with a **clientfield** (GSC sets it → a CSC callback calls `PlayMainCamXCam`). Note the CSC side **cannot read a server-side struct**, so pass the base origin/angles as constants that match the scene's spawn point.

#### Camera settings ↔ export values (FOV / DOF), and the conversion

The export's per-frame `fov`/`fdist`/`fstop` come from the Maya camera (CoDMayaTools `ExportXCam`), and the FOV conversion is non-obvious:

| Export field | CoDMayaTools formula | Maya attribute (`cameraShape`) |
|---|---|---|
| `fov` | `verticalFieldOfView(deg) × 1.5714` | Focal Length (+ film back) — **the VERTICAL FOV, not Maya's displayed horizontal "Angle of View"** |
| `fdist` | `focusDistance × CM_TO_INCH` (~0.3937) | Depth of Field → Focus Distance |
| `fstop` | `fStop` (direct) | Depth of Field → F Stop |

Consequences seen in practice: a default camera exports `fov ≈ 59.5` (vertical ~37.9° × 1.5714), which won't match the horizontal Angle of View Maya shows. And **over-strong DOF** is usually a tiny **Focus Distance** — e.g. a ~5 cm focus distance exports `fdist ≈ 1.97`, focusing ~2 units away and blurring everything past it; set Focus Distance high (≈ 2000 → `fdist ≈ 800`) so the scene is sharp, or raise F Stop. For fast iteration these values are **constant per-frame in the `.xcam_export`** and can be edited there directly instead of re-exporting.

> **FOV is driven by `flen`, NOT `fov` — the `fov` field is a decoy.** Each camera block in the `.xcam_export` has `"aperture": "FOCAL_LENGTH"`; with that mode the game **derives the runtime FOV from `flen` (focal length), and ignores the `fov` field entirely**. Editing `"fov"` (any value, 40→160) changes nothing in-game — verified — and neither does `cg_fov` (a played main-cam xcam ignores it too). To widen/narrow the cinematic FOV, edit **`flen`**: **lower `flen` = wider FOV** (e.g. `flen 10` is very wide; ~10–14 for a ~120°-ish feel; shipped CAC inspect cams sit near `flen 27` = tight). Also set **`aspectratio` to `1.7786`** (16:9) — CoDMayaTools may export `1.5`, which skews the framing. The clean source-side fix is the Maya camera's **Focal Length** attribute (it writes `flen`); editing `flen` (all per-frame occurrences + the camera-def) directly in the export is the fast iteration path.

## CoDMayaTools export bugs (patch the `.py`)

**`ValueError: No object matches name: XAnimExporterInfo.notetracks[1]`** kills an XAnim export outright, and it has nothing to do with your anim. `cmds.getAttr` **raises** on an element of a multi attribute that was never written, so the source's `cmds.getAttr(...) or ""` never gets the chance to default — any export slot that has never had a notetrack saved blows up. Wrap the read:

```python
def GetNoteList(attr):
    try:
        return cmds.getAttr(attr) or ""
    except Exception:
        return ""
```

and call it from the two export paths (`ExportXAnim`, `ExportXCam`). The ~11 other reads live in the notetrack manager windows, which create the attribute before reading it. Reload the script in Maya afterwards — the in-memory copy is still the broken one.

The rest are **XCam-export code still written for Python 2**; on modern Maya (Py3) each fails with a different traceback. All are one-line fixes in `CoDMayaTools.py`:

- `TypeError: a bytes-like object is required, not 'str'` → `…encode('ascii','ignore').replace('\\','/')` — in Py3 `.encode()` returns bytes; **drop the `.encode(...)`**, keep the `.replace`.
- `TypeError: 'float' object cannot be interpreted as an integer` → `range(0, numframes)` with a float → **`range(0, int(numframes))`**.
- Linker error at build time `JSON: Value is not an int64_t` on the xcam → the export wrote `"framerate": 30.0` / `"numframes": 1096.0` as **floats**; the linker wants ints → cast at the source (`"framerate": int(fps)`, `"numframes": int(fLength)`), or integer-ise those two keys in the `.xcam_export` after export.

## Don't invent

Bone names, HIK role names, APE fields, tool-version bugs, and the axis/roll quirks above churn by rig and by tool version — most specifics here (the wrist roll, the exact joint list, every measured number in Path B, the `all_player.atr` gen path, the CoDMayaTools Py3 line-fixes, the `× 1.5714` FOV factor and `CM_TO_INCH` focus scaling, the camera axis offset) are empirical findings from one port on one CoDMayaTools fork. Re-verify against the raw install, the specific rig you ripped, and your CoDMayaTools version before relying on them, and prefer reproducing the *check* (scrub the retarget, inspect the bone axis, confirm `NUMPARTS`, confirm `tag_camera`'s offset varies in the export) over trusting a remembered value.
