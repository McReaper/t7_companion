# Path B — viewhands (first-person arms), via `-mo` constraints

Referenced from `plugin/skills/anim-retarget/SKILL.md`. Read that file first — this covers the direct-constraint retarget for arms-only rigs in depth: constraint-type selection per joint, the finger-name remapping trap, the "arms rotated 90°" fix, dead ends already ruled out, and the three post-bake fixes (arm length, wrist-twist chain, curled fingers).

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
