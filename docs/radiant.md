# Radiant integration

Your agent works in the Radiant you have open: it opens a map, finds what's in it, looks at its views, places and edits brushes, patches and entities, and runs Radiant's own menu commands. You watch each change land in your editor, and take any of them back with Ctrl+Z.

## What you need

- **[MTPlugins](https://github.com/Aidan601/MTPlugins)**, which loads plugins into Radiant.
- **[T7RadiantBridge](https://github.com/McReaper/t7_radiant_bridge)**, the plugin t7kb talks to. It has no release yet: its first version is in review in [McReaper/t7_radiant_bridge#3](https://github.com/McReaper/t7_radiant_bridge/pull/3).
- t7kb itself, [installed](../README.md#-install) with its MCP server in your agent.

## What the agent can do

| Action | Tool | What happens |
| --- | --- | --- |
| Open a map | `radiant_open` | In the Radiant that has it open, else the one running (never over unsaved changes unless you say so), else a new Radiant started on it. |
| Find things | `map_find` | Entities by class, key, distance to a point or your selection, with short ids and bounds; their brushes and patches on request. |
| Look | `map_view` | An image of the 3D view or a top, front or side view, with ids written on it, optionally under Radiant's baked lighting. Your camera and views are put back after. |
| Place and edit | `map_edit` | Brushes by their corners, patches by their control points, map text pasted as is, point entities; keys, moves, copies, deletions, materials and per-face textures. |
| Use Radiant's tools | `map_command` | Any menu entry (CSG Hollow, the Patch menu, File > Save…) on the things it names. |
| Build and play | `build` | Compile, light, link and launch the game, with cod2map's and the linker's problems in the report: [Maps and mods](cli/build.md). |

Every tool has a command-line twin: [the Radiant commands](cli/radiant.md).

## What stays in your hands

- **One undo per batch.** Each `map_edit` batch is one entry in Radiant's Edit menu, named `t7kb: …`; a `map_command` leaves the one entry Radiant's own command makes.
- **Your edits win.** A batch that names something you changed or deleted since the agent last read the map is refused whole; the agent reads again first.
- **Only what you ask.** The agent places what you ask for and leaves the rest of the map alone: materials, props, lights, probes and mood are yours unless you hand them over.
- **Radiant holds the map.** The agent never writes the `.map` file behind Radiant's back, since Radiant's next save would overwrite it; it saves through Radiant before a build.

What each call costs in tokens: [map-tools-tokens.md](map-tools-tokens.md).
