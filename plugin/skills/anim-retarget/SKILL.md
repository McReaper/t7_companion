---
name: bo3-anim-retarget
description: How to port an animation from an older CoD generation onto a Black Ops 3 rig in Maya and into the game — HumanIK retargeting for full-body, direct `-mo` constraints for first-person VIEWHANDS/viewmodel (HumanIK cannot characterise an arms-only rig), cross-gen bind-axis mismatch, and a locked rig reading the solver not the skeleton. Use when a ported anim binds but limbs twist/explode, one side is off, the other fine, a joint edit "sticks" (nothing moves, or re-locking restores the bug), the Cast importer rejects a track with `name conflict in the scene`, first-person arms rotate ~90°, drift/stretch, or fingers stay curled, a first-person camera won't move, a scripted scene errors `unable to find animation '<name>' in tree 'all_player'`, or a CoDMayaTools export throws `notetracks[N]`/Py3 errors or the linker rejects an xcam. Distinct from bo3-animation (export2bin/APE-xanim pipeline), bo3-assets (model/material porting), and bo3-crossref (reads another title's source; doesn't retarget).
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
- **Getting a loose `.xmodel_bin` into Maya as a target rig — unpack it, don't recompile-and-re-rip.** A `.xmodel_bin` is an **LZ4-compressed `.xmodel_export`**, so mesh *and* skeleton are recoverable as text with a converter (`export2bin.exe`, `exportxbin.exe`, or `exportx.exe` — the one that worked from the command line). Verify `NUMBONES`/`NUMVERTS`/`NUMFACES` in the result before importing via CoDMayaTools. Exact invocation, tool sources, and the fallback when a converter fails to decompress: **`references/workflow-extras.md`**.

---

# Path A — full body, via HumanIK

The retarget is done in **Maya with HumanIK** (the built-in retargeter) — manual per-joint constraints are a dead end for cross-gen work (parentConstraint stretches the rig on bone-length differences; orientConstraint-only leaves the local-axis mismatch unfixed). Rip rigs and anims as **Cast** (SEanim/SEModel are deprecated).

**The order that works — do exactly this (source first, then target, then link):** import the old-CoD (source) rig alone (fresh Cast import = bind pose) → characterize it (joint-mapping table, full detail in the reference below) → **Lock it while still in bind** → only now import the anim onto the source rig → import the BO3 (target) rig (also fresh = bind) → characterize it the same way → set the target's **Source = the source character** to retarget live, scrub to check → **Bake** onto the target skeleton, then export (Select > Hierarchy). The single invariant that makes or breaks it is **Lock each rig in bind before any animation touches it** — full workflow, the joint-role table, why manual constraints fail in detail, and transferring fingers/camera afterward: **`references/path-a-humanik.md`**.

That reference also covers three traps worth knowing exist before you hit them: HumanIK's Lock **snapshots the current pose as reference**, so characterizing on an already-posed rig bakes the wrong "rest" into every frame — re-locking does not fix this, only deleting the HIK character and re-characterizing in bind does; a **locked** rig with a Source set is being driven every evaluation, so measuring its joints reads the solver, not the skeleton (unlock and restore bind pose first); and two rigs sharing joint names in one scene make the Cast anim importer reject every shared track with `name conflict in the scene` — prefix the target's names, never the source's, and strip the prefix before export.

---

# Path B — viewhands (first-person arms), via `-mo` constraints

