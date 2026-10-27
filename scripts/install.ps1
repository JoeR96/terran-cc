# Installs tcc and tccd with `go install`, builds the windowless tccw and the
# tcc-desktop app, adds Start Menu and sign-in shortcuts, and copies the
# terran-cc skill into ~/.claude/skills.
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

    # The desktop app: build the Svelte frontend, then embed it in the Go shell.
    Push-Location desktop/frontend
    try {
        npm install --no-fund --no-audit
        if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw 'building the frontend failed' }
    } finally {
        Pop-Location
    }
    Get-Process tcc-desktop -ErrorAction SilentlyContinue | Stop-Process -Force
    go -C desktop build -ldflags -H=windowsgui -o (Join-Path $bin 'tcc-desktop.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'building tcc-desktop failed' }
} finally {
    Pop-Location
}
Write-Host "Installed tcc, tccw, tccd and tcc-desktop to $bin"

# Start Menu shortcut, and start the app when you sign in.
$shell = New-Object -ComObject WScript.Shell
$programs = [Environment]::GetFolderPath('Programs')
$startup = [Environment]::GetFolderPath('Startup')
foreach ($dir in @($programs, $startup)) {
    $lnk = $shell.CreateShortcut((Join-Path $dir 'Terran CC.lnk'))
    $lnk.TargetPath = Join-Path $bin 'tcc-desktop.exe'
    $lnk.WorkingDirectory = $bin
    $lnk.Description = 'Terran CC command centre'
    $lnk.IconLocation = (Join-Path $bin 'tcc-desktop.exe') + ',0'
    $lnk.Save()
}
Write-Host 'Added Terran CC to the Start Menu and to sign-in startup'
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
