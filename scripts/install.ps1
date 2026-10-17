# Installs tcc and tccd with `go install`, builds the windowless tccw, and copies the terran-cc skill into
# ~/.claude/skills.
#
# The rewired dispatch-tasks skill reads tickets from Terran CC instead of
# Obsidian. It only replaces your current one when you pass -DispatchTasks,
# and the old one is backed up first.
param([switch]$DispatchTasks)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

$bin = Join-Path (go env GOPATH) 'bin'

# Stop the running service so the new tccd takes over. (tcc also replaces an
# older tccd by itself, but stopping first keeps the swap clean.)
if (Test-Path (Join-Path $bin 'tcc.exe')) {
    & (Join-Path $bin 'tcc.exe') service stop 2>&1 | Out-Null
}

Push-Location $repo
try {
    go install ./cmd/tcc ./cmd/tccd
    if ($LASTEXITCODE -ne 0) { throw 'go install failed' }
    # tccw is tcc built as a Windows GUI program: no console window flashes
    # when a Stream Deck button runs it.
    go build -ldflags -H=windowsgui -o (Join-Path $bin 'tccw.exe') ./cmd/tcc
    if ($LASTEXITCODE -ne 0) { throw 'building tccw failed' }
} finally {
    Pop-Location
}
Write-Host "Installed tcc, tccw and tccd to $bin"
if (-not (($env:PATH -split ';') -contains $bin)) {
    Write-Warning "$bin is not on your PATH. Add it so 'tcc' works from any terminal."
}

$skills = Join-Path $HOME '.claude\skills'
New-Item -ItemType Directory -Force $skills | Out-Null

$names = @('terran-cc')
if ($DispatchTasks) { $names += 'dispatch-tasks' }
foreach ($name in $names) {
    $dest = Join-Path $skills $name
    if ($name -eq 'dispatch-tasks' -and (Test-Path $dest)) {
        $backup = "$dest.bak-$(Get-Date -Format yyyyMMdd-HHmmss)"
        Move-Item $dest $backup
        Write-Host "Backed up your old dispatch-tasks skill to $backup"
    }
    New-Item -ItemType Directory -Force $dest | Out-Null
    Copy-Item (Join-Path $repo "skills\$name\*") $dest -Recurse -Force
    Write-Host "Installed the $name skill to $dest"
}
if (-not $DispatchTasks) {
    Write-Host 'Left your dispatch-tasks skill alone. Run with -DispatchTasks once your tickets are in Terran CC.'
}
