---
name: bo3-anim-retarget
description: How to port a full-body/first-person animation from an older CoD (BO1/BO2/WAW/MW) onto the Black Ops 3 skeleton in Maya, and get it playing in-game — the cross-generation skeleton mismatch (shared joint names but different bind-pose axes → twisted limbs), the HumanIK retarget workflow, the characterize-in-bind-pose-FIRST trap, transferring fingers/camera with direct constraints, baking, the in-game gotchas (`Player`-object scene needs the anim in `all_player.atr`; first-person `int_` vs third-person `ch_` variants), and driving a first-person cinematic CAMERA in Zombies with an XCam (`PlayMainCamXCam`) since a scene's view-lock is SP-only. Use when a ported/retargeted anim looks bound but limbs are mis-rotated/exploding, when porting a character/player anim across CoD generations, when a scripted-scene player animation errors `unable to find animation '<name>' in tree 'all_player'`, when a first-person cinematic camera won't move (body animates but view stays put), or when the CoDMayaTools XCam export throws Py3 errors or the linker rejects the xcam (`JSON: Value is not an int64_t`). Distinct from bo3-animation (the export/`export2bin`/APE-xanim pipeline) and bo3-assets (model/material porting) — this is the Maya retargeting + XCam craft and its traps.
---

# Retargeting an animation onto the BO3 skeleton

Porting an animation from an **older CoD generation** (BO1/BO2/WAW, or any non-T7 rig) is not "import the anim onto `c_t7_ally_fb` and export." Treyarch reuses joint **names** across games, so the anim *binds* — every joint matches — but the **bind-pose orientations and local axes differ** between generations, so the rotation curves apply in the wrong frame and the result is subtly-to-badly **twisted** (a foot that folds at its middle, fingers splayed, an arm that pops to the wrong side). This skill is that retarget and its hard-won traps. For the export half (`.xanim_export` → `export2bin` → APE xanim asset) see **bo3-animation**; for the broader model/rig porting see **bo3-assets**. Confirm exact bone names against the raw install and the rig you actually ripped.

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

## BO3 characters do NOT share one skeleton — pick one target rig and commit

The costly surprise: **different BO3 characters have completely different skeletons.** Bone counts seen on one project: a ported MWR body ≈ its own rig, the Der Eisendrache crew full-body (`c_zom_der_*_mpc_fb`) = **260** bones, the same crew's **viewbody** = **115**, the generic `c_t7_ally_fb` = **105**. Same `j_`-prefixed core names, but different extra bones *and* different bind poses. Re-characterizing on a new skin gives a visibly different joint selection — that's the tell.

The rule that ends the pain:

> **Retarget target rig == the model you render in-game == same skeleton.** Full stop.

You cannot retarget onto rig A and render the baked anim on model B if their skeletons differ (105-bone anim on a 260-bone rig, or mismatched bind poses → frozen or twisted). Consequences for planning:

