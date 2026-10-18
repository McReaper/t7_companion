# What a build packs

The linker writes a report of everything it packed, and why, on every link; these commands read it, so they answer for your map's last link. When a source changed since, they say so: link again before trusting the answer. [All commands](../cli.md).

## `t7kb zone explain <map> <asset>`

Why the asset is in the build: the chain that pulled it in, from your `.zone` down (`.zone` → `.zpkg` → weapon → fx → material → image), and its size. It also says when the last link didn't pack it.

```
t7kb zone explain zm_leviathan t6_skybox
t7kb zone explain zm_leviathan zombie/fx_blood --type fx
```

| Option | What it does |
| --- | --- |
| `--type` | the asset type, when several assets share the name (`image`, `material`, `xmodel`, `fx`…) |

## `t7kb zone contents <map> [line]`

What a zone line pulls in: how many assets, their size in memory and streamed, counts per type, the largest. With no line, every line of the zone by weight, to see what makes the fastfile big, and what publishing uploads (the `zone/` folder), the `.xpak` data earlier links left in it included.

```
t7kb zone contents zm_leviathan
t7kb zone contents zm_leviathan "weapon,t8_knife_zm"
t7kb zone contents zm_leviathan --images
```

| Option | What it does |
| --- | --- |
| `--images` | weigh the images instead, by who defines them (your GDTs, stock, none), with what one `mipBase` step down would save: where the upload shrinks without cutting content |

## `t7kb zone check <map>`

Your versions of stock assets that won't ship: a stock script or asset you copied and zoned, or redefined in your own GDT, which a stock assetlist inherited through the map's `>class` still lists, so the linker packs a reference to the shipped one instead. It gives the assetlist line to comment out. It works before the first link; after one, it also checks every GDT asset the build drew on.

```
t7kb zone check zm_leviathan
```
