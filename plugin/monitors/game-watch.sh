#!/bin/sh
# The plugin's game monitor: runs `t7kb game watch`, which prints one line per
# problem the game has (a script error, a fatal error, a crash, the assets the
# map's zone lists that the game didn't find), each one a notification to the
# agent. Quiet, and gone, when t7kb or the game's root can't be found, or the
# binary predates the command.

# T7KB_BIN, else where the installers put it
bin="${T7KB_BIN:-}"
if [ -z "$bin" ]; then
  for c in "${LOCALAPPDATA:-}/t7kb/t7kb.exe" "$HOME/.t7kb/t7kb"; do
    if [ -f "$c" ]; then bin="$c"; break; fi
  done
fi
[ -n "$bin" ] || exit 0

exec "$bin" game watch 2>/dev/null
