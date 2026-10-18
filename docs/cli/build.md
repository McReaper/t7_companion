# Maps and mods: create and build

Both drive the mod tools you have installed, as the Launcher does, without its window. [All commands](../cli.md).

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

Runs the build pipeline on a map, or a mod with `--mod`, and prints a short result per stage: `gdtdb` first (so your GDT edits reach the link), then the stages you ask for.

| Stage | What runs |
| --- | --- |
| `compile` | `cod2map64` turns the `.map` into the `.d3dbsp` |
| `light` | Radiant's lighting pass, at `--light` quality |
| `link` | `linker_modtools` packs the fastfile: its errors and warnings come back whole, with what it looked for in no GDT |
| `run` | starts the game on the map |

```
t7kb build zm_leviathan
t7kb build zm_leviathan --stages link,run
t7kb build my_mod --mod --stages link
```

After a script-only change, `--stages link` is enough. A full compile and light can take tens of minutes.

| Option | What it does |
| --- | --- |
| `--stages` | which stages, comma-separated (default `compile,light,link`) |
| `--mod` | the name is a mod (`mods/<name>`), not a usermap |
| `--light` | light quality: `low`, `medium` (default) or `high` |
| `--onlyents` | a fast compile of the entities alone; invalid once a brush changed since the last full compile |
| `--language` | the language to link for |
| `--fresh-xpak` | link: write the `.xpak` from scratch instead of growing it; use it before publishing, since the whole `zone/` folder is uploaded |
| `--dvar` | run: start the game with this dvar, `name=value`, repeatable (`--dvar developer=2`) |
| `--launcher-dvars` | run: start the game with the dvars saved in the Launcher's Dvars dialog |
| `--game-path` | the game's root, if not the mod tools' (default `$TA_GAME_PATH`) |
| `--skip-gdt` | skip the `gdtdb` pass |
| `--gdt-rebuild` | rebuild the GDT database instead of updating it: only when an update reports 0 GDTs and the link then misses an edited asset |
| `--verbose` | stream each tool's whole output as it runs |
| `--json` | the report as JSON, as the agent gets it |
