#!/bin/sh
# SessionStart: compare the t7kb binary's version with this plugin's. A release
# ships both at the same version, but they update separately (`claude plugin
# update` never touches the binary, the installer never touches the plugin), so
# skills can name MCP tools the installed binary doesn't have. Offline: it only
# runs `t7kb --version`. Silent when they match, when the binary isn't found,
# or for a local dev build.

root="${CLAUDE_PLUGIN_ROOT:-$(dirname "$0")/..}"
plugin="$(tr -d ' "\r' < "$root/.claude-plugin/plugin.json" 2>/dev/null | grep -m 1 '^version:' | cut -d: -f2 | tr -d ',')"
[ -n "$plugin" ] || exit 0

# T7KB_BIN, else where the installers put it
bin="${T7KB_BIN:-}"
if [ -z "$bin" ]; then
  for c in "${LOCALAPPDATA:-}/t7kb/t7kb.exe" "$HOME/.t7kb/t7kb"; do
    if [ -f "$c" ]; then bin="$c"; break; fi
  done
fi
[ -n "$bin" ] || exit 0

binary="$("$bin" --version 2>/dev/null | sed -n 's/^t7kb version v*//p')"
[ -n "$binary" ] && [ "$binary" != "dev" ] && [ "$binary" != "$plugin" ] || exit 0

msg="t7kb: the installed t7kb binary is $binary but the t7kb plugin (its skills) is $plugin, so tools a skill names may be missing from the t7kb MCP server, or skills may not know a tool it has. Run /t7kb:setup to update the binary, or run t7kb update-check for the plugin update commands."
printf '{"systemMessage":"%s","hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$msg" "$msg"
