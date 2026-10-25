# Maps and mods: create, build and the game's log

`create` and `build` drive the mod tools you have installed, as the Launcher does, without its window; `game log` reads what the game printed while it ran. [All commands](../cli.md).

## `t7kb create <name>`

A new usermap or mod from the install's templates, as the Launcher's File > New makes one: for a map, its `.map` under `map_source/` and its scripts, zone and sound config under `usermaps/<name>/`; for a mod, its zone files under `mods/<name>/`. A map's name starts with `zm_` or `mp_`. Without `--write` it only lists the files it would create, and it never overwrites one.

```
t7kb create zm_leviathan
t7kb create zm_leviathan --write
```

| Option | What it does |
| --- | --- |
| `--write` | create the files |
| `--template` | a folder of `rex/templates` (default `ZM Mod Level` for `zm_`, `MP Mod Level` for `mp_`, `Mod` otherwise) |
| `--zones` | mods only: the zones to keep, among `core`, `mp`, `cp`, `zm` (default all) |

## `t7kb build <name>`

Runs the build pipeline on a map, or a mod with `--mod`: `gdtdb` first (so your GDT edits reach the link), then the stages you ask for. The build runs on its own and the command returns at once; `--status` then says how far it got, and gives its report once done, a short result per stage. One build of a map or mod runs at a time.

| Stage | What runs |
| --- | --- |
| `compile` | `cod2map64` turns the `.map` into the `.d3dbsp` |
| `light` | Radiant's lighting pass, at `--light` quality |
| `link` | `linker_modtools` packs the fastfile: its errors and warnings come back whole, with what it looked for in no GDT |
| `run` | starts the game on the map, writing a fresh `console_mp.log`, and checks its start; [`t7kb watch`](../cli.md#t7kb-watch) and `game log` take it from there |

```
t7kb build zm_leviathan
t7kb build zm_leviathan --status
t7kb build zm_leviathan --stages link,run --wait
t7kb build my_mod --mod --stages link
```

After a script-only change, `--stages link` is enough. A full compile and light can take tens of minutes.

| Option | What it does |
| --- | --- |
| `--wait` | run the build in the terminal and print its report as it ends |
| `--status` | the state of the map's last build: the stage running, its report once done |
| `--stages` | which stages, comma-separated (default `compile,light,link`) |
| `--mod` | the name is a mod (`mods/<name>`), not a usermap |
| `--light` | light quality: `low`, `medium` (default) or `high` |
| `--onlyents` | a fast compile of the entities alone; invalid once a brush changed since the last full compile |
| `--language` | the language to link for |
| `--fresh-xpak` | link: write the `.xpak` from scratch instead of growing it; use it before publishing, since the whole `zone/` folder is uploaded |
| `--dvar` | run: start the game with this dvar, `name=value`, repeatable (`--dvar developer=2`); `logfile` is 2 unless you set it |
| `--launcher-dvars` | run: start the game with the dvars saved in the Launcher's Dvars dialog |
| `--game-path` | the game's root, if not the mod tools' (default `$TA_GAME_PATH`) |
| `--skip-gdt` | skip the `gdtdb` pass |
| `--gdt-rebuild` | rebuild the GDT database instead of updating it: only when an update reports 0 GDTs and the link then misses an edited asset |
| `--verbose` | with `--wait`: stream each tool's whole output as it runs |
| `--json` | the report as JSON, as the agent gets it |

## `t7kb game log [map-or-mod]`

What the game printed while it ran, from its `console_mp.log`: for when something goes wrong in game. It finds the current log (the newest of the game root's and each mod's), and sorts its last launch into script errors with their call stacks, the map's own problems (assets its zone lists that the game didn't find, other Error lines) and a count of the stock lookups every load fails, each with its line number in the file. It also names the other logs beside it, `crashes.log` among them.

```
t7kb game log zm_leviathan
t7kb game log zm_leviathan --grep "zombie_door" --context 5
t7kb game log --tail 100
```

| Option | What it does |
| --- | --- |
| `--grep` | the last launch's lines matching a regular expression, case-insensitive, with the lines around them |
| `--context` | lines before and after each match (default 2) |
| `--max` | matches given at most (default 50); the rest are counted |
| `--tail` | the last launch's last lines |
| `--path` | a log file to read instead of the current one |
| `--after` | read only the lines after this line of the file: what is new since an earlier read |
