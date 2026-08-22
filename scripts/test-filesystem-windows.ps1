[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not $IsWindows) {
    throw 'filesystem Windows qualification requires a native Windows host'
}
if ((go env GOOS) -ne 'windows' -or (go env GOARCH) -ne 'amd64') {
    throw 'filesystem Windows qualification requires Go windows/amd64'
}
if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) {
    throw 'filesystem Windows qualification requires RUNNER_TEMP for isolated storage'
}

$scratchPrefix = [IO.Path]::GetFullPath((Join-Path $env:RUNNER_TEMP 'sluice-filesystem-windows-'))
$scratchRoot = $scratchPrefix + [Guid]::NewGuid().ToString('N')
$vhdPath = Join-Path $scratchRoot 'enospc.vhdx'
$usedDriveLetters = @(Get-Volume | Where-Object DriveLetter | ForEach-Object { [string] $_.DriveLetter })
$driveLetter = @('Z', 'Y', 'X', 'W', 'V', 'U', 'T', 'S', 'R') |
    Where-Object { $usedDriveLetters -notcontains $_ } |
    Select-Object -First 1
if ([string]::IsNullOrWhiteSpace($driveLetter)) {
    throw 'filesystem Windows qualification could not find an unused drive letter'
}
$volumeRoot = [string]::Concat($driveLetter, ':\')
$junctionFixture = Join-Path $scratchRoot 'junction-fixture'
$junctionRoot = Join-Path $junctionFixture 'root'
$junctionOutside = Join-Path $junctionFixture 'outside'
$junctionPath = Join-Path $junctionRoot 'linked'
$createScript = Join-Path $scratchRoot 'create-vhd.txt'
$detachScript = Join-Path $scratchRoot 'detach-vhd.txt'
$attached = $false

function Invoke-QualificationDiskPart {
    param(
        [Parameter(Mandatory)]
        [string] $ScriptPath,
        [Parameter(Mandatory)]
        [string[]] $Commands
    )

    $Commands | Set-Content -LiteralPath $ScriptPath -Encoding ascii
    $output = & diskpart.exe /s $ScriptPath 2>&1
    $exitCode = $LASTEXITCODE
    $output | ForEach-Object { Write-Host $_ }
    if ($exitCode -ne 0 -or ($output -join "`n") -match 'DiskPart has encountered an error') {
        throw "diskpart qualification command failed with exit code $exitCode"
    }
}

try {
    New-Item -ItemType Directory -Path $junctionRoot, $junctionOutside | Out-Null
    New-Item -ItemType Junction -Path $junctionPath -Target $junctionOutside | Out-Null
    if (-not ((Get-Item -LiteralPath $junctionPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw 'junction fixture is not a reparse point'
    }

    Invoke-QualificationDiskPart -ScriptPath $createScript -Commands @(
        "create vdisk file=`"$vhdPath`" maximum=64 type=expandable",
        "select vdisk file=`"$vhdPath`"",
        'attach vdisk',
        'create partition primary',
        'format fs=ntfs label=SLUICE_ENOSPC quick',
        "assign letter=$driveLetter",
        'exit'
    )
    $attached = $true

    $volume = Get-Volume -DriveLetter $driveLetter
    if ($volume.FileSystem -ne 'NTFS') {
        throw "filesystem Windows qualification requires NTFS, got $($volume.FileSystem)"
    }
    Write-Host "Windows qualification OS: $([Environment]::OSVersion.VersionString)"
    Write-Host "Windows qualification architecture: $env:PROCESSOR_ARCHITECTURE"
    Write-Host "Windows qualification filesystem: $($volume.FileSystem)"
    Write-Host "Windows qualification Go: $(go version)"

    $env:SLUICE_FS_REQUIRE_NATIVE_LINKS = '1'
    $env:SLUICE_FS_REQUIRE_WINDOWS_NATIVE = '1'
    $env:SLUICE_FS_WINDOWS_JUNCTION_ROOT = $junctionRoot
    $env:SLUICE_FS_WINDOWS_ENOSPC_ROOT = $volumeRoot
    go test -race -count=1 ./internal/filesystem
    if ($LASTEXITCODE -ne 0) {
        throw "Windows filesystem qualification tests failed with exit code $LASTEXITCODE"
    }

    Write-Host 'filesystem Windows qualification: NTFS reparse, junction, ACL, disk-full, pre-rename recovery, and race suites passed'
}
finally {
    Remove-Item Env:SLUICE_FS_REQUIRE_NATIVE_LINKS -ErrorAction SilentlyContinue
    Remove-Item Env:SLUICE_FS_REQUIRE_WINDOWS_NATIVE -ErrorAction SilentlyContinue
    Remove-Item Env:SLUICE_FS_WINDOWS_JUNCTION_ROOT -ErrorAction SilentlyContinue
    Remove-Item Env:SLUICE_FS_WINDOWS_ENOSPC_ROOT -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $junctionPath) {
        Remove-Item -LiteralPath $junctionPath -Force -ErrorAction Continue
    }
    if ($attached) {
        @(
            "select vdisk file=`"$vhdPath`"",
            'detach vdisk',
            'exit'
        ) | Set-Content -LiteralPath $detachScript -Encoding ascii
        & diskpart.exe /s $detachScript | ForEach-Object { Write-Host $_ }
    }
    $resolvedScratch = [IO.Path]::GetFullPath($scratchRoot)
    if (-not $resolvedScratch.StartsWith($scratchPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "refusing to remove unexpected scratch path: $resolvedScratch"
    }
    if (Test-Path -LiteralPath $resolvedScratch) {
        Remove-Item -LiteralPath $resolvedScratch -Recurse -Force
    }
}
