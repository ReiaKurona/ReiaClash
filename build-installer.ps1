[CmdletBinding()]
param(
    [string] $PayloadDirectory,
    [string] $OutputDirectory,
    [string] $SevenZip,
    [string] $InnoCompiler
)

$ErrorActionPreference = 'Stop'

function Find-Executable {
    param([string] $Path, [string] $Name, [string[]] $Candidates, [string] $Parameter)

    if ($Path) {
        return (Get-Item -LiteralPath $Path).FullName
    }

    $command = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($command) {
        return $command.Source
    }

    foreach ($candidate in $Candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return (Get-Item -LiteralPath $candidate).FullName
        }
    }

    throw "Could not find $Name. Install the tool or pass its path with -$Parameter."
}

if (!$PayloadDirectory) {
    $PayloadDirectory = Join-Path $PSScriptRoot 'installer'
    $legacyDirectory = Join-Path $PSScriptRoot 'installler'
    if (!(Test-Path -LiteralPath $PayloadDirectory) -and (Test-Path -LiteralPath $legacyDirectory -PathType Container)) {
        $PayloadDirectory = $legacyDirectory
        Write-Host "Using existing payload directory: $PayloadDirectory"
    }
}

$payload = Get-Item -LiteralPath $PayloadDirectory
if (!$payload.PSIsContainer) {
    throw "The payload path must be a directory: $PayloadDirectory"
}

if (!$OutputDirectory) {
    $OutputDirectory = Join-Path $PSScriptRoot '.build\installer'
}
$outputPath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDirectory)
$payloadPrefix = $payload.FullName.TrimEnd('\') + '\'
if (($outputPath.TrimEnd('\') + '\').StartsWith($payloadPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'The output directory must be outside the payload directory.'
}

$requiredFiles = @(
    'reia.exe', 'reia-agent.exe', 'reia-tray.exe', 'reia-service.exe', 'mihomo.exe',
    'test-webui.html', 'data\geoip.metadb', 'data\GeoSite.dat'
)
foreach ($relativePath in $requiredFiles) {
    $requiredFile = Get-Item -LiteralPath (Join-Path $payload.FullName $relativePath) -ErrorAction SilentlyContinue
    if (!$requiredFile -or $requiredFile.PSIsContainer -or !$requiredFile.Length) {
        throw "Missing or empty installer payload file: $relativePath"
    }
}

$entries = @(Get-ChildItem -LiteralPath $payload.FullName -Recurse -Force)
if ($entries | Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint } | Select-Object -First 1) {
    throw 'Replace payload symlinks and junctions with regular files and directories before packaging.'
}
$payloadBytes = [long] ($entries | Where-Object { !$_.PSIsContainer } | Measure-Object -Property Length -Sum).Sum

$sevenZipPath = Find-Executable -Path $SevenZip -Name '7z.exe' -Parameter 'SevenZip' -Candidates @(
    "$env:ProgramW6432\7-Zip\7z.exe",
    "$env:ProgramFiles\7-Zip\7z.exe",
    "${env:ProgramFiles(x86)}\7-Zip\7z.exe"
)
$innoPath = Find-Executable -Path $InnoCompiler -Name 'ISCC.exe' -Parameter 'InnoCompiler' -Candidates @(
    "$env:ProgramW6432\Inno Setup 7\ISCC.exe",
    "$env:ProgramFiles\Inno Setup 7\ISCC.exe",
    "${env:ProgramFiles(x86)}\Inno Setup 7\ISCC.exe",
    "$env:LOCALAPPDATA\Programs\Inno Setup 7\ISCC.exe",
    (Join-Path $PSScriptRoot '.build\installer-tools\inno\ISCC.exe')
)
$output = New-Item -ItemType Directory -Path $outputPath -Force
$work = Join-Path $output.FullName ('.work-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
try {
    $archive = Join-Path $work 'ReiaClash-payload.7z'
    Push-Location -LiteralPath $payload.FullName
    try {
        # Compress the entire tree in one call so files can share the solid dictionary.
        & $sevenZipPath a -t7z -mx=9 '-m0=LZMA2:d=256m' -ms=on -mqs=on -mmt=1 -ssw -sse $archive '.\*'
        if ($LASTEXITCODE -ne 0) { throw "7-Zip compression failed ($LASTEXITCODE)." }
    } finally {
        Pop-Location
    }

    & $sevenZipPath t $archive
    if ($LASTEXITCODE -ne 0) { throw "7-Zip integrity check failed ($LASTEXITCODE)." }

    $definition = Join-Path $PSScriptRoot 'scripts\windows-installer.iss'
    & $innoPath "/DArchivePath=$archive" "/DPayloadBytes=$payloadBytes" "/O$work" $definition
    if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed ($LASTEXITCODE)." }

    $installer = Join-Path $output.FullName 'ReiaClash-Setup.exe'
    [IO.File]::Copy((Join-Path $work 'ReiaClash-Setup.exe'), $installer, $true)
    Get-Item -LiteralPath $installer | Select-Object FullName, Length
} finally {
    $resolvedWork = (Resolve-Path -LiteralPath $work).ProviderPath
    if ((Split-Path -Parent $resolvedWork) -eq $output.FullName -and (Split-Path -Leaf $resolvedWork) -match '^\.work-[a-f0-9]{32}$') {
        Remove-Item -LiteralPath $resolvedWork -Recurse -Force
    }
}
