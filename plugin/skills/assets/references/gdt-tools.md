# The `gdt_*` MCP tools

Referenced from `plugin/skills/assets/SKILL.md` — read that file first. The t7kb MCP server reads and edits GDTs with the install's own rules as the schema: `deffiles/<type>.awi` for the fields APE declares, the techsetdef a `materialType` resolves to for a material's texture slots and category.

## What each tool is for

`t7kb:gdt_find` (where an asset is defined, duplicates, stock or not), `t7kb:gdt_get` (fields, inherited ones included), `t7kb:gdt_schema` (what APE declares for a type, read from its `deffiles/*.awi`; for a material, the texture slots and category its techset really has), and `t7kb:gdt_edit` — which copies a donor with its LOD paths cleared, escapes backslashes, refuses stock GDTs and duplicate names, and validates ranges, combo values, color/vector shapes, `materialType`/`materialCategory` agreement and image semantics before writing (dry run by default). The hand-authoring traps in `SKILL.md` are what it guards against.

- **A material with its own textures is one `gdt_edit` call:** pass `assets` with an `image` item per texture (`{"texture": "texture_assets/…/wall_n.tif", "material_type": "lit", "field": "normalMap"}` — the semantic comes from that techset slot, every other setting from the most common value among stock images of that semantic, and a missing or non-power-of-two texture is an error), then the material naming those images. Items apply in order and the file is written only if none has an error, so a half-made set never lands.
- **Before building, `t7kb:gdt_check` the GDT.** It reports what would otherwise surface one link error at a time: a reference to an asset that's in no GDT or is the wrong type (an xmodel in a `colorMap`), an xmodel/xanim export or texture missing on disk, a missing parent, a name defined in two GDTs.
- **Before renaming or deleting an asset, `t7kb:gdt_refs` it** — every asset that names it in a field or derives from it. Material names baked into an `.xmodel_bin` aren't searched: a model's materials come from its export.
