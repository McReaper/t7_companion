Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — turning a plan or a brief into brushes through t7kb's map tools. Read the parent skill first, and `map-tools.md` for the traps of working through the tools.

Contents:

- [Read the plan before placing anything](#read-the-plan-before-placing-anything)
- [Place each wall where the plan draws it, nothing overlapping](#place-each-wall-where-the-plan-draws-it-nothing-overlapping)
- [A door is an opening with a lintel, wide enough for the zombies](#a-door-is-an-opening-with-a-lintel-wide-enough-for-the-zombies)
- [Cut doors by building around the hole, not with CSG Subtract](#cut-doors-by-building-around-the-hole-not-with-csg-subtract)

## Read the plan before placing anything

A plan says where the walls, the openings and the exits are; it rarely says how big the spaces should play. Find its scale from something it measures (a dimension line, a door, a known room), then check the spaces against gameplay: corridors and doors a group of zombies passes. A zombies map trades real proportions for that, so a plan's sizes are a start: when a space plays too tight, scale the plan up or ask the mapper, and look up Treyarch's reference sizes in t7kb (`Scale_Standards.pdf`) rather than settling on a number alone. Say what scale was taken and why, in the reply, so the mapper can correct it before the walls go in.

## Place each wall where the plan draws it, nothing overlapping

Each wall goes on the plan's line, its corners on the 8-unit grid; a slanted wall's corners follow the plan, so it keeps its angle. Where walls meet nothing overlaps: mitre a corner (each wall's two corners there on the mitre line), stop a wall that ends on another at its face. Floors and ceilings are slabs, none over another (two slabs over one spot z-fight, flickering in game), a concave floor cut into convex brushes. Openings come from the plan's own marks — a gap between jambs, a swing arc — never from gaps a trace left: those are a trace's errors, and an opening there is a wall missing. An exit to the outside stays a wall unless the mapper wants one: past it there is no floor. One batch per room or corridor keeps each one undo step.

## A door is an opening with a lintel, wide enough for the zombies

Treyarch's doorways are an opening with a lintel and a trim (zm_giant's: the wall's two sides, the wall continuing above as a lintel, a 2-unit `weaponClip detail` trim round the opening; verified in the install). How wide is the plan's or the mapper's; what decides it is the AI: a zombie is 30 wide (`physRadius` 15) and zombies push each other, so a door one player wide jams a group, and two side by side is what keeps them flowing. When the door should look narrower to players than the AI's way through, narrow it with `nonColliding` jambs, each doubled by a brush with `clip_player` on **every** face: anything that narrows a door stops players only, never the AI. One caulk face makes such a clip solid to the AI, and the navmesh, which `radiant/configs/navmesh.json` builds through `clip_player`, stops there (verified on a real build, the navmesh read back).

## Cut doors by building around the hole, not with CSG Subtract

**`Selection/CSG/CSG Subtract` carves the selected brush out of every brush it touches, volumes included**, with no warning: a cutter through a wall of the template's start room also splits the start zone's `info_volume`, the `volume_sun`, the `umbra_volume` and the `volume_fpstool` into pieces (verified in a test Radiant). A zone or sun volume in pieces no longer covers what it did. Build a wall with a door as brushes around the hole instead — the wall's two sides and the lintel above, in one batch. Done by mistake, one Ctrl+Z (`t7kb:map_command` with `Edit/Undo`) takes the whole subtract back.
