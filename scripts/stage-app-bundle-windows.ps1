[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$StageRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not [Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
        [Runtime.InteropServices.OSPlatform]::Windows
    )) {
    throw 'app bundle staging: requires a native Windows host'
}
if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne
    [Runtime.InteropServices.Architecture]::X64) {
    throw 'app bundle staging: requires a native Windows x64 host'
}
if (-not [IO.Path]::IsPathFullyQualified($StageRoot)) {
    throw 'app bundle staging: stage directory must be absolute'
}

$StageRoot = [IO.Path]::GetFullPath($StageRoot)
if (Test-Path -LiteralPath $StageRoot) {
    $stageItem = Get-Item -LiteralPath $StageRoot -Force
    if (-not $stageItem.PSIsContainer) {
        throw 'app bundle staging: stage path exists and is not a directory'
    }
    if (($stageItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'app bundle staging: stage directory must not be a reparse point'
    }
    if (@(Get-ChildItem -LiteralPath $StageRoot -Force).Count -ne 0) {
        throw 'app bundle staging: stage directory must be empty'
    }
} else {
    New-Item -ItemType Directory -Path $StageRoot | Out-Null
}

$repositoryRoot = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$manifestPath = Join-Path $repositoryRoot 'packaging/runtime/manifest.json'
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$target = @($manifest.targets | Where-Object { $_.id -eq 'windows-amd64' })
if ($target.Count -ne 1) {
    throw 'app bundle staging: windows-amd64 runtime target is missing or duplicated'
}
$target = $target[0]
if ($target.os -ne 'windows' -or $target.arch -ne 'amd64') {
    throw 'app bundle staging: windows-amd64 runtime target is inconsistent'
}

$stagePrefix = $StageRoot.TrimEnd([IO.Path]::DirectorySeparatorChar) +
    [IO.Path]::DirectorySeparatorChar
function Resolve-StagePath {
    param([Parameter(Mandatory = $true)][string]$RelativePath)

    if ([IO.Path]::IsPathRooted($RelativePath)) {
        throw "app bundle staging: absolute payload path $RelativePath"
    }
    $nativeRelative = $RelativePath.Replace('/', [IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath((Join-Path $StageRoot $nativeRelative))
    if (-not $resolved.StartsWith($stagePrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "app bundle staging: payload path escapes stage root: $RelativePath"
    }
    return $resolved
}

function Assert-Sha256 {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Expected,
        [Parameter(Mandatory = $true)][string]$Label
    )

    if ($Expected -notmatch '^[0-9a-f]{64}$') {
        throw "app bundle staging: invalid checksum for $Label"
    }
    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Expected) {
        throw "app bundle staging: checksum mismatch for $Label"
    }
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory = $true)][string]$Output,
        [Parameter(Mandatory = $true)][string]$Package,
        [Parameter(Mandatory = $true)][bool]$CgoEnabled,
        [switch]$WindowsGui
    )

    $oldGoos = $env:GOOS
    $oldGoarch = $env:GOARCH
    $oldCgo = $env:CGO_ENABLED
    try {
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = if ($CgoEnabled) { '1' } else { '0' }
        $arguments = @('build', '-trimpath')
        if ($WindowsGui) {
            $arguments += @('-ldflags', '-H=windowsgui')
        }
        $arguments += @('-o', $Output, $Package)
        & go @arguments
        if ($LASTEXITCODE -ne 0) {
            throw "app bundle staging: go build failed for $Package"
        }
    } finally {
        $env:GOOS = $oldGoos
        $env:GOARCH = $oldGoarch
        $env:CGO_ENABLED = $oldCgo
    }
}

function Assert-PeX64 {
    param([Parameter(Mandatory = $true)][string]$Path)

    $stream = [IO.File]::OpenRead($Path)
    $reader = [IO.BinaryReader]::new($stream)
    try {
        if ($stream.Length -lt 64 -or $reader.ReadUInt16() -ne 0x5A4D) {
            throw "app bundle staging: $(Split-Path -Leaf $Path) is not a PE executable"
        }
        $stream.Position = 0x3C
        $peOffset = $reader.ReadInt32()
        if ($peOffset -lt 0 -or $peOffset + 6 -gt $stream.Length) {
            throw "app bundle staging: $(Split-Path -Leaf $Path) has an invalid PE header"
        }
        $stream.Position = $peOffset
        if ($reader.ReadUInt32() -ne 0x00004550 -or $reader.ReadUInt16() -ne 0x8664) {
            throw "app bundle staging: $(Split-Path -Leaf $Path) is not PE x64"
        }
    } finally {
        $reader.Dispose()
        $stream.Dispose()
    }
}

$downloadRoot = Join-Path $StageRoot '.downloads'
New-Item -ItemType Directory -Path $downloadRoot | Out-Null
foreach ($artifact in @($target.artifacts)) {
    if ($artifact.runtime -notin @('tachyon', 'fylo')) {
        throw "app bundle staging: unknown runtime $($artifact.runtime)"
    }
    if ($artifact.downloadUrl -notmatch '^https://github\.com/d31ma/(Tachyon|FYLO)/releases/download/v26\.33\.02/') {
        throw "app bundle staging: runtime URL is not version-pinned"
    }
    $download = Join-Path $downloadRoot $artifact.asset
    Invoke-WebRequest -Uri $artifact.downloadUrl -OutFile $download
    Assert-Sha256 -Path $download -Expected $artifact.sha256 -Label $artifact.asset

    $destination = Resolve-StagePath -RelativePath $artifact.installRelativePath
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
    Copy-Item -LiteralPath $download -Destination $destination
    Write-Host "app bundle staging: verified $($artifact.runtime) $($artifact.asset)"
}

