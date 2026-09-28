# Playing animations from script: script_models, moving anchors, and the first-person camera

Referenced from `plugin/skills/scripting/SKILL.md` — read that file first. This covers the detail behind its animation summary: why `AnimScripted` is the primitive on a `script_model`, the four ways to move *and* animate something, and driving the player's first-person view from an animation.

## Contents

- [`AnimScripted` is the primitive on a script_model](#animscripted-is-the-primitive-on-a-script_model)
- [Moving *and* animating: four shapes](#moving-and-animating-four-shapes)
- [Driving the first-person camera from an animation](#driving-the-first-person-camera-from-an-animation)

## `AnimScripted` is the primitive on a script_model

On a plain `script_model` the **reliable playback primitive is `AnimScripted`** — which `scene::play` / `animation::play` wrap, and which shipped code also calls directly with a **string** anim anchored at a passed transform (e.g. `vehicle_death_shared` plays a crush anim: `self AnimScripted("anim_notify", self.origin, self.angles, crush_anim, "normal", …)`). **`SetAnim` and the `SetAnimKnob*` family are reported not to work on plain script_models in T7** — this is community-sourced (t7kb, ~0.25) and consistent with working map scripts that animate server script_models via `AnimScripted` instead, but it is *not* a shipped-token guarantee: treat it as a strong heuristic and test `SetAnim` on your own model before relying on it. `SetAnim` *does* work for **vehicles and AI** — their entity *type* carries an animtree/ASM, which is why a driving vehicle animates its turret relative to itself (`vehicle_shared`, `vehicleriders_shared`) — and for some CSC cases.

Two prerequisites before `AnimScripted` on a script_model:

- **Load an animtree:** `model UseAnimTree(#animtree)`, with `#using_animtree("generic")` (or a custom `.atr` you author/extend — the tree just has to contain the anim) at the top of the file. Skipping this is a classic "plays in APE, silent in-game." The `.atr` also needs its `rawfile,animtrees/<tree>.atr` zone line, or the server dies at load with no error (**t7kb:moving-platforms**).
- **Name the anim by string:** `model AnimScripted("notify", origin, angles, "my_xanim", "normal", "my_xanim", rate, blend)`. It **anchors at the `origin`/`angles` you pass** and plays the anim — root/delta motion included — from that **fixed world transform**; it does **not** track an entity you move afterwards. (`IsPlayingAnimScripted` / `StopAnimScripted(blend, b_clear)` manage it.)

A **siege** prop (`*_smod` model, APE shows *Is Siege*) is a different pipeline — `sanim` asset, client-side playback — see **t7kb:animation**.

## Moving *and* animating: four shapes

Given the anchor is frozen at play time, moving something while it animates is one of:

- **Re-anchor each frame** — drive a `script_model` align's `.origin` and re-issue the scene/`AnimScripted` at its new transform every tick. A working zipline does exactly this: its travel loop moves `align_model.origin` while a pose loop re-plays `scene::play(IDLE)` every 0.05s, so the pose re-anchors onto the moved align. **Only reach for this when there is no entity to link to.** When there is one, `animation::play(anim, ent, tag)` teleports, links and anchors once, and the engine does the interpolating; re-anchoring on top of a link holds alignment but visibly stutters (**t7kb:moving-platforms**).
- **Split phases** — movement by `LinkTo`/engine vehicle path with rotor/exhaust as **FX** (not anim), then hand off to one stationary anchored anim. BO1 Hue City's heli intro is this: a vehicle flies a node path in, is deleted, and a fake static model plays the anchored crash.
- **Bake the travel into the anim** — an anim carrying root motion slides the model along its *baked* path from the fixed anchor (a zip, a flythrough); fine when the path is fixed, useless when it's data-driven from Radiant nodes.
- **Make it a real vehicle/AI** — then `SetAnim` animates relative to the moving entity for free, at the cost of the full vehicle/ASM setup.

## Driving the first-person camera from an animation

An animation can move the player's **view**, not just render arms (a get-up, a mantle, a scripted first-person moment). The robust mechanism — transposed from MW3's `_id_72AD`, found by reading the source game per **t7kb:crossref** — uses **neither a weapon nor an XCam** (both were tried and were the wrong path for a camera-*moving* clip):

- Spawn a **node** (a viewhands `script_model`) and play the clip on it via a **camera-less scene bundle** (`scene::play`) — the node's animated `tag_camera` carries the motion.
- Link the player's view to it: `player PlayerLinkToDelta(mount, "tag_origin", 1, …)`. `PlayerLinkToDelta` seats the player's **ORIGIN** on its target and the engine then re-adds the player's own eye height — so link to a **mount** `LinkTo`'d one `GetPlayerViewHeight()` **below** the node's `tag_camera` (no magic number), and the eye lands on the animated camera.
- **Ground the clip:** play the node lowered by the low pose's *lowest-hand height above the anim root* (read it off the `.xanim_export`), so the downed hands touch the floor instead of hovering.

Make it multiplayer- and disconnect-safe: the scene bundle **AllowMultiple** (independent per-player instances); show the node **only to its owner** (`node SetInvisibleToAll(); node SetVisibleToPlayer(self);`) so nobody sees floating arms; and **own the teardown on a world entity** (the align/node), never `endon("death"/"disconnect")` on a thread holding the spawned entities — that skips cleanup and leaks them. Instead race the clip's end against `death`/`disconnect` and always Delete. Worked end-to-end on a ported first-person get-up; the Maya side (retargeting the arms onto BO3 viewhands) and the XCam alternative for cinematics are **t7kb:anim-retarget**.
