Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — turning a plan into brushes through t7kb's map tools. Read the parent skill first, and `map-tools.md` for the sizes to build to and the other tools' traps.

Contents:

- [Place each wall where the plan draws it, nothing overlapping](#place-each-wall-where-the-plan-draws-it-nothing-overlapping)
- [A door is an opening with a lintel, clear for the zombies](#a-door-is-an-opening-with-a-lintel-clear-for-the-zombies)
- [Cut doors by building around the hole, not with CSG Subtract](#cut-doors-by-building-around-the-hole-not-with-csg-subtract)

## Place each wall where the plan draws it, nothing overlapping

Nothing lays a plan out for the agent: each wall is a `brush` through its corners, 8 thick on the plan's line, corners on the 8-unit grid; a slanted wall's corners follow the plan, so it keeps its angle. Where walls meet nothing overlaps: mitre a corner (each wall's two corners there on the mitre line), stop a wall that ends on another at its face. Floors and ceilings are slabs, none over another (two slabs over one spot z-fight, flickering in game), a concave floor cut into convex brushes. Doors come from the plan's own marks — a gap between jambs, a swing arc — never from gaps a trace left: those are a trace's errors, and a door there is a wall missing. An exit to the outside stays a wall: past it there is no floor. One batch per room or corridor keeps each one undo step.

## A door is an opening with a lintel, clear for the zombies

zm_giant's doorways are an opening with a lintel and a 2-unit `weaponClip detail` trim (verified in the install): the wall's two sides, a lintel from 96 to the top, the trim. Leave the AI 72 clear, two abreast: a zombie is 30 wide (`physRadius` 15) and they push each other, so 56 (Treyarch's door) jams a group, and three can still jam 72. To show and pass players 56, narrow it with `nonColliding` jambs, each doubled by a brush with `clip_player` on **every** face: one caulk face makes the clip solid to the AI, and the navmesh, which `radiant/configs/navmesh.json` builds through `clip_player`, stops there (verified on a real build, the navmesh read back).

## Cut doors by building around the hole, not with CSG Subtract

**`Selection/CSG/CSG Subtract` carves the selected brush out of every brush it touches, volumes included**, with no warning: a door cutter through a wall of the template's start room also split the start zone's `info_volume`, the `volume_sun`, the `umbra_volume` and the `volume_fpstool` six ways each (verified in a test Radiant). A zone or sun volume in pieces no longer covers what it did. Build a wall with a door as brushes around the hole instead — the wall's two sides and the lintel above, in one batch. Done by mistake, one Ctrl+Z (`t7kb:map_command` with `Edit/Undo`) takes the whole subtract back.