$appRoot = Resolve-StagePath -RelativePath 'Sluice'
$libexec = Join-Path $appRoot 'libexec'
$resources = Join-Path $appRoot 'resources'
$routeDirectory = Join-Path $resources 'tachyon/server/routes/v1/health'
$sluiceLicenseDirectory = Join-Path $appRoot 'licenses/Sluice'
New-Item -ItemType Directory -Force -Path $libexec, $routeDirectory, $sluiceLicenseDirectory | Out-Null

Push-Location $repositoryRoot
try {
    Invoke-GoBuild -Output (Join-Path $appRoot 'sluice-desktop.exe') -Package './cmd/sluice-desktop' -CgoEnabled $true -WindowsGui
    Invoke-GoBuild -Output (Join-Path $libexec 'sluice-agent.exe') -Package './cmd/sluice-agent' -CgoEnabled $false
    Invoke-GoBuild -Output (Join-Path $libexec 'sluice-yon.exe') -Package './internal/transport/yon/cmd/sluice-yon' -CgoEnabled $false
} finally {
    Pop-Location
}

Copy-Item -LiteralPath (Join-Path $repositoryRoot 'packaging/windows/app-manifest.json') -Destination (Join-Path $appRoot 'app-manifest.json')
Copy-Item -LiteralPath (Join-Path $repositoryRoot 'packaging/tachyon/server/routes/v1/health/yon.sluice') -Destination (Join-Path $routeDirectory 'yon.sluice')
Copy-Item -LiteralPath (Join-Path $repositoryRoot 'LICENSE') -Destination (Join-Path $sluiceLicenseDirectory 'LICENSE')

foreach ($source in @($manifest.sources.PSObject.Properties)) {
    $sourceDirectory = Join-Path $appRoot "licenses/$($source.Value.name)"
    New-Item -ItemType Directory -Force -Path $sourceDirectory | Out-Null
    foreach ($notice in @($source.Value.notices)) {
        $sourcePath = [IO.Path]::GetFullPath((Join-Path $repositoryRoot $notice.sourcePath))
        $repositoryPrefix = $repositoryRoot.TrimEnd([IO.Path]::DirectorySeparatorChar) +
            [IO.Path]::DirectorySeparatorChar
        if (-not $sourcePath.StartsWith($repositoryPrefix, [StringComparison]::OrdinalIgnoreCase)) {
            throw 'app bundle staging: notice path escapes repository'
        }
        Assert-Sha256 -Path $sourcePath -Expected $notice.sha256 -Label $notice.sourcePath
        Copy-Item -LiteralPath $sourcePath -Destination (Join-Path $sourceDirectory (Split-Path -Leaf $sourcePath))
    }
}

$programs = @(
    (Join-Path $appRoot 'sluice-desktop.exe'),
    (Join-Path $libexec 'sluice-agent.exe'),
    (Join-Path $libexec 'sluice-yon.exe'),
    (Join-Path $appRoot 'runtime/bin/ty.exe'),
    (Join-Path $appRoot 'runtime/bin/fylo.exe')
)
foreach ($program in $programs) {
    Assert-PeX64 -Path $program
}

$applicationManifest = Get-Content -LiteralPath (Join-Path $appRoot 'app-manifest.json') -Raw | ConvertFrom-Json
if ($applicationManifest.schemaVersion -ne 1 -or
    $applicationManifest.applicationId -ne 'ma.del.sluice.desktop' -or
    $applicationManifest.entrypoint -ne 'sluice-desktop.exe' -or
    $applicationManifest.architecture -ne 'amd64' -or
    $applicationManifest.runtimeVersion -ne $manifest.releaseVersion) {
    throw 'app bundle staging: Windows application manifest mismatch'
}

$tyPath = Join-Path $appRoot 'runtime/bin/ty.exe'
$fyloPath = Join-Path $appRoot 'runtime/bin/fylo.exe'
$tyVersion = (& $tyPath --version | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or $tyVersion -ne $manifest.releaseVersion) {
    throw 'app bundle staging: Tachyon version mismatch'
}
$fyloVersion = (& $fyloPath version --output json | Out-String) | ConvertFrom-Json
if ($LASTEXITCODE -ne 0 -or
    $fyloVersion.runtimeVersion -ne $manifest.releaseVersion -or
    $fyloVersion.commit -ne $manifest.sources.fylo.sourceCommit -or
    $fyloVersion.buildTarget -ne 'windows-x64') {
    throw 'app bundle staging: Fylo release identity mismatch'
}

Remove-Item -LiteralPath $downloadRoot -Recurse -Force
Write-Host "app bundle staging: unsigned Windows amd64 portable package passed in $StageRoot"
