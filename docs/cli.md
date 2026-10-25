# The t7kb CLI

Every tool your agent calls is also a terminal command that does the same thing: the MCP tool `gdt_edit` is `t7kb gdt edit`, `zone_check` is `t7kb zone check`. The pages below say what each command is for, with an example and the options you'll reach for; `t7kb <command> --help` lists them all.

| Page | Commands | For |
| --- | --- | --- |
| [Knowledge](cli/knowledge.md) | `t7kb`, `search`, `get` | looking something up in the modding knowledge base |
| [Maps and mods](cli/build.md) | `create`, `build`, `game log` | starting a map or mod from a template; compiling, lighting, linking and launching it; what the game printed while it ran |
| [GDT assets](cli/gdt.md) | `gdt find`, `get`, `schema`, `edit`, `check`, `refs` | reading, writing and checking assets without APE |
| [What a build packs](cli/zone.md) | `zone explain`, `contents`, `check` | why an asset is in your map, what each zone line weighs, what won't ship |
| [Shaders](cli/shader.md) | `shader decompile` | a stock shader's HLSL, to start a custom one from |
| [Radiant](cli/radiant.md) | `radiant open`, `map find`, `view`, `edit`, `command` | the map open in your Radiant: finding, seeing and editing what's in it |

## Options every command shares

| Option | What it does |
| --- | --- |
| `--tools-path` | the BO3 mod-tools root, for every command that reads the install; default `$TA_TOOLS_PATH` |
| `--db` | the knowledge base file; default `$T7KB_DB`, then `t7kb.db` beside the binary, then in the current folder |
| `--write` | `create` and `gdt edit` only show what they would change until you pass it |

## `T7KB_DEBUG`: a journal of every call

Set `T7KB_DEBUG` to a folder, or to `1` for `debug/` beside t7kb, and t7kb records each command and each MCP tool call it serves in that folder's `journal.jsonl`, one JSON line each: when, the command or tool, what it was given (its arguments, the ops it read from stdin), what it answered, whether it failed, how long it took. The images `map view` answered are saved beside it. Read it to see what an agent did in your Radiant, call by call. For the MCP server, set it in your client's config, as `claude mcp add t7kb -e T7KB_DEBUG=1 -- <path to t7kb> mcp`. Unset, t7kb records nothing.

## `t7kb mcp`

Runs the MCP server, the surface your agent uses. Your client starts it; [clients.md](clients.md) has the setup per client.

## `t7kb watch`

Says, one line each, as it happens, until you stop it: each problem the game has (a script error, a fatal error, a crash, the assets the map's zone lists that the game didn't find once it has loaded), each build the mod tools run, from the Launcher or from t7kb: a link's errors, warnings, and the assets it named that are in no GDT, or that it went fine; a compile's leak and the problems its `.errlog` places; and what a Radiant whose bridge runs reports: its errors (a model or material no GDT defines), another map opened, a prefab entered or left, a bake ended, Radiant closing. It reads what the game and the tools write, once the tool has quit, and says each thing once. In Claude Code, the t7kb plugin runs it for the whole session, so your agent hears of each without you telling it; `game log` and the `zone` commands then investigate.

```
t7kb watch
```

## `t7kb update-check`

Says whether a newer t7kb release is out and whether your Claude Code plugin is behind it, with the commands to update each. It never downloads anything: the README's [Updating](../README.md#-updating) section says how to update.
