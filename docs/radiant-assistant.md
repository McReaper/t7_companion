# The agent in your Radiant

**The map is yours; the agent places only what you ask, each element its own.** Its geometry detail, materials, props, light fixtures, reflection probes and mood stay the mapper's choice. The agent works in the Radiant you have open, through the T7RadiantBridge plugin (hosted by MTPlugins), and every batch it sends is one entry in Radiant's undo menu, named `t7kb: …`, so one Ctrl+Z takes it back. The tools are `radiant_open`, `map_find`, `map_view`, `map_edit` and `map_command`; the `t7kb:mapping` and `t7kb:lighting` skills hold the method.

## What it takes on now

- **Placing what you ask, with nothing laid out for it**: a brush through its corners (a box, a wedge, a slanted wall, a column), a patch through its control points, map text pasted as is, entities. A blockout from a plan, or zones, go in brush by brush, each one in the batch for you to read.
- **Edits** on what it names: keys, moves, copies, deletes, materials and per-face textures.
- **Radiant's own commands** (CSG, patch tools) on what it names, and the build and a launch of the game: cod2map's and the linker's own warnings are the map's check.
- **No edit over yours**: a batch naming something you changed or deleted since the agent read it is refused whole, so it reads again before acting.

