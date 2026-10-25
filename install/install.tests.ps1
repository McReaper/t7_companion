# Tests install.ps1 offline: the functions below stand in for curl.exe and
# Invoke-RestMethod (a function wins over a command of the same name), and the
# release they serve is a fake binary bundle and a fake database zip. CI's
# install job runs it on Windows PowerShell, what users run.
#
#   powershell -NoProfile -File install\install.tests.ps1
$ErrorActionPreference = "Stop"
$installer = Join-Path $PSScriptRoot "install.ps1"
$root = Join-Path ([IO.Path]::GetTempPath()) "t7kb-install-tests-$PID"
New-Item -ItemType Directory -Force -Path $root | Out-Null
$savedTemp = $env:TEMP
$env:TEMP = $root # install.ps1 stages the bundle there

# The mocks run inside install.ps1, whose variables would shadow the test's:
# everything they read is in this global.
$global:Release = @{ downloads = @(); digest = $null; apiFails = $false }
New-Item -ItemType Directory -Force -Path "$root\bundle" | Out-Null
Set-Content "$root\bundle\t7kb.exe" "binary"
Compress-Archive -Path "$root\bundle\*" -DestinationPath "$root\t7kb_windows_amd64.zip"
$global:Release.bundle = "$root\t7kb_windows_amd64.zip"
foreach ($version in "old", "new") {
    Set-Content "$root\$version.db.zip" "database $version" -NoNewline
    $global:Release[$version] = "$root\$version.db.zip"
}

function curl.exe {
    $out = $args[[array]::IndexOf($args, "-o") + 1]
    $name = Split-Path -Leaf $args[-1]
    $global:Release.downloads += $name
    $from = if ($name -eq "t7kb.db.zip") { $global:Release.new } else { $global:Release.bundle }
    Copy-Item $from $out -Force
    $global:LASTEXITCODE = 0
}

function Invoke-RestMethod {
    if ($global:Release.apiFails) { throw "API rate limit exceeded" }
    [pscustomobject]@{ assets = @(
        [pscustomobject]@{ name = "t7kb_windows_amd64.zip"; digest = "sha256:0123" },
        [pscustomobject]@{ name = "t7kb.db.zip"; digest = $global:Release.digest }) }
}

function Get-Digest([string]$path) { "sha256:" + (Get-FileHash $path -Algorithm SHA256).Hash.ToLower() }

# What an earlier install leaves: t7kb.exe, the zip of $zip's version, and a
# t7kb.db unpacked from it an hour later, or one an hour older ("older": not
# unpacked from this zip).
function New-Install([string]$name, [string]$zip, [string]$dbAge = "newer") {
    $dir = Join-Path $root $name
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Set-Content "$dir\t7kb.exe" "old binary"
    Copy-Item $global:Release[$zip] "$dir\t7kb.db.zip"
    Set-Content "$dir\t7kb.db" "unpacked"
    $zipTime = (Get-Date).AddHours(-2)
    (Get-Item "$dir\t7kb.db.zip").LastWriteTime = $zipTime
    $hours = 1
    if ($dbAge -eq "older") { $hours = -1 }
    (Get-Item "$dir\t7kb.db").LastWriteTime = $zipTime.AddHours($hours)
    $dir
}

# Runs the installer and returns what it printed.
function Invoke-Install([string]$dir, [switch]$Force) {
    $global:Release.downloads = @()
    & $installer -Target $dir -Force:$Force 6>&1 | Out-String
}

function Assert([bool]$ok, [string]$what) { if (-not $ok) { throw $what } }
function Assert-Downloads([string]$want) {
    $got = $global:Release.downloads -join ","
    Assert ($got -eq $want) "downloaded [$got], want [$want]"
}

$cases = [ordered]@{
    "a fresh install downloads the binary and the database" = {
        $global:Release.digest = Get-Digest $global:Release.new
        $dir = Join-Path $root "fresh"
        $out = Invoke-Install $dir
        Assert-Downloads "t7kb_windows_amd64.zip,t7kb.db.zip"
        Assert ((Get-Content "$dir\t7kb.exe" -Raw).Trim() -eq "binary") "t7kb.exe isn't the release's"
        Assert ((Get-Content "$dir\t7kb.db.zip" -Raw) -eq "database new") "t7kb.db.zip isn't the release's"
        Assert ($out -match "t7kb installed") "printed: $out"
    }
    "an update keeps the database when its zip is the release's" = {
        $global:Release.digest = Get-Digest $global:Release.new
        $dir = New-Install "same" "new"
        $out = Invoke-Install $dir -Force
        Assert-Downloads "t7kb_windows_amd64.zip"
        Assert ((Get-Content "$dir\t7kb.exe" -Raw).Trim() -eq "binary") "t7kb.exe wasn't updated"
        Assert (Test-Path "$dir\t7kb.db") "t7kb.db was deleted"
        Assert ($out -match "database didn't change") "printed: $out"
    }
    "an update replaces a database whose zip isn't the release's" = {
        $global:Release.digest = Get-Digest $global:Release.new
        $dir = New-Install "changed" "old"
        $out = Invoke-Install $dir -Force
        Assert-Downloads "t7kb_windows_amd64.zip,t7kb.db.zip"
        Assert ((Get-Content "$dir\t7kb.db.zip" -Raw) -eq "database new") "t7kb.db.zip isn't the release's"
        Assert (-not (Test-Path "$dir\t7kb.db")) "the old t7kb.db stayed"
        Assert ($out -match "unpacks the new database") "printed: $out"
    }
    "an update downloads the database when the API fails" = {
        $global:Release.apiFails = $true
        try {
            $dir = New-Install "offline" "new"
            Invoke-Install $dir -Force | Out-Null
        } finally { $global:Release.apiFails = $false }
        Assert-Downloads "t7kb_windows_amd64.zip,t7kb.db.zip"
        Assert (-not (Test-Path "$dir\t7kb.db")) "the old t7kb.db stayed"
    }
    "an update downloads the database when the release lists no digest" = {
        $global:Release.digest = $null
        $dir = New-Install "nodigest" "new"
        Invoke-Install $dir -Force | Out-Null
        Assert-Downloads "t7kb_windows_amd64.zip,t7kb.db.zip"
        Assert (-not (Test-Path "$dir\t7kb.db")) "the old t7kb.db stayed"
    }
    "an update deletes a t7kb.db older than its zip, which is kept" = {
        $global:Release.digest = Get-Digest $global:Release.new
        $dir = New-Install "stale" "new" "older"
        $out = Invoke-Install $dir -Force
        Assert-Downloads "t7kb_windows_amd64.zip"
        Assert (-not (Test-Path "$dir\t7kb.db")) "the stale t7kb.db stayed"
        Assert ($out -match "unpacks the new database") "printed: $out"
    }
    "without -Force, an existing install downloads nothing" = {
        $global:Release.digest = Get-Digest $global:Release.new
        $dir = New-Install "inplace" "old"
        Invoke-Install $dir | Out-Null
        Assert-Downloads ""
        Assert (Test-Path "$dir\t7kb.db") "t7kb.db was deleted"
    }
}

$failed = 0
try {
    foreach ($name in $cases.Keys) {
        try {
            & $cases[$name]
            Write-Host "ok   $name"
        } catch {
            $failed++
            Write-Host "FAIL $name`n     $_"
        }
    }
} finally {
    $env:TEMP = $savedTemp
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Variable Release -Scope Global
}
if ($failed) { Write-Host "$failed of $($cases.Count) failed"; exit 1 }
Write-Host "all $($cases.Count) passed"
