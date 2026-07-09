#!/usr/bin/env pwsh
#
# Packages the TenuVault-TUI PowerShell Gallery module: stages the module files,
# drops in the release binary, stamps the version, validates the manifest, and
# (when an API key is supplied) publishes to the PowerShell Gallery.
#
# Usage:
#   ./scripts/build-psmodule.ps1 -Version v1.4.0 -ExePath path\to\tenuvault.exe
#   ./scripts/build-psmodule.ps1 -Version v1.4.0 -ExePath ... -ApiKey $env:PSGALLERY_API_KEY
#
# Write-Host is intentional: the script returns the staged module path on the
# output stream, so progress messages must stay on the information stream.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '')]
[CmdletBinding()]
param(
    # Release version; a leading 'v' is trimmed (v1.4.0 -> 1.4.0).
    [Parameter(Mandatory)] [string] $Version,

    # Path to the 64-bit Windows tenuvault.exe to bundle.
    [Parameter(Mandatory)] [string] $ExePath,

    # Module source directory (defaults to the checked-in module).
    # Nested two-arg Join-Path: the multi-segment overload needs PS 6+.
    [string] $SourceDir = (Join-Path (Join-Path (Join-Path $PSScriptRoot '..') 'powershell') 'TenuVault-TUI'),

    # Where to stage the packaged module (defaults to a temp dir).
    [string] $StagingDir,

    # PowerShell Gallery API key. When omitted, the module is staged but not published.
    [string] $ApiKey,

    [string] $Repository = 'PSGallery'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Version = $Version.TrimStart('v')

# ModuleVersion must parse as System.Version; prerelease tags (v1.4.0-rc1)
# would need PSData.Prerelease support, which this pipeline does not implement.
# The psgallery workflow job skips prerelease tags; catch direct invocations here.
$parsedVersion = $null
if (-not [Version]::TryParse($Version, [ref] $parsedVersion)) {
    throw "Version '$Version' is not a valid module version. Prerelease tags (e.g. v1.4.0-rc1) are not published to the PowerShell Gallery."
}

if (-not (Test-Path -LiteralPath $ExePath)) {
    throw "Binary not found: $ExePath"
}

if (-not $StagingDir) {
    $StagingDir = Join-Path ([System.IO.Path]::GetTempPath()) "tenuvault-psmodule-$Version"
}

$moduleDir = Join-Path $StagingDir 'TenuVault-TUI'
if (Test-Path -LiteralPath $moduleDir) {
    Remove-Item -LiteralPath $moduleDir -Recurse -Force
}
New-Item -ItemType Directory -Path (Join-Path $moduleDir 'bin') -Force | Out-Null

Copy-Item -LiteralPath (Join-Path $SourceDir 'TenuVault-TUI.psd1') -Destination $moduleDir
Copy-Item -LiteralPath (Join-Path $SourceDir 'TenuVault-TUI.psm1') -Destination $moduleDir
Copy-Item -LiteralPath (Join-Path $SourceDir 'README.md')          -Destination $moduleDir
Copy-Item -LiteralPath $ExePath -Destination (Join-Path (Join-Path $moduleDir 'bin') 'tenuvault.exe')

# Stamp the version. Targeted replace keeps the hand-authored manifest (tags,
# PrivateData) intact, unlike Update-ModuleManifest which can rewrite it.
$manifestPath = Join-Path $moduleDir 'TenuVault-TUI.psd1'
$manifest = Get-Content -LiteralPath $manifestPath -Raw
$stamped = $manifest -replace "ModuleVersion\s*=\s*'[^']*'", "ModuleVersion        = '$Version'"
if ($stamped -eq $manifest) {
    throw "Failed to stamp ModuleVersion in $manifestPath"
}
Set-Content -LiteralPath $manifestPath -Value $stamped -NoNewline

Write-Host "Staged TenuVault-TUI $Version at $moduleDir"

# Validate before publishing.
$null = Test-ModuleManifest -Path $manifestPath
Write-Host "Manifest validated."

if ($ApiKey) {
    Write-Host "Publishing TenuVault-TUI $Version to $Repository ..."
    Publish-Module -Path $moduleDir -NuGetApiKey $ApiKey -Repository $Repository -Verbose
    Write-Host "Published TenuVault-TUI $Version."
}
else {
    Write-Host "No -ApiKey supplied; skipping publish. Inspect the staged module at $moduleDir"
}

return $moduleDir