- **Choose ONE rig for a group of NPCs and stick to it.** Retarget every NPC anim onto that same rig, render every NPC on that same model. Don't mix skins across skeletons.
- **The xanim asset's `model` field must be that same rig's `.xmodel_bin`** — the exact model you rigged onto, "not another one." A bare model *name* there fails to link (`GetFileAttributesEx … failed` / `xanim not found`); it must be a real bin **path** relative to `model_export\`.
- **Characterize that rig once, `Export Character Definition`, reuse it** for every anim in the group (seconds vs re-mapping ~20 bones each).
- **Getting a loose `.xmodel_bin` into Maya as a target rig:** you need a **`.cast`** (Porter deprecated XETool/SEModel/Wraith utils — Cast is the only importer now). There's no offline bin→cast converter: compile the bin into a BO3 map/mod and **re-rip it with Greyhound/Saluki** to produce the `.cast`.

## Why not manual constraints

The obvious "constrain each BO3 joint to the same-named old joint, then bake" **fails**, and knowing *why* saves hours:

- **`parentConstraint` on every joint explodes the rig.** It forces each child joint to the old rig's world **position**; since bone *lengths* differ between generations, the skeleton stretches/shatters within a few frames.
- **`orientConstraint` on all + `parentConstraint` on the root only** stops the explosion (positions now come from BO3's own bone lengths) but still **does not fix the local-axis difference** — the limbs stay mis-rotated, because copying world orientation joint-by-joint ignores that "zero rotation" means a different pose on each skeleton.

HumanIK exists precisely to solve that: it maps both skeletons to a common biped rig and resolves the axis/bind-pose difference automatically. Use it.

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

## Bake, then unbind

Once the retarget looks right live:

1. **Bake** the target skeleton — select the target joint hierarchy and plot the retarget/constraints to keys:
   ```mel
   float $mn = `playbackOptions -q -min`; float $mx = `playbackOptions -q -max`;
   select -r `listRelatives -ad -type "joint" -f "Joints1"`;
   bakeResults -simulation true -t ($mn + ":" + $mx) -sampleBy 1 -disableImplicitControl true -preserveOutsideKeys false `ls -sl`;
   ```
   `-disableImplicitControl true` stops the HIK solver double-driving during the bake.
2. **Cut the drivers** so the baked keys play alone: Character Controls → **Source = None**, then `delete \`ls -type "orientConstraint" -type "parentConstraint" -type "pointConstraint"\`;`.
3. Scrub — the target should replay on its own keys, camera and fingers included (they're joints, so they baked too).

## Correcting a baked anim on an anim layer — the display gotcha that wastes time

To tweak an artifact non-destructively after bake (an arm/hand offset, a wrist), use an **additive anim layer**. The trap that will mislead you (and me): a freshly `Create Empty Layer` shows the **dense BASE keys** in the timeline/Graph Editor, so the empty layer looks like it "inherited every frame" — it hasn't.

**The timeline reflects a layer's own keys only once the joint is a MEMBER of that layer.** So: `Layers > Create Empty Layer` → select the joint → **`Add Selected Objects`** → *now* the timeline shows the layer's real (empty) keys, and your corrections land cleanly. Skipping "Add Selected Objects" is what makes the layer look polluted.

Then: make the layer **active** (highlighted), rotate the joint, `S`. A constant misalignment needs just **2 keys** (range start + end) — the additive offset holds across the dense base frames, no per-frame re-keying. (Additive preserves the base *motion* shifted by your offset; to fully replace a limb's motion over a range, use an **Override** layer instead.)

**Prerequisite:** HIK **Source = None** first (previous section). If the retarget is still live and Auto Key is on, every scrub bakes a key onto the active layer and it fills up *for real* — the tell is that deleting all keys leaves the anim still playing (HIK is still driving it).

## Export gotcha: select the HIERARCHY, not the root

CoDMayaTools exports **what's selected**. Clicking only the root joint *looks* like it grabbed the skeleton but exports **one bone** → a tiny file with `NUMPARTS 1`. Use **Select → Hierarchy** (or `select -r \`listRelatives -ad -type "joint" -f "Joints1"\`;`) before *Export XAnim*. **Verify** the export is large and `NUMPARTS` equals the joint count — a multi-MB text file, not a few hundred KB. Then convert per **bo3-animation** (Quality 0, notetracks cleared, single-arg `export2bin`/`exportxbin`, verify the `*LZ4*…55 c3` header).

## Playing it in-game: two things that bite

- **First-person (`int_`) vs third-person (`ch_`) variants.** A shipped IGC exports both: `int_*` is the **player** (first person — carries `tag_camera` + `tag_view`, the arms/body you see), `ch_*` is a **third-person body** with no camera (that's the *NPC beside you*, not the player). Retarget `int_` onto a BO3 **viewbody** (which has `tag_camera`); retarget `ch_` onto a full body. Set the xanim asset's **Model File** to the rig it was authored on (the viewbody for `int_`) so the `tag_camera` track survives.
- **A scene bundle with a `Player` object plays through the player's animtree, not the raw xanim.** If the scene's object is `type Player` / `player 1`, the engine looks the anim up in **`all_player`** and you get `unable to find animation '<name>' in tree 'all_player'` at runtime — even though the xanim linked fine. **Fix:** add the anim's name to `share/raw/animtrees/all_player.atr` (a plain indented list) **and its generated copy** under `share/raw/animtrees/gen/animtrees/all_player.atr`. This is the same animtree override the zipline used for its `pb_zipline_*` player anims. Non-player scene objects (`Prop`) don't need this — they play the xanim directly.
- **An animated prop (ported IGC fxanim: rope, cloth, debris) uses `AnimScripted`, NOT `scene::play` or `SetAnim`.** Playing such a model's anim via a scene bundle's `MainAnim` — or via `SetAnim` on its animtree — leaves the mesh **frozen** (the model spawns, no bones move). What works is the cymbal-monkey verb: `model UseAnimTree(#animtree); model AnimScripted("note", origin, angles, %anim);` — it advances the scripted anim frame-by-frame on the model's own skeleton. The anim must still be listed in the animtree you `#using_animtree`.
- **Don't attach an IGC fxanim prop to its moving parent — play it at the shared scene origin.** A ripped fxanim (e.g. a rappel rope hanging off a heli) typically has **no root motion** (its `tag_origin`/PART 0 is static every frame) yet its *child bones* carry the full world-space sweep (verify: PART 1's `OFFSET` varies hugely across frames). Since the prop anim and the vehicle anim were authored on the **same IGC origin**, `AnimScripted`-ing the prop at that same origin makes it track the moving vehicle *for free*. `LinkTo`, the scene `AlignTargetTag`, and `scene::play` **on** the vehicle all fight this — each either froze the rope or killed the vehicle's own anim. Attach nothing; co-locate the origins.

## First-person cinematic CAMERA in Zombies: use an XCam, not the scene

This is the biggest trap and cost the most time, so lead with the conclusion: **a scene does NOT drive the player's first-person view in ZM.** The body anim plays (you can confirm it in third person), but the camera stays put. Two ground-truth reasons:

- In the shipped `scripts/shared/scene_shared.gsc`, the player view-lock calls in `reset_player` (`ShowViewModel`, `StartCameraTween`, …) are **commented out `//TODO_CODE: only supported for SP`**. The scene's first-person camera path is single-player only.
- The compiled viewbody model often **exposes no camera tag at runtime** — both `entity LinkTo(model, "tag_camera")` and `model GetTagOrigin("tag_camera")` fail/return undefined, even though the anim animates `tag_camera` and the `.XMODEL_EXPORT` lists it. So you cannot drive the camera off the model's tag either.

Verifying the anim is *not* the problem: parse the exported `.xcam_export`/`.xanim_export` and confirm `tag_camera`'s offset **varies across frames** (many distinct offsets, not one) — if it moves in the file, the camera data is fine and the wall is the engine/model, not your export.

**The correct mechanism is an XCam** — a dedicated camera animation asset, played per-client with **`PlayMainCamXCam` (CSC)**. Because it's CSC/per-client, it also satisfies "every player sees the cinematic" in co-op (each client plays it on its own camera). `PlayMainCamXCam` is how the shipped campaign/MP cinematics drive their cameras.

### Making the XCam in Maya

Work in the scene where the retargeted rig's `tag_camera` animates.

1. **Create a camera** (`Create → Cameras → Camera`).
2. **Snap it to `tag_camera`** (position + rotation): select `tag_camera` then the camera, `parentConstraint tag_camera camera1;` then delete that constraint (leaves the camera at the tag). Do **not** use `-mo` here (you want an exact snap, not the current offset).
3. **Fix the axis offset** — a Maya camera looks down its **−Z**, a CoD `tag_camera`'s forward is **+X**, so the raw-inherited orientation looks sideways/into the body. Apply a relative object-space rotation (`rotate -r -os -fo 90 0 -90 camera1;` is the usual starting point — verify by looking through the camera, adjust by 90° steps until forward is correct).
4. **Re-constrain WITH `-mo`** (`parentConstraint -mo tag_camera camera1;`) — now the corrected aim is locked and it follows the anim. (This is the one place `-mo` is right.)
5. **Bake** the camera, then **Call of Duty Tools → Export XCam** (frame range = full anim). The sub-camera name in the export is your Maya camera's name (e.g. `camera1`) — you need it to play the XCam.

### CoDMayaTools Py3 XCam-export bugs (patch the `.py`)

The community CoDMayaTools forks have **XCam-export code still written for Python 2**; on modern Maya (Py3) each fails with a different traceback. All are one-line fixes in `CoDMayaTools.py`:

- `TypeError: a bytes-like object is required, not 'str'` → `…encode('ascii','ignore').replace('\\','/')` — in Py3 `.encode()` returns bytes; **drop the `.encode(...)`**, keep the `.replace`.
- `TypeError: 'float' object cannot be interpreted as an integer` → `range(0, numframes)` with a float → **`range(0, int(numframes))`**.
- Linker error at build time `JSON: Value is not an int64_t` on the xcam → the export wrote `"framerate": 30.0` / `"numframes": 1096.0` as **floats**; the linker wants ints → cast at the source (`"framerate": int(fps)`, `"numframes": int(fLength)`), or integer-ise those two keys in the `.xcam_export` after export.

### The xcam asset + playing it

- **Asset** (`xcam.gdf` in your GDT, or APE): `filename` → the `.xcam_export`; **`use_firstperson_player` = 1** → this is what makes the **first-person body/arms render** during the XCam (the missing-hands fix lives here, not in the scene); `parent_scene` → the scene bundle that animates the body; `disableNearDof` = 1 to kill close-range blur. Zone it `xcam,<name>`.
- **Body vs camera are separate**: the XCam is only the camera (+ FP body visibility). The body animation still comes from playing the retargeted anim on a model (a `Prop`-type scene object is simplest — it plays the xanim directly, no `all_player.atr` needed).
- **Play** (CSC): `PlayMainCamXCam(localClientNum, "<xcam>", lerp, "<subcam>", "", origin, angles)` — `<subcam>` is the Maya camera name; `origin`/`angles` are the world placement (the scene's align point). `StopMainCamXCam(localClientNum)` ends it. Bridge from server logic with a **clientfield** (GSC sets it → a CSC callback calls `PlayMainCamXCam`). Note the CSC side **cannot read a server-side struct**, so pass the base origin/angles as constants that match the scene's spawn point.

### Camera settings ↔ export values (FOV / DOF), and the conversion

The export's per-frame `fov`/`fdist`/`fstop` come from the Maya camera (CoDMayaTools `ExportXCam`), and the FOV conversion is non-obvious:

| Export field | CoDMayaTools formula | Maya attribute (`cameraShape`) |
|---|---|---|
| `fov` | `verticalFieldOfView(deg) × 1.5714` | Focal Length (+ film back) — **the VERTICAL FOV, not Maya's displayed horizontal "Angle of View"** |
| `fdist` | `focusDistance × CM_TO_INCH` (~0.3937) | Depth of Field → Focus Distance |
| `fstop` | `fStop` (direct) | Depth of Field → F Stop |

Consequences seen in practice: a default camera exports `fov ≈ 59.5` (vertical ~37.9° × 1.5714), which won't match the horizontal Angle of View Maya shows. And **over-strong DOF** is usually a tiny **Focus Distance** — e.g. a ~5 cm focus distance exports `fdist ≈ 1.97`, focusing ~2 units away and blurring everything past it; set Focus Distance high (≈ 2000 → `fdist ≈ 800`) so the scene is sharp, or raise F Stop. For fast iteration these values are **constant per-frame in the `.xcam_export`** and can be edited there directly instead of re-exporting.

> **FOV is driven by `flen`, NOT `fov` — the `fov` field is a decoy.** Each camera block in the `.xcam_export` has `"aperture": "FOCAL_LENGTH"`; with that mode the game **derives the runtime FOV from `flen` (focal length), and ignores the `fov` field entirely**. Editing `"fov"` (any value, 40→160) changes nothing in-game — verified — and neither does `cg_fov` (a played main-cam xcam ignores it too). To widen/narrow the cinematic FOV, edit **`flen`**: **lower `flen` = wider FOV** (e.g. `flen 10` is very wide; ~10–14 for a ~120°-ish feel; shipped CAC inspect cams sit near `flen 27` = tight). Also set **`aspectratio` to `1.7786`** (16:9) — CoDMayaTools may export `1.5`, which skews the framing. The clean source-side fix is the Maya camera's **Focal Length** attribute (it writes `flen`); editing `flen` (all per-frame occurrences + the camera-def) directly in the export is the fast iteration path.

## Don't invent

Bone names, HIK role names, APE fields, tool-version bugs, and the axis/roll quirks above churn by rig and by tool version — most specifics here (the wrist roll, the exact joint list, the `all_player.atr` gen path, the CoDMayaTools Py3 line-fixes, the `× 1.5714` FOV factor and `CM_TO_INCH` focus scaling, the camera axis offset) are empirical findings from one port on one CoDMayaTools fork. Re-verify against the raw install, the specific rig you ripped, and your CoDMayaTools version before relying on them, and prefer reproducing the *check* (scrub the retarget, inspect the bone axis, confirm `NUMPARTS`, confirm `tag_camera`'s offset varies in the export) over trusting a remembered value.
