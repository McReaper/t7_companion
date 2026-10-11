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

New-Item -ItemType Directory -Force -Path $Target | Out-Null
$tmp = Join-Path $env:TEMP "t7kb_install.zip"

Write-Host "Installing t7kb to $Target"
Get-Release "t7kb_windows_amd64.zip" $tmp

# Each open agent session runs t7kb.exe, and Windows can't overwrite a running
# exe, but it can rename one: move it aside, and drop those no session still runs.
Remove-Item (Join-Path $Target "t7kb.exe.*.old") -Force -ErrorAction SilentlyContinue
$aside = $null
if (Test-Path $exe) {
    $aside = "$exe.$(Get-Date -Format yyyyMMddHHmmss).old"
    Move-Item $exe $aside
}
try {
    Expand-Archive -Path $tmp -DestinationPath $Target -Force
} catch {
    if ($aside) { Move-Item $aside $exe -Force }
    throw
}
if ($aside) { Remove-Item $aside -Force -ErrorAction SilentlyContinue }
Remove-Item $tmp -Force

# Downloaded beside, then moved: an interrupted download leaves the old zip whole.
Get-Release "t7kb.db.zip" "$dbZip.part"
Move-Item "$dbZip.part" $dbZip -Force

# The zip stays: unpack-db compares the database with it, and unpacks it only
# when they differ, staging it as t7kb.db.new while a running t7kb holds t7kb.db.
# This script comes from main, the binary from the latest release: one older than
# unpack-db would take it for a browse query and wait on stdin, so ask first.
if ((& $exe --help | Out-String) -match 'unpack-db') {
    & $exe unpack-db --db $db
    if ($LASTEXITCODE -ne 0) { throw "unpacking the database failed (t7kb exit $LASTEXITCODE)" }
}

# No exit here: piped into iex, it would close the user's PowerShell.
if (Test-Path "$db.new") {
    Write-Host @"

t7kb updated, but the new database waits as t7kb.db.new: close every agent
session (Claude Code, Codex, ...) and start one to swap it in.  Binary: $exe
"@
} else {
    Write-Host @"

t7kb installed.  Binary: $exe

Register it as an MCP server in your agent (command + args):
    command: $exe
    args:    ["mcp"]

See the README for per-client config (Claude Code, Codex, OpenCode, Copilot, Cursor).
"@
}
