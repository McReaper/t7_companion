# The raw Launcher binaries — last resort

Referenced from `plugin/skills/compiling/SKILL.md`. Read that file first — this is the fallback for when neither the `t7kb:build` MCP tool nor the `t7kb build` command is available: each stage's command line, and the shell traps the two tools already handle for you.

`t7kb:build` and `t7kb build` already run these exact command lines for you, gotchas and all — reach for them directly only when neither is available, never as a shortcut around them. `%T` = `TA_TOOLS_PATH`, `%G` = `TA_GAME_PATH` (both the BO3 root; the `TA_*` vars must be set — the tools resolve their paths from them), `<map>` = full map name, `<pp>` = its first two letters (`mp`/`zm`), `<mod>`/`<zone>` = mod container and zone name.

```
# 1. Index GDTs (always first; assets edited but not indexed link stale) — run from %T\gdtdb\
%T\gdtdb\gdtdb.exe /update

# 2. Compile map geometry (BSP) — run from %T\bin\
%T\bin\cod2map64.exe -platform pc -navmesh -navvolume -loadFrom %G\map_source\<pp>\<map>.map %G\share\raw\maps\<pp>\<map>.d3dbsp
#   entity-only fast recompile: swap "-navmesh -navvolume" for "-onlyents"

# 3. Bake lighting / LEDs — headless, no GUI (quality: +low | +medium | +high)
%T\bin\radiant_modtools.exe -ledSilent +medium +localprobes +forceclean +recompute %G\map_source\<pp>\<map>.map

# 4. Link Fast Files — <lang> is the game's own language (below)
%T\bin\linker_modtools.exe -language <lang> -modsource <map>                          # a map
%T\bin\linker_modtools.exe -language <lang> -fs_game <mod> -modsource <zone>           # a mod (repeat per zone)

# 5. Run — with SteamAppId=311210 and SteamGameId=311210 in the environment (below)
%G\BlackOps3.exe +set fs_game <map> +devmap <map>      # a usermap
%G\BlackOps3.exe +set fs_game <mod>                    # a mod
```

Notes: link in **the game's own language**, the first line of `%G\localization.txt` (else the language Steam installed it in): the game loads `<lang>_<map>.ff` and stops on `Could not find zone '<lang>_<map>'` without it (verified on a French install; Treyarch's launcher repeats `-language <lang>` per language for an all-languages build); the linker prints an `L3akMod` banner then the zone's link log; a failing stage names itself in its output — feed that to **t7kb:debugging**. Run only the stages whose input changed (see *Iterate fast*): a script-only change is `gdtdb /update` → `linker … -modsource` and nothing else.

## Shell gotchas (verified on a real headless build)

- **Run these from PowerShell or `cmd`, not git-bash/MSYS.** MSYS rewrites the `/update` and `+low`/`+medium` arguments into filesystem paths (silently breaks `gdtdb` and the light step) *and* mis-reports a native exe's exit code — a clean `exit 0` can come back as `127`. In PowerShell read the true code from `$LASTEXITCODE`.
- **Run `cod2map64` with the working directory set to `bin/`.** It loads `default_navmesh_settings.json` from the current directory; launched from elsewhere it aborts navmesh with `ERROR: Unable to load navigation mesh generation settings` (the geometry `.d3dbsp` still writes, but you get no navmesh — AI won't path).
- **Run `gdtdb` with the working directory set to its own `gdtdb/` folder**, as the Launcher does. It records asset paths relative to its cwd, so run from anywhere else against a Launcher-built database it flags **every asset as a duplicate** — which reads as a corrupted GDT set rather than a wrong folder.
- **Start the game with Steam's ids.** `BlackOps3.exe` started from a plain shell can't attach to the running Steam client and refuses with "you must launch Steam to play the game"; with `SteamAppId=311210` and `SteamGameId=311210` set, the same command runs (the Launcher, started by Steam, passes them on).
- **The light step detaches.** `radiant_modtools.exe -ledSilent` is a GUI-subsystem exe: it returns immediately with no captured stdout and no usable exit code, then bakes in the background. Wait for it by polling for the output `.led` (or for the process to exit), not on a synchronous return.

Confirm any flag not shown here against the raw install / t7kb before relying on it.
