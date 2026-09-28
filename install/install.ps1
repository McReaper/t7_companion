# Universal installer for t7kb on Windows — the BO3-modding knowledge search tool.
# Downloads the latest release (binary + embedding model) and the database into
# one directory; any MCP-capable agent (Claude Code, Codex, OpenCode, Copilot,
# Cursor) then points at "<dir>\t7kb.exe mcp". Re-runnable, and skips the
# ~0.9 GB download if already installed — pass -Force to reinstall/update.
#
#   irm https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.ps1 | iex
#   .\install.ps1 [-Target <dir>] [-Force]    # default target: %LOCALAPPDATA%\t7kb
param([string]$Target = "$env:LOCALAPPDATA\t7kb", [switch]$Force)

$ErrorActionPreference = "Stop"
$repo = "McReaper/t7_companion"
$base = "https://github.com/$repo/releases/latest/download"

$exe = Join-Path $Target "t7kb.exe"
$db = Join-Path $Target "t7kb.db"
$dbZip = Join-Path $Target "t7kb.db.zip"

if (-not $Force -and (Test-Path $exe) -and ((Test-Path $db) -or (Test-Path $dbZip))) {
    Write-Host "t7kb is already installed at $Target (binary: $exe). Skipping download."
    Write-Host "Pass -Force to reinstall/update."
    exit 0
}

New-Item -ItemType Directory -Force -Path $Target | Out-Null
$tmp = Join-Path $env:TEMP "t7kb_install.zip"

Write-Host "Installing t7kb to $Target"
Write-Host "  downloading t7kb_windows_amd64.zip ..."
curl.exe -L --fail --progress-bar -o $tmp "$base/t7kb_windows_amd64.zip"
Expand-Archive -Path $tmp -DestinationPath $Target -Force
Remove-Item $tmp -Force

Write-Host "  downloading t7kb.db.zip (~0.9 GB) ..."
# Left zipped on purpose: the binary unpacks it once on first run.
curl.exe -L --fail --progress-bar -o $dbZip "$base/t7kb.db.zip"

Write-Host @"

t7kb installed.  Binary: $exe

Register it as an MCP server in your agent (command + args):
    command: $exe
    args:    ["mcp"]

The database unpacks automatically the first time the server runs.
See the README for per-client config (Claude Code, Codex, OpenCode, Copilot, Cursor).
"@
