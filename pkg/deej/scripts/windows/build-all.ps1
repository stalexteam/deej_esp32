. "$PSScriptRoot\tools.ps1"

$ErrorActionPreference = "Stop"

Write-Host "Building deej (all)..." -ForegroundColor Cyan
Write-Host ""

& (Join-Path $PSScriptRoot "build-dev.ps1")
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build development version"
    exit 1
}

Write-Host ""

& (Join-Path $PSScriptRoot "build-release.ps1")
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build release version"
    exit 1
}
