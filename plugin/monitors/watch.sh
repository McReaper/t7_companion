#!/bin/sh
# The plugin's monitor: runs `t7kb watch`, which prints one line per problem
# the game has (a script error, a fatal error, a crash, the assets the map's
# zone lists that the game didn't find), per build the mod tools run (a
# link's verdict, a compile's leak) and per thing Radiant reports (its errors,
# a prefab entered, a bake ended), each one a notification to the agent.
# Only in a session working inside the game's root: the mod tools set
# TA_TOOLS_PATH and TA_GAME_PATH for the whole machine, so they're found from
# any session. Quiet, and gone, outside it, when t7kb can't be found, or the
# binary predates the command.

# The session's directory against each root, as Windows compares paths: either
# slash, any case.
norm() { printf '%s' "$1" | tr '\\' '/' | tr 'A-Z' 'a-z' | sed 's:/*$::'; }
here="$(norm "$(pwd -W 2>/dev/null || pwd)")"
inside=
for r in "${TA_GAME_PATH:-}" "${TA_TOOLS_PATH:-}"; do
  r="$(norm "$r")"
  [ -n "$r" ] || continue
  case "$here/" in "$r/"*) inside=1 ;; esac
done
[ -n "$inside" ] || exit 0

# T7KB_BIN, else where the installers put it
bin="${T7KB_BIN:-}"
if [ -z "$bin" ]; then
  for c in "${LOCALAPPDATA:-}/t7kb/t7kb.exe"; do
    if [ -f "$c" ]; then bin="$c"; break; fi
  done
fi
[ -n "$bin" ] || exit 0

exec "$bin" watch 2>/dev/null
