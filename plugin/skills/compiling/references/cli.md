# `t7kb build` — the shell fallback

Referenced from `plugin/skills/compiling/SKILL.md`. Read that file first — this is for when no MCP server is registered this session, or you want the CLI's `--verbose` output.

`t7kb build` runs the same pipeline as the MCP tool, every gotcha `SKILL.md` describes handled, and prints a compact per-stage summary instead of the hundreds of lines each tool spews; `--json` gives the report the MCP tool returns. `t7kb build --help` lists every flag.

```
t7kb build zm_mymap                          # usermap: compile,light,link (reads $TA_TOOLS_PATH)
t7kb build zm_mymap --stages link            # script-only iteration — just re-link
t7kb build my_mod --mod --stages link        # a mod's zone
t7kb build zm_mymap --stages link,run --dvar developer=2 --dvar logfile=2   # relink, then play with a fresh log
```

It exits non-zero when a stage fails and prints the parsed error (a linker `SCRIPT ERROR … line N`, the dialog the game stopped on): hand that to **t7kb:debugging**.
