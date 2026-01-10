. "$PSScriptRoot\tools.ps1"

$ErrorActionPreference = "Stop"

$RepoRoot = Get-RepoRoot
Set-Location $RepoRoot

$VersionInfoFile = Join-Path $RepoRoot "versioninfo.cfg"
git checkout -- "$VersionInfoFile" 2>&1 | Out-Null

$VersionInfo = Get-VersionInfo -RepoRoot $RepoRoot
$Build = Get-GitBuildCount
$VersionTag = Get-VersionTag -VersionInfo $VersionInfo -Build $Build
$GitCommit = Get-GitCommit

Write-Host "Building deej (development)..." -ForegroundColor Cyan
Write-Host "Embedding: gitCommit=$GitCommit, versionTag=$VersionTag, buildType=dev" -ForegroundColor Gray

Invoke-Build -RepoRoot $RepoRoot -BuildType "dev" -OutputFile "deej-dev.exe" -VersionTag $VersionTag -GitCommit $GitCommit

Write-Host "Output: build\deej-dev.exe" -ForegroundColor Green
