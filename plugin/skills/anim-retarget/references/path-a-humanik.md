# Path A — full body, via HumanIK

Referenced from `plugin/skills/anim-retarget/SKILL.md`. Read that file first — this covers the full-body/biped retarget in depth: the order of operations, why manual constraints fail, the bind-pose characterization trap, measuring a locked rig correctly, renaming around the two-rigs-one-scene conflict, when HumanIK itself is the wrong tool, the joint-mapping table, and transferring fingers/camera after the body retarget.

## Contents

- [The shape of the job](#the-shape-of-the-job)
- [Why not manual constraints](#why-not-manual-constraints)
- [The one trap that wastes the most time: characterize in BIND POSE, first](#the-one-trap-that-wastes-the-most-time-characterize-in-bind-pose-first)
- [Never measure a LOCKED rig — you are reading the solver, not the skeleton](#never-measure-a-locked-rig--you-are-reading-the-solver-not-the-skeleton)
- [Two rigs in one scene: the anim importer will refuse every shared bone name](#two-rigs-in-one-scene-the-anim-importer-will-refuse-every-shared-bone-name)
- [When HumanIK is the wrong tool](#when-humanik-is-the-wrong-tool)
- [Mapping CoD joints → HumanIK roles](#mapping-cod-joints--humanik-roles)
- [HumanIK doesn't retarget everything — fingers and camera need direct constraints](#humanik-doesnt-retarget-everything--fingers-and-camera-need-direct-constraints)

## The shape of the job

```
old-CoD rig + its anim  ──HumanIK retarget──▶  BO3 rig (baked keys)  ──export──▶ .xanim  ──(Player anim? add to all_player.atr)──▶ in-game
```

Retarget in **Maya with HumanIK**; manual per-joint constraints are a dead end cross-gen (below). Rip rigs and anims as **Cast** (SEanim/SEModel are deprecated).

**Order (source first, then target, then link):**

1. **Import the old-CoD (source) rig** alone — fresh Cast import is in bind pose.
2. **Characterize it** (map the joints, table below).
3. **Lock it** — *while still in bind.*
4. **Import the anim** onto the source rig (only now, after the Lock).
5. **Import the BO3 (target) rig** (also fresh = bind).
6. **Characterize it** (same mapping).
7. On the target, set **Source = the source character** → it retargets live; scrub to check.
8. **Bake** onto the target skeleton, then **export** (Select > Hierarchy).

The invariant is **step 3 before step 4**: Lock each rig in bind *before* any animation touches it (next sections).

## Why not manual constraints

"Constrain each BO3 joint to the same-named old joint, then bake" **fails**:

- **`parentConstraint` on every joint explodes the rig.** It forces each child joint to the old rig's world **position**; since bone *lengths* differ between generations, the skeleton stretches/shatters within a few frames.
- **`orientConstraint` on all + `parentConstraint` on the root only** stops the explosion (positions now come from BO3's own bone lengths) but still **does not fix the local-axis difference** — the limbs stay mis-rotated, because copying world orientation joint-by-joint ignores that "zero rotation" means a different pose on each skeleton.

HumanIK maps both skeletons to a common biped rig and resolves the axis/bind-pose difference.

*(Path B has to solve this by hand, with `-mo` — because HumanIK isn't available there at all.)*

## The one trap that wastes the most time: characterize in BIND POSE, first

HumanIK's **Lock/characterize snapshots the skeleton's current pose as its reference pose.** If the rig is already **posed by the animation** when you Lock it (e.g. the anim opens mid-action — sitting, crouched), HIK records that as "rest," and every retargeted frame is offset from a wrong reference → the same twisting you were trying to fix.

So: import the rig **alone** (fresh Cast = bind pose), **characterize + Lock in bind**, *then* import the anim.

Once an anim is on the rig, `gotoBindPose` fails (it needs the skinned **mesh shape**; `No shape found` on a joint or group) and `doEnableNodeItems false animCurve` merely **freezes at the current frame**. No re-import needed — restore the stored bind pose:

```python
cmds.select("Joints1", hierarchy=True)
cmds.dagPose(restore=True, g=True, bindPose=True)      # g=global, or the root's placement is left behind
```

`bindPose*` nodes are written by Maya at skin time and survive everything; `cmds.ls(type="dagPose")` tells you they're there (one per rig). Scripted equivalent of `Skin > Go to Bind Pose` (Rigging menu set, **F3**).

**Re-locking is NOT enough to fix a bad reference pose.** Unlock, restore bind, re-Lock — and the retarget comes back *bit-identical*, because unlock only re-opens the bone **mapping**; the stance was captured when the character was **created**. The real fix keeps the rig and throws away the character: Source → None, restore bind, **Character Controls → Character → Delete**, create a new character, map, Lock *in bind*. Example (BO2→BO3 body port): the wrist gap went from **14.03 left / 2.35 right** to **6.40 / 6.38**, the residual being the genuine A-pose difference between generations.

**Save each definition** (Character Controls → *Export Character Definition*, written to `…/HIKCharacterizationTool6/template/*.xml`). You reuse them for every other anim you port — *Import Character Definition* re-applies the mapping (still locked in bind) in seconds instead of re-clicking ~20 bones. They are plain XML, so you can diff two definitions to spot a bad mapping.

## Never measure a LOCKED rig — you are reading the solver, not the skeleton

Before diagnosing "the wrist is off", check what drives the joints. A locked HIK character with a Source set **writes the target's joints every evaluation**: their local rotations are *solver output*, not bind pose. Any `setAttr` "fix" reads back fine, changes nothing in the viewport, and is wiped on the next solve (tell: the value sticks, the model doesn't move, re-locking "restores the bug").

**Detection trap:** HIK connects the **compound** `.rotate` plug. `listConnections(joint + ".rotateX", s=True, d=False)` returns **nothing** on a fully-driven joint — querying a child plug does not see a connection on its parent. Query both:

```python
for p in (".rotate", ".translate", ".rotateX"):
    if cmds.listConnections(j + p, s=True, d=False):
        ...   # driven
```

On a BO2→BO3 zombie pair this reports **22 driven joints** (an `HIKState2SK` node) when locked and **0** when unlocked. So: unlock → `dagPose` restore → *then* measure; a rig asymmetric under the solver can be symmetric in bind.

Useful invariant while measuring: on these rigs **mirrored joints carry identical local ROTATIONS** (the mirror lives in the joint orients), while their local **TRANSLATIONS mirror by negating one axis** — and *which* axis varies per joint (X at the shoulder, Z at the hip), so compare translations in absolute value. Comparing rotations for equality and translations for equal magnitude makes left/right asymmetry fall out immediately. Expect `j_hip` to diverge by ~180° between sides on both BO2 and BO3: that is the leg's mirror convention, not a defect — a genuinely wrong 180° hip puts the foot in the air, not 1 unit off.

## Two rigs in one scene: the anim importer will refuse every shared bone name

Cast puts each rig under a **group** (`Joints`, `Joints1`), *not* a namespace — so both skeletons own the same short names (72 in common on a BO2/BO3 zombie pair). The Cast **anim** importer then rejects tracks with `name conflict in the scene`.

**Prefix the TARGET, never the source.** The anim's tracks are keyed to the *source's* bone names; rename those and the anim binds to nothing. Renaming the target is safe in every direction that matters:

- **It does not break the characterization.** Maya connections are node-based, not name-based, so the HIK definition still resolves after the rename.
- **It must be undone before export.** An xanim carrying bones called `bo3_j_wrist_le` binds to no model at all. Strip the prefix after the bake, before exporting.

Rename **deepest-first** (`sort(key=lambda p: p.count("|"), reverse=True)`): renaming a parent invalidates the full DAG paths you already collected for its children.

## When HumanIK is the wrong tool

HIK is not universal: on a BO2→BO3 **full-body zombie** port, a correct, symmetric, bind-locked characterization still threw the arm out mid-animation, and only **direct constraints** worked — Path B's technique applied to a full body (orient everywhere with `-mo`, parent on `tag_origin`/`j_mainroot` to carry root motion, a direction-only pre-align so `-mo` absorbs only the axis convention).

That variant is easier than Path B because same-family rigs **share bone names** (72 of 73/78 on this pair, fingers included), so the pairing can be built by name at runtime. Bones present on one side only (`j_neck2`, `j_pinkybase_*`, `j_ringbase_*`, `tag_eye`, `j_head_end`) are left unconstrained and correctly follow their parent — the source has no motion to give them.

Try HIK first; use this when a *verified-correct* HIK setup still misbehaves.

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

HIK only drives the **biped roles**. Fingers and tags like `tag_camera` are left in bind. Transfer them *after* the body retarget, with direct constraints (source → target, **no `-mo`**, so they snap exactly):

- **Finger joints → `orientConstraint`** (rotation only). Never `parentConstraint` a finger — same bone-length-stretch explosion as above.
- **`tag_camera` (and other tags) → `parentConstraint`.** A tag has no children/bone-length, so position+rotation is safe, and this gives you the *exact* cinematic camera trajectory (e.g. snapping a mis-placed BO3 camera onto the correctly-placed source camera). This is the go-to for "this one bone sits in the wrong place."

General rule you can reuse for any single misplaced bone: select the **source** bone, then the **target** bone, `orientConstraint` (skeleton bone) or `parentConstraint` (tag).

> **Wrist caveat (known cross-gen pain point):** the hand/wrist is a frequent offender even after a good HIK retarget — the wrist's local axis and roll convention differ between generations, so the hand can read rotated/rolled while the forearm is fine. If HIK leaves the wrist twisted, fix it like fingers: an `orientConstraint` from the source `j_wrist_*` onto the target `j_wrist_*` before baking, and if it's a pure roll, correct that one axis by hand. Confirm the bone axes on your rig (empirical).
