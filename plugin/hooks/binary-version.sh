#!/bin/sh
# SessionStart: say when this t7kb install is out of date. First the t7kb
# binary's version against this plugin's: a release ships both at the same
# version, but they update separately (`claude plugin update` never touches the
# binary, the installer never touches the plugin), so skills can name MCP tools
# the installed binary doesn't have. When they agree, whether a newer release
# is out: `t7kb update-check --quiet`, the plugin's only network call, none
# under CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC, silent offline or on error.
# Silent when all is current, when the binary isn't found, or for a local dev
# build.

root="${CLAUDE_PLUGIN_ROOT:-$(dirname "$0")/..}"
plugin="$(tr -d ' "\r' < "$root/.claude-plugin/plugin.json" 2>/dev/null | grep -m 1 '^version:' | cut -d: -f2 | tr -d ',')"
[ -n "$plugin" ] || exit 0

# T7KB_BIN, else where the installers put it
bin="${T7KB_BIN:-}"
if [ -z "$bin" ]; then
  for c in "${LOCALAPPDATA:-}/t7kb/t7kb.exe"; do
    if [ -f "$c" ]; then bin="$c"; break; fi
  done
fi
[ -n "$bin" ] || exit 0

binary="$("$bin" --version 2>/dev/null | sed -n 's/^t7kb version v*//p')"
[ -n "$binary" ] && [ "$binary" != "dev" ] || exit 0

# say tells the user and the agent; its text must hold no `"` or `\`.
say() {
  printf '{"systemMessage":"%s","hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$1" "$1"
}

if [ "$binary" != "$plugin" ]; then
  say "t7kb: the installed t7kb binary is $binary but the t7kb plugin (its skills) is $plugin, so tools a skill names may be missing from the t7kb MCP server, or skills may not know a tool it has. Run /t7kb:setup-or-update to update the binary, or run t7kb update-check for the plugin update commands."
  exit 0
fi

[ -z "${CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:-}" ] || exit 0
latest="$("$bin" update-check --quiet 2>/dev/null | tr -cd 'A-Za-z0-9.+-')"
[ -n "$latest" ] || exit 0
say "t7kb: $latest is out (this install is $binary). /t7kb:setup-or-update updates the binary and the plugin."
