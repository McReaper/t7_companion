# The agent in your Radiant

**The map is yours; the agent takes the layout and the chores you hand it.** Its geometry detail, materials, props, light fixtures and mood stay the mapper's choice. The agent works in the Radiant you have open, through the T7RadiantBridge plugin (hosted by MTPlugins), and every batch it sends is one entry in Radiant's undo menu, named `t7kb: …`, so one Ctrl+Z takes it back. The tools are `radiant_open`, `map_find`, `map_view`, `map_edit`, `map_command` and `map_check`; the `t7kb:mapping` and `t7kb:lighting` skills hold the method.

## What it takes on now

- **A layout from a plan**: walls and doors drawn from a plan's lines, corners on the grid, joints mitred, every door the same size (72 clear for zombies, narrowed to 56 for players by `clip_player`, with a lintel and a trim); or a storey of rectangle rooms meeting along their edges.
- **Zones**: `info_volume`s over the floor players reach, riser spawners on it, a pathnode for the navmesh, and the `add_adjacent_zone` calls your script still lacks.
- **Reflection probes**: the floor players reach, walked from the spawn (stairs, storeys and prefabs included, kept to the zones), cut into boxes snapped to the walls, turned along the walls off the axes, floor to ceiling, none overlapping; the global probe moved off the sky.
- **Caulk and lint**: caulk on the faces nobody sees; `map_check` lists faces left drawn where nothing sees them, z-fighting, seams, stretched tiles and unknown materials, by brush and face.
- **Radiant's own commands** (CSG and the like) on what it names, and the build and a launch of the game.
- **No edit over yours**: a batch naming something you changed or deleted since the agent read it is refused whole, so it reads again before acting.

## Planned

- `map_check` reporting floor no zone covers, and surfaces no probe covers.
- Lights placed room by room from the fixture you choose: the spot under it and a probe-only fill.
- The lighting set as one: the map's SSI and exposure, the sun's shadow resolution, fog, probes and lights tuned together.
- Probes over the vistas players see but don't walk.
- Clip hulls over finished detail.
