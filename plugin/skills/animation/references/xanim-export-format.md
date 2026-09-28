# Reading a `.xanim_export` by hand

Referenced from `plugin/skills/animation/SKILL.md` — read that file first. The text format's layout, what its numbers mean, and why translating an export is a no-op for playback.

The text format is simple enough to inspect or patch with a script, which is worth knowing because it settles arguments that are otherwise guesswork: a header (`NUMPARTS`, then `PART <i> "<joint>"`), then one `FRAME n` block per frame listing each `PART i` with its `OFFSET x y z`, `SCALE`, and three `X`/`Y`/`Z` rotation rows.

Two facts about the numbers, both **verified by inspection** on a Treyarch character rig:

- **`PART 0` is `tag_origin` — the root — and `PART 1` is `j_mainroot`.** Travel is the root's, so measure `PART 0`. Measuring `j_mainroot` instead manufactures a discrepancy of a few units that does not exist, and sends you hunting a bug that was never there.
- **`OFFSET`s are absolute in the animation's own space**, not parent-relative. So translating a whole clip really is one constant subtracted from every `OFFSET` of every `PART` on every frame.

**And that translation is a no-op for playback.** It is tempting — shift an export so the body ends at `(0,0,0)`, and unlinking should leave the entity exactly where the clip finished. It does nothing: `AnimScripted` is handed the **starting** transform and the engine reads travel from the root track, so shifting the file moves start and end together and the played result is identical. When an anim lands in the wrong place the **anchor** is wrong, not the file — fix it in script with `GetStartOrigin`/`GetStartAngles` (place the entity so the clip lands where you want) or `GetMoveDelta( anim, 0, 1, ent )` (read the travel), and don't touch the export. See **t7kb:scripting** for playing it and **t7kb:moving-platforms** for the moving-parent case.
