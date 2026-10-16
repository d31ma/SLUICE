[CmdletBinding()]
param(
    [string]$StagedRoot = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not [Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
        [Runtime.InteropServices.OSPlatform]::Windows
    )) {
    throw 'app bundle test: requires a native Windows host'
}

$ownsStage = [string]::IsNullOrWhiteSpace($StagedRoot)
if ($ownsStage) {
    $stageLeaf = 'sluice-app-windows-' + [Guid]::NewGuid().ToString('N')
    $stageRoot = Join-Path ([IO.Path]::GetTempPath()) $stageLeaf
} else {
    if (-not [IO.Path]::IsPathFullyQualified($StagedRoot)) {
        throw 'app bundle test: staged root must be absolute'
    }
    $stageRoot = [IO.Path]::GetFullPath($StagedRoot)
    $stageLeaf = Split-Path -Leaf $stageRoot
}
$repositoryRoot = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))

function Start-SluiceSmoke {
    param(
        [Parameter(Mandatory = $true)][string]$Desktop,
        [Parameter(Mandatory = $true)][string]$StateRoot
    )

    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $Desktop
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    foreach ($argument in @(
            '--background',
            '--native-smoke',
            '--qualification-state-root',
            $StateRoot
        )) {
        $startInfo.ArgumentList.Add($argument)
    }

    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    if (-not $process.Start()) {
        throw 'app bundle test: could not start packaged desktop'
    }

    try {
        $sessionFile = Join-Path $StateRoot 'run/agent-session.json'
        $deadline = [DateTime]::UtcNow.AddSeconds(20)
        while (-not (Test-Path -LiteralPath $sessionFile) -and
            -not $process.HasExited -and
            [DateTime]::UtcNow -lt $deadline) {
            Start-Sleep -Milliseconds 20
        }
        if (-not (Test-Path -LiteralPath $sessionFile)) {
            $process.WaitForExit(5000) | Out-Null
            if (-not $process.HasExited) {
                $process.Kill($true)
                $process.WaitForExit()
            }
            throw "app bundle test: agent descriptor did not appear; desktop exit $($process.ExitCode)"
        }

        $descriptor = Get-Content -LiteralPath $sessionFile -Raw | ConvertFrom-Json
        if ($descriptor.network -ne 'npipe' -or
            $descriptor.address -notmatch '^\\\\\.\\pipe\\sluice-agent-[0-9a-f]{32}$') {
            throw 'app bundle test: packaged agent did not publish a private named-pipe descriptor'
        }

        if (-not $process.WaitForExit(30000)) {
            $process.Kill($true)
            $process.WaitForExit()
            throw 'app bundle test: packaged desktop did not finish native smoke in time'
        }
        if ($process.ExitCode -ne 0) {
            throw "app bundle test: packaged desktop exited $($process.ExitCode)"
        }
        if (Test-Path -LiteralPath $sessionFile) {
            throw 'app bundle test: agent descriptor survived desktop shutdown'
        }
    } finally {
        if (-not $process.HasExited) {
            $process.Kill($true)
            $process.WaitForExit()
        }
        $process.Dispose()
    }
}

try {
    if ($ownsStage) {
        & (Join-Path $PSScriptRoot 'stage-app-bundle-windows.ps1') -StageRoot $stageRoot
    }
    $appRoot = Join-Path $stageRoot 'Sluice'
    $desktop = Join-Path $appRoot 'sluice-desktop.exe'
    $stateRoot = Join-Path $stageRoot 'qualification-state'

    Start-SluiceSmoke -Desktop $desktop -StateRoot $stateRoot
    $firstToken = (Get-FileHash -LiteralPath (Join-Path $stateRoot 'run/api-token') -Algorithm SHA256).Hash
    Start-SluiceSmoke -Desktop $desktop -StateRoot $stateRoot
    $secondToken = (Get-FileHash -LiteralPath (Join-Path $stateRoot 'run/api-token') -Algorithm SHA256).Hash
    if ($firstToken -ne $secondToken) {
        throw 'app bundle test: packaged restart changed the local API token'
    }

    $survivors = @(Get-CimInstance Win32_Process | Where-Object {
            $_.ExecutablePath -and
            $_.ExecutablePath.StartsWith($appRoot, [StringComparison]::OrdinalIgnoreCase)
        })
    if ($survivors.Count -ne 0) {
        throw 'app bundle test: a packaged child survived desktop shutdown'
    }

    Write-Host 'app bundle test: Windows package health, named-pipe IPC, restart, and clean shutdown passed'
} finally {
    if ($ownsStage) {
        $expectedPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar) +
            [IO.Path]::DirectorySeparatorChar
        $resolvedStage = [IO.Path]::GetFullPath($stageRoot)
        if ($resolvedStage.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase) -and
            (Split-Path -Leaf $resolvedStage) -eq $stageLeaf) {
            Remove-Item -LiteralPath $resolvedStage -Recurse -Force -ErrorAction SilentlyContinue
        } else {
            Write-Warning "refusing to remove unexpected scratch path: $resolvedStage"
        }
    }
}
