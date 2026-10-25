# Radiant: the map open in your Radiant

These commands work on the map Radiant has open, as it is now, unsaved edits included, through [T7RadiantBridge](https://github.com/McReaper/t7_radiant_bridge), the bridge t7kb loads into it. [Radiant integration](../radiant.md) has what to install and what stays in your hands. Each command takes `--pid` to pick a Radiant when several run, and its `--help` gives the whole description of the MCP tool it is, with each argument's. [All commands](../cli.md).

Entities and brushes go by short ids, the first 8 hex digits of their Radiant GUID, as `map find` lists them. `map find`, `map view` and `map command` take them after the command or as `--ids`, comma- or space-separated.

## `t7kb radiant open <map>`

Opens a map, by name (`zm_leviathan` is `map_source/zm/zm_leviathan.map`) or `.map` path. It uses the Radiant that has the map open already; else the one running, which loads it in place of its own map (refused over unsaved changes, under a dialog or inside a prefab); else it starts Radiant on the map and waits for it, up to 3 minutes. Never edit a `.map` file Radiant may hold: its next save overwrites it.

```
t7kb radiant open zm_leviathan
```

| Option | What it does |
| --- | --- |
| `--discard` | drop that Radiant's unsaved changes to load the map |

## `t7kb map find [id…]`

The entities that match every filter given, each with its id, its rev (what `map edit` wants back), class and origin; a brush entity also its bounds and brush count. Only the naming keys show (`targetname`, `target`, `script_noteworthy`, `model`…) unless you ask for others.

```
t7kb map find --class "light*"
t7kb map find --near 0,0,64 --radius 512
t7kb map find 5ec8de46 --brushes
```

| Option | What it does |
| --- | --- |
| `--class` | classname; `*` matches any run of characters |
| `--key` | a key the entity has, or `key=value` |
| `--near`, `--radius` | what comes within the radius of a point `x,y,z` (default 256) by bounds, nearest first: an entity that owns brushes, the worldspawn too, by its brushes', listing those near the point (its bounds and count stay the whole entity's) |
| `--selected` | only the entities with a brush selected in Radiant |
| `--brushes` | each entity's brushes and patches; a patch named by its id comes with its control points |
| `--keys` | which keys to show: `all`, or a comma list |
| `--limit` | how many entities at most (default 50) |

## `t7kb map view [id…]`

Saves an image of the 3D view or a 2D view as Radiant draws it, with the ids written where each thing is (in 3D, only what no wall hides from the camera), and prints how the image maps to the world. Your camera, render mode and 2D views are put back after. Radiant mustn't be minimised, and a screen that went to sleep is turned back on for the shot: Windows draws nothing under a sleeping screen. A 3D shot from a camera waits, up to 5 s, while you hold a mouse button.

```
t7kb map view --view top --out top.png
t7kb map view 5ec8de46 --render fullbright
t7kb map view --camera 0,-512,128,-10,90,0 --bake
```

| Option | What it does |
| --- | --- |
| `--view` | `3d` (default), `top`, `front` or `side`; top shows a layout best |
| `--at`, `--radius` | frame a point `x,y,z` and what's around it |
| `--camera` | 3D: shoot from `x,y,z,pitch,yaw,roll` (positive pitch looks up, yaw 90 faces +y) |
| `--render` | 3D: `fullbright` for textures, `fake` for shapes, or `wireframe`, `real`, `view`, `density` |
| `--bake` | 3D: bake Radiant's lighting first and shoot under it, to see sun leaks and dark rooms; Radiant then asks, when you close it, whether to export the lighting |
| `--hide` | `volumes`, `sky` or both: hide them for this shot |
| `--size`, `--out` | the image's longer side (default 768 pixels) and where to write it |
| `--no-labels` | leave the ids off |

## `t7kb map edit --ops <file|->`

Applies a batch of ops, a JSON array, in order: it lands as one entry of Radiant's undo menu, named `t7kb: …`, so one Ctrl+Z takes it all back, or it doesn't land at all. Every element of the map an op names needs the rev `map find` gave it, in `--revs`: a batch naming something you changed or deleted since is refused whole. The answer reads the map back: each op's id, rev and bounds, the tile each material got.

| Op | What it places or changes |
| --- | --- |
| `brush` | a convex brush through its corners (a box is its 8, a wedge its 6), in a material, in the world or as a new brush entity (`class`) |
| `patch` | a patch through its control points, rows of as many each: a curve (3, 5, 7… each way) or a terrain mesh |
| `raw` | map text pasted as Radiant writes it: entities with their brushes and patches |
| `entity` | a point entity at an origin, with angles and keys |
| `set`, `move`, `copy`, `delete` | keys (`null` removes one), a move by an offset, copies each further on, deletions |
| `material`, `texture` | a material swapped, each face keeping its tile; a material laid on some faces with its tile, origin, rotation |

`@n` names what op `n` of the batch made, counting from 0, so a brush placed, textured and copied is one undo step. `t7kb map edit --help` gives each op's fields.

```json
[
  {"brush": {"points": [[0,0,0],[256,0,0],[256,256,0],[0,256,0],[0,0,16],[256,0,16],[256,256,16],[0,256,16]], "material": "caulk"}},
  {"copy": {"ids": ["@0"], "by": [0,0,128], "count": 2}}
]
```

```
t7kb map edit --ops slabs.json --label "three slabs"
t7kb map edit --ops move.json --revs '{"5ec8de46":"9f3a0c1b"}'
```

| Option | What it does |
| --- | --- |
| `--ops` | the ops' JSON file, `-` for stdin (default) |
| `--revs` | each named element's rev, as JSON `{"id": "rev"}`: a brush's own, not its entity's |
| `--label` | the undo entry's name (default: the ops counted) |

## `t7kb map command <menu path> [id…]`

Runs one of Radiant's own menu entries on the ids named, selected alone for it; your selection comes back after. It's for what `map edit` has no op for: CSG, the patch tools, textures, prefabs. The answer reads the map back: the undo entry, what the command created and deleted, what it left selected. A command that opens a dialog waits on you.

```
t7kb map command --menu Selection/CSG
t7kb map command "Selection/CSG/CSG Hollow" 5ec8de46
```

| Option | What it does |
| --- | --- |
| `--menu` | list a menu's entries instead, a submenu marked by `>`: a menu is its labels from the menu bar down, joined by `/` (`Selection/CSG`), a label holding a `/` written as it reads; `--menu=`, empty, lists the menu bar |
