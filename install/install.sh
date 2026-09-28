#!/usr/bin/env bash
# Universal installer for t7kb — the BO3-modding knowledge search tool.
# Downloads the latest release (binary + embedding model) and the database into
# one directory; any MCP-capable agent (Claude Code, Codex, OpenCode, Copilot,
# Cursor) then points at "<dir>/t7kb mcp". Re-runnable, and skips the ~0.9 GB
# download if already installed — pass --force to reinstall/update.
#
#   curl -fsSL https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.sh | bash
#   ./install.sh [target-dir] [--force]      # default target: ~/.t7kb
set -euo pipefail

REPO="McReaper/t7_companion"
BASE="https://github.com/$REPO/releases/latest/download"

FORCE=0
TARGET="$HOME/.t7kb"
for arg in "$@"; do
  case "$arg" in
    --force|-f) FORCE=1 ;;
    *) TARGET="$arg" ;;
  esac
done

case "$(uname -s)" in
  Linux) ARCHIVE="t7kb_linux_amd64.tar.gz" ;;
  Darwin)
    echo "No macOS build is published yet (releases are linux/amd64 + windows/amd64)." >&2
    echo "On Apple Silicon/Intel macOS, run under a Linux environment for now." >&2
    exit 1 ;;
  *) echo "Unsupported OS '$(uname -s)'. On Windows, use install.ps1." >&2; exit 1 ;;
esac

if [ "$FORCE" -ne 1 ] && [ -x "$TARGET/t7kb" ] && { [ -f "$TARGET/t7kb.db" ] || [ -f "$TARGET/t7kb.db.zip" ]; }; then
  echo "t7kb is already installed at $TARGET (binary: $TARGET/t7kb). Skipping download."
  echo "Pass --force to reinstall/update."
  exit 0
fi

have() { command -v "$1" >/dev/null 2>&1; }
fetch() { # fetch <url> <out>
  if have curl; then curl -fSL --progress-bar "$1" -o "$2"
  elif have wget; then wget -q --show-progress "$1" -O "$2"
  else echo "need curl or wget" >&2; exit 1; fi
}

mkdir -p "$TARGET"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Installing t7kb to $TARGET"
echo "  downloading $ARCHIVE …"
fetch "$BASE/$ARCHIVE" "$tmp/$ARCHIVE"
tar -xzf "$tmp/$ARCHIVE" -C "$TARGET"
chmod +x "$TARGET/t7kb"

echo "  downloading t7kb.db.zip (~0.9 GB) …"
fetch "$BASE/t7kb.db.zip" "$TARGET/t7kb.db.zip"
# Left zipped on purpose: the binary unpacks it once on first run (saves a
# redundant 3.5 GB write here and keeps one code path for it).

cat <<EOF

t7kb installed.  Binary: $TARGET/t7kb

Register it as an MCP server in your agent (command + args):
    command: $TARGET/t7kb
    args:    ["mcp"]

The database unpacks automatically the first time the server runs.
See the README for per-client config (Claude Code, Codex, OpenCode, Copilot, Cursor).
EOF
