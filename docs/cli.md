# The t7kb CLI

Every tool your agent calls is also a terminal command that does the same thing: the MCP tool `gdt_edit` is `t7kb gdt edit`, `zone_check` is `t7kb zone check`. The pages below say what each command is for, with an example and the options you'll reach for; `t7kb <command> --help` lists them all.

| Page | Commands | For |
| --- | --- | --- |
| [Knowledge](cli/knowledge.md) | `t7kb`, `search`, `get` | looking something up in the modding knowledge base |
| [Maps and mods](cli/build.md) | `create`, `build` | starting a map or mod from a template; compiling, lighting, linking and launching it |
| [GDT assets](cli/gdt.md) | `gdt find`, `get`, `schema`, `edit`, `check`, `refs` | reading, writing and checking assets without APE |
| [What a build packs](cli/zone.md) | `zone explain`, `contents`, `check` | why an asset is in your map, what each zone line weighs, what won't ship |
| [Shaders](cli/shader.md) | `shader decompile` | a stock shader's HLSL, to start a custom one from |

## Options every command shares

| Option | What it does |
| --- | --- |
| `--tools-path` | the BO3 mod-tools root, for every command that reads the install; default `$TA_TOOLS_PATH` |
| `--db` | the knowledge base file; default `$T7KB_DB`, then `t7kb.db` beside the binary, then in the current folder |
| `--write` | `create` and `gdt edit` only show what they would change until you pass it |

## `t7kb mcp`

Runs the MCP server, the surface your agent uses. Your client starts it; [clients.md](clients.md) has the setup per client.

## `t7kb update-check`

Says whether a newer t7kb release is out and whether your Claude Code plugin is behind it, with the commands to update each. It never downloads anything: the README's [Updating](../README.md#-updating) section says how to update.
