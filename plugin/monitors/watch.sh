#!/bin/sh
# The plugin's monitor: runs `t7kb watch`, which prints one line per problem
# the game has (a script error, a fatal error, a crash, the assets the map's
# zone lists that the game didn't find), per build the mod tools run (a
# link's verdict, a compile's leak) and per thing Radiant reports (its errors,
# a prefab entered, a bake ended), each one a notification to the agent.
#
# Only in a session working inside the game's root: the mod tools set
# TA_TOOLS_PATH and TA_GAME_PATH for the whole machine, so they're found from
# any session. Claude Code starts the monitor in every session and wakes the
# agent when it exits, so outside the root it idles for the session and says
# why on stderr, the task's output, which wakes no one. Inside, what keeps the
# watch from running (t7kb not found, its error) goes to stdout: the agent
# hears why.
#
# Stopping the monitor kills the shell Claude Code started, and a process Git
# Bash starts is outside that shell's Windows process tree, so the kill misses
# it: the script ends itself, and the watch, once that shell is gone.

parent=$PPID
every=5

# Whether the shell that started the monitor still runs. Started by a program
# outside Git Bash, the script has no parent it can see (PPID 1), so it can't
# tell, and runs on.
alive() { [ "$parent" = 1 ] || kill -0 "$parent" 2>/dev/null; }

# Say why on stderr, then wait out the session.
idle() {
  echo "t7kb watch is off: $1" >&2
  while alive; do sleep "$every"; done
  exit 0
}

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
if [ -z "$inside" ]; then
  root="${TA_TOOLS_PATH:-${TA_GAME_PATH:-}}"
  [ -n "$root" ] || idle "no BO3 folder: the mod tools set TA_TOOLS_PATH, and it is unset."
  idle "this session was opened outside the BO3 folder, $root. Your agent starts it when it builds, runs the game or works in Radiant."
fi

# T7KB_BIN, else where the installers put it
bin="${T7KB_BIN:-}"
if [ -z "$bin" ]; then
  for c in "${LOCALAPPDATA:-}/t7kb/t7kb.exe"; do
    if [ -f "$c" ]; then bin="$c"; break; fi
  done
fi
if [ -z "$bin" ]; then
  echo "t7kb watch can't start: t7kb isn't installed (no ${LOCALAPPDATA:-%LOCALAPPDATA%}/t7kb/t7kb.exe, and T7KB_BIN is unset). /t7kb:setup-or-update installs it."
  exit 1
fi

# The watch, its errors on stdout, and beside it a guard that stops it once the
# monitor's shell is gone; the guard's output goes nowhere, so a sleep it
# leaves behind holds no pipe open.
"$bin" watch 2>&1 &
w=$!
(
  while alive; do sleep "$every"; done
  kill "$w" 2>/dev/null
) >/dev/null 2>&1 &
g=$!
wait "$w"
code=$?
kill "$g" 2>/dev/null
exit "$code"
