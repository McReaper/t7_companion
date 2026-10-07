# `t7kb build` — the shell fallback

Referenced from `plugin/skills/compiling/SKILL.md`. Read that file first — this is for when no MCP server is registered this session, or you want the CLI's `--json`/`--verbose` output.

`t7kb build` runs the whole pipeline with every gotcha `SKILL.md` describes handled — cwd, arg passing, the detached light poll, output-file verification — and prints a **compact per-stage summary** (or `--json`) with the first actionable error, instead of the hundreds of lines each tool spews:

```
t7kb build zm_mymap                          # usermap: compile,light,link (reads $TA_TOOLS_PATH)
t7kb build zm_mymap --stages link            # script-only iteration — just re-link
t7kb build my_mod --mod --stages link        # a mod's zone
t7kb build zm_mymap --onlyents --json        # fast entity-only compile, machine-readable report
```

Flags: `--stages compile,light,link,run`, `--light low|medium|high`, `--onlyents`, `--mod`, `--tools-path`/`--game-path` (default `$TA_TOOLS_PATH`/`$TA_GAME_PATH`), `--verbose` to stream raw tool output, `--fresh-xpak` (MCP `fresh_xpak`) to write the `.xpak` from scratch for the build you publish; for the run stage, `--launcher-dvars` and `--dvar name=value` (MCP `launcher_dvars`, `dvars`). The run stage starts the game as the Launcher's Run does — `+set fs_game <map> +devmap <map>` for a usermap, `+set fs_game <mod>` for a mod — and reports a failure if the game exits within a few seconds (Steam not running or not signed in). It exits non-zero and surfaces the parsed error (e.g. a linker `SCRIPT ERROR … line N`) when a stage fails — hand that to **t7kb:debugging**.