A **viewhands** rig is arms-only — no `j_mainroot`, no spine, no legs — so **HumanIK cannot characterise it at all**: Lock only enables once the required biped bones are filled, and they never will be. Drive it with direct constraints instead: `-mo` on every constraint (captures the bind-pose offset — a plain `orientConstraint` copies the source's absolute axis and twists every joint at rest), and co-locate the two rigs' roots *before* constraining, or that offset bakes in a lever arm that swings the arms wide once the source rotates. Pick the constraint type per joint from the source anim's **measured** per-bone translation amplitude, not from habit — a joint driven mostly by translation (e.g. a shoulder) wants `pointConstraint -mo` + `orientConstraint -mo`, not a plain `parentConstraint`, and vice versa. Finger names are **not** a reliable 1:1 map across generations — an index shift plus a rename can silently move every phalanx one joint down the chain even though enough short names match to look correct.

**The trap that actually causes "arms rotated 90° in game":** the source rig (e.g. MW3) can keep a torso tag permanently rotated by some constant offset and author the arms in that frame, while BO3 expects that same tag at identity. `-mo` faithfully copies the convention, offset included, and that constant rotation is what reads in-game as arms tilted off to the side. **Fix:** after baking, zero the torso tag's rotation and restore the shoulders' world transforms (sampled per frame first) — everything below the shoulders keeps its own local keys and follows.

Full derivation — the constraint-type table, the finger-remap table, a set of measured dead ends already ruled out, and the three post-bake fixes for arm-length mismatch, the multi-joint wrist-twist chain, and curled fingers: **`references/path-b-viewhands.md`**. Read the numbers there as one worked example (measured on a single MW3→`c_zom_der_dempsey_viewhands` port), not as constants — the techniques transfer to any title pair, the specific angles and offsets do not.

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

To tweak an artifact non-destructively after bake, use an **additive anim layer** — but a freshly created empty layer shows the dense BASE keys in the timeline/Graph Editor, which looks like it "inherited every frame" when it hasn't. The timeline only reflects a layer's own keys once the joint has been explicitly added to it, and HIK **Source = None** (previous section) is a prerequisite or every scrub bakes a real key onto the active layer. Full sequence, including the 2-key trick for a constant misalignment: **`references/workflow-extras.md`**.

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
- **An animated prop (not a character) needs `AnimScripted`, not `scene::play`/`SetAnim`.** Playing a ported IGC fxanim (rope, cloth, debris) via a scene bundle's `MainAnim`, or via `SetAnim` on its animtree, leaves the mesh **frozen** — and don't `LinkTo` it to a moving parent either, co-locate origins instead so it tracks for free. Full mechanism and a worked rappel-rope case: **`references/workflow-extras.md`**.

## Moving the first-person CAMERA: three mechanisms, pick by case

Start from the finding that costs the most time, because it holds in every case: **a scene does NOT drive the player's first-person view in ZM.** The body anim plays (you can confirm it in third person), but the camera stays put. Two ground-truth reasons:

- In the shipped `scripts/shared/scene_shared.gsc`, the player view-lock calls in `reset_player` (`ShowViewModel`, `StartCameraTween`, …) are **commented out `//TODO_CODE: only supported for SP`**. The scene's first-person camera path is single-player only.
- The compiled viewbody model often **exposes no camera tag at runtime** — both `entity LinkTo(model, "tag_camera")` and `model GetTagOrigin("tag_camera")` fail/return undefined, even though the anim animates `tag_camera` and the `.XMODEL_EXPORT` lists it. So you cannot drive the camera off the model's tag either.

Verifying the anim is *not* the problem: parse the exported `.xcam_export`/`.xanim_export` and confirm `tag_camera`'s offset **varies across frames** (many distinct offsets, not one) — if it moves in the file, the camera data is fine and the wall is the engine/model, not your export.

What follows is **not** "always use an XCam" — it's that the view must be driven by something other than the scene. Three mechanisms do that; pick by what the clip has to do:

| the clip | mechanism |
|---|---|
| a cinematic on a **`Player`-object scene** | **XCam** — `PlayMainCamXCam` (CSC). See below and `references/xcam-camera.md`. |
| a mostly-static **viewmodel** | play it through a **weapon**; the viewmodel's own `tag_camera` moves the view directly. Verified — but a dead end once the camera has to *travel* (see the weapon note above). |
| a **get-up / mantle that travels** | **link the player to an animated node** playing a *camera-less* scene bundle: `PlayerLinkToDelta` onto a mount `LinkTo`'d one `GetPlayerViewHeight()` **below** the node's moving `tag_camera`, so the eye lands on the animated camera. This is MW3's `_id_72AD` transposed, and the path that actually carried a traveling first-person get-up; the GSC lives in **bo3-scripting**. |

Counter-pressure worth knowing whichever you pick: every stock/ported reference keeps `tag_torso` / `tag_cambone` / `tag_camera` **static**, with all motion in the arm joints. An animated `tag_camera` works, but it is off the beaten path — so if the arms misbehave, suspect the tags before the arms.

### The XCam path

An **XCam** is a dedicated camera animation asset, played per-client with **`PlayMainCamXCam` (CSC)** — this is how the shipped campaign/MP cinematics drive their cameras, and because it's per-client it also satisfies "every player sees the cinematic" in co-op. Built in Maya from the retargeted rig's own `tag_camera` (a camera snapped to the tag, axis-corrected, then re-constrained with `-mo` and baked), exported via **Call of Duty Tools → Export XCam**. The xcam asset needs **`use_firstperson_player` = 1** to render the first-person body/arms during playback (the missing-hands fix lives on the asset, not the scene) and a `parent_scene` pointing at the scene bundle that animates the body; play it with `PlayMainCamXCam(localClientNum, "<xcam>", lerp, "<subcam>", "", origin, angles)` from CSC.

One correction worth surfacing here because it silently wastes time otherwise: **the export's `fov` field is a decoy — the game derives runtime FOV from `flen` (focal length) instead**, under the export's `"aperture": "FOCAL_LENGTH"` mode, and ignores `fov` (and `cg_fov`) entirely. Lower `flen` = wider FOV; edit it (and `aspectratio`, `1.7786` for 16:9) directly in the `.xcam_export` for fast iteration instead of re-exporting for every tweak.

Full Maya camera setup steps, the xcam asset's other fields, and the complete FOV/DOF export-field conversion table: **`references/xcam-camera.md`**.

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
