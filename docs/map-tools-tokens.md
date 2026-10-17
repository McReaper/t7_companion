# Map tools: tokens per call

What each of t7kb's map tools costs an agent, from `TestTokenReport` (how to run it: [benchmarks.md](benchmarks.md)).

Measured on the stock ZM Mod Level template's map (33 entities, 20 world brushes), what a map made by `create` holds, served by `radianttest`'s fake Radiant with the views of a real one: a 1648×869 3D view, a 693×1165 2D view (`mapSamples` in `internal/cli/tokens_report_map_test.go`). An image counts as Claude prices it, about width × height / 750 tokens; the JSON beside it, and the ops an agent writes, as text.

| Tool | Call | Tokens | Bytes |
|---|---|---:|---:|
| `radiant_open` | the map already open | 53 | 183 |
| `map_find` | every entity (33) | 1859 | 5895 |
| `map_find` | class `script_struct` (15) | 884 | 3097 |
| `map_find` | near the player start, 256 units | 802 | 2557 |
| `map_find` | worldspawn's 20 brushes | 831 | 2073 |
| `map_find` | one entity, all keys | 94 | 282 |
| `map_view` | top, framed on the start zone: JSON + 457×768 image | 48 + 467 | 117 + 3477 |
| `map_view` | 3D, framed, `render fake`: JSON + 768×405 image | 52 + 414 | 129 + 2610 |
| `map_view` | 3D as it is, `size 1568`: JSON + 1568×827 image | 84 + 1728 | 190 + 8402 |
| `map_edit` | the ops an agent writes for a room (floor, 4 walls, ceiling, a light) | 254 | 643 |
| `map_edit` | the answer to them, with each material's tile | 434 | 1068 |
| `map_edit` | the ops for a floor, two walls caulked top and bottom, a pillar copied three times | 238 | 556 |
| `map_edit` | the answer to them (7 made, 5 materials' tiles) | 323 | 818 |
| `map_edit` | a `layout` op: two rooms, a door between, a window | 130 | 323 |
| `map_edit` | the answer to it (17 pieces) | 832 | 2116 |
| `map_edit` | a `zone` op: an L outline, linked to the start zone | 76 | 181 |
| `map_edit` | the answer to it (a volume, 4 spawners, a pathnode, the script's call, a note) | 372 | 1192 |
| `map_edit` | moving a point entity | 57 | 157 |
| `map_command` | listing `Selection/CSG` | 32 | 111 |
| `map_command` | `CSG Hollow` on a box | 206 | 579 |
| `map_check` | the template and the rooms above, 50 findings listed | 1664 | 5170 |
| `map_check` | seams, stretch and unknown materials only | 210 | 608 |

A room costs about twelve hundred tokens: its ops (254), the answer (434), a top view to check it (515); a `layout` of two rooms, door and window included, is 130, its answer 832; a `zone` over them is 76, its answer 372 with the script's missing call and the overlap with the template's start zone. The whole map's `map_find` is the dearest call, about 56 tokens an entity, so `class`, `near` or `ids` narrow it; the default image, 768 pixels on the long side, stays near 450 tokens, and `size 1568` quadruples it. A finding of `map_check` costs about 32: `kinds` narrows a long one.
