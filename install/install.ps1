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

# curl.exe is a native command: a failed download sets $LASTEXITCODE, it doesn't throw.
function Get-Release([string]$name, [string]$out) {
    Write-Host "  downloading $name ..."
    curl.exe -L --fail --progress-bar -o $out "$base/$name"
    if ($LASTEXITCODE -ne 0) { throw "downloading $name failed (curl exit $LASTEXITCODE)" }
}

$updating = Test-Path $db
New-Item -ItemType Directory -Force -Path $Target | Out-Null
$tmp = Join-Path $env:TEMP "t7kb_install.zip"

Write-Host "Installing t7kb to $Target"
Get-Release "t7kb_windows_amd64.zip" $tmp

# Each open agent session runs this t7kb.exe as its MCP server, which holds the
# exe and t7kb.db open, and Windows can't replace an open file: stop them.
# Each session starts the new one when it restarts.
Get-Process t7kb -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $exe } |
    Stop-Process -Force -PassThru | Wait-Process -Timeout 10 -ErrorAction SilentlyContinue

Expand-Archive -Path $tmp -DestinationPath $Target -Force
Remove-Item $tmp -Force

Get-Release "t7kb.db.zip" $dbZip
# Left zipped on purpose: the binary unpacks it on its next start. On an
# update that means deleting the old database, which would otherwise stay.
if ($updating) { Remove-Item $db -Force }

# No exit here: piped into iex, it would close the user's PowerShell.
if ($updating) {
    Write-Host @"

t7kb updated.  Binary: $exe
Its MCP server was stopped: restart your agent sessions. The first one to
start unpacks the new database (~3.8 GB, about 20 s, once).
"@
} else {
    Write-Host @"

t7kb installed.  Binary: $exe

Register it as an MCP server in your agent (command + args):
    command: $exe
    args:    ["mcp"]

The database unpacks automatically the first time the server runs.
See the README for per-client config (Claude Code, Codex, OpenCode, Copilot, Cursor).
"@
}
