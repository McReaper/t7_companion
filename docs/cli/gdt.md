# GDT assets: read, write and check them without APE

The rules come from the install itself: each asset type's fields from `deffiles/<type>.awi`, as APE shows them, and a material's texture slots from its techset. Every answer is JSON. [All commands](../cli.md).

## `t7kb gdt find <asset>`

Every GDT that defines the asset: file, line, type, parent, and whether the GDT is stock. Two definitions make the link fail with `Duplicate asset`.

```
t7kb gdt find mtl_p7_zm_bgb_ghost
```

## `t7kb gdt get <asset>`

The asset's fields, those it inherits from its parents included, and what's wrong with them against the schema (and, for a material, its techset).

```
t7kb gdt get mtl_p7_zm_bgb_ghost --filter Map
```

| Option | What it does |
| --- | --- |
| `--filter` | only the fields whose name contains this |
| `--all` | empty and zero fields too |
| `--file` | the GDT to read, when several define the asset |

## `t7kb gdt schema [type]`

The fields a type declares: kind, default, range, the choices of a combo. With no type, every asset type. For a material with `--material-type`, the techset's texture slots and the image semantic each wants.

```
t7kb gdt schema material --material-type lit --filter colorMap
```

| Option | What it does |
| --- | --- |
| `--filter` | only the entries matching this |
| `--material-type` | for materials: read this material type's techset |

## `t7kb gdt edit`

Creates or changes an asset, checked against the schema first: ranges, combo values, unknown keys, a material's techset, missing source files. Without `--write` it shows the changes and the issues and saves nothing. A write is atomic, keeps a `.bak`, leaves every other asset byte for byte, and is refused if the file changed meanwhile. Stock GDTs are refused: copy the stock asset into your own GDT with `--copy-from`.

```
t7kb gdt edit --file source_data/zm_leviathan.gdt --asset my_sign --type material --set materialType=lit --set colorMap=my_sign_c
t7kb gdt edit --file source_data/zm_leviathan.gdt --asset my_sign --set surfaceType=metal --write
```

| Option | What it does |
| --- | --- |
| `--file`, `--asset` | the GDT, relative to the install's root, and the asset (both required) |
| `--type` | create a new asset of this type |
| `--copy-from` | create it as a copy of this asset, stock ones included |
| `--parent` | create it derived from this asset, which must be in the same GDT |
| `--set`, `--unset` | set `key=value`, or remove a key; repeatable |
| `--image-texture` | create an image asset from this texture file; its semantic comes from `--image-semantic`, or from the slot it fills (`--image-material-type` and `--image-field`) |
| `--batch` | several assets from a JSON file (`-` for stdin), applied in order and saved all or nothing |
| `--write` | save |

## `t7kb gdt check <file.gdt>`

Checks a whole GDT the way the linker would, before you build: field values, techset rules, that each referenced asset exists with the right type, source files on disk, missing parents, names defined twice. It lists only the assets that have a problem.

```
t7kb gdt check source_data/zm_leviathan.gdt
t7kb gdt check source_data/zm_leviathan.gdt --asset my_sign
```

## `t7kb gdt refs <asset>`

Every asset, in every GDT, that names this one: through a field, as a parent, or for a material inside an xmodel's LOD or collision file. Run it before renaming or deleting an asset.

```
t7kb gdt refs my_sign_c
```
