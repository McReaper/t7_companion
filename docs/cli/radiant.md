# Radiant: the map open in your Radiant

Every map edit happens in the Radiant you have open, through the [T7RadiantBridge](https://github.com/McReaper/t7_radiant_bridge) plugin (loaded by MTPlugins). [All commands](../cli.md).

## `t7kb radiant open <map>`

Opens a map, by name (`zm_leviathan` is `map_source/zm/zm_leviathan.map`) or `.map` path. It uses the Radiant that has the map open already; else the one running, which loads it in place of its own map (refused over unsaved changes, under a dialog or inside a prefab); else it starts Radiant on the map and waits for it, up to 3 minutes. Never edit a `.map` file Radiant may hold: its next save overwrites it.

```
t7kb radiant open zm_leviathan
```

| Option | What it does |
| --- | --- |
| `--discard` | drop that Radiant's unsaved changes to load the map |
| `--pid` | the Radiant to use when several run |
