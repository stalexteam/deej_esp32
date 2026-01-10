param(
    [switch]$SkipBuild = $false
)

. "$PSScriptRoot\tools.ps1"

$ErrorActionPreference = "Stop"

$RepoRoot = Get-RepoRoot
$VersionInfoFile = Join-Path $RepoRoot "versioninfo.cfg"

Write-Host "Preparing release..." -ForegroundColor Cyan
Write-Host ""

Set-Location $RepoRoot

Write-Host "Restoring versioninfo.cfg from git..." -ForegroundColor Yellow
try {
    git checkout -- "$VersionInfoFile" 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "versioninfo.cfg might not exist in git yet"
    }
}
catch {
    Write-Warning "Could not restore versioninfo.cfg from git: $_"
}

Write-Host "Reading version from versioninfo.cfg..." -ForegroundColor Yellow
$VersionInfo = Get-VersionInfo -RepoRoot $RepoRoot
$Major = $VersionInfo.Major
$CurrentMinor = $VersionInfo.Minor

Write-Host "Getting build number from git..." -ForegroundColor Yellow
$Build = Get-GitBuildCount
if ($Build -eq 0) {
    Write-Error "Failed to get build number from git"
    exit 1
}
Write-Host "Build number: $Build" -ForegroundColor Green

$GitCommit = Get-GitCommit

if (-not $SkipBuild) {
    Write-Host ""
    Write-Host "Building with current version..." -ForegroundColor Yellow
    
    $CurrentVersionInfo = @{
        Major = $Major
        Minor = $CurrentMinor
        Build = $Build
    }
    $CurrentVersionTag = Get-VersionTag -VersionInfo $CurrentVersionInfo -Build $Build
    Write-Host "Building with version: $CurrentVersionTag" -ForegroundColor Gray
    
    Write-Host "Building development version..." -ForegroundColor Yellow
    Write-Host "Embedding: gitCommit=$GitCommit, versionTag=$CurrentVersionTag, buildType=dev" -ForegroundColor Gray
    
    $buildResult = Invoke-Build -RepoRoot $RepoRoot -BuildType "dev" -OutputFile "deej-dev.exe" -VersionTag $CurrentVersionTag -GitCommit $GitCommit -NoExit
    
    if (-not $buildResult) {
        Write-Host ""
        Write-Host "Build failed, cleaning up..." -ForegroundColor Red
        git checkout -- "$VersionInfoFile" 2>&1 | Out-Null
        Write-Error "Failed to build development version"
        exit 1
    }
    
    Write-Host ""
    Write-Host "Building release version..." -ForegroundColor Yellow
    Write-Host "Embedding: gitCommit=$GitCommit, versionTag=$CurrentVersionTag, buildType=release" -ForegroundColor Gray
    
    $buildResult = Invoke-Build -RepoRoot $RepoRoot -BuildType "release" -OutputFile "deej-release.exe" -VersionTag $CurrentVersionTag -GitCommit $GitCommit -NoExit
    
    if (-not $buildResult) {
        Write-Host ""
        Write-Host "Build failed, cleaning up..." -ForegroundColor Red
        git checkout -- "$VersionInfoFile" 2>&1 | Out-Null
        Write-Error "Failed to build release version"
        exit 1
    }
    
    Write-Host ""
    Write-Host "Build successful!" -ForegroundColor Green
    
    # Use current version for tag (the version we built with)
    $VersionTag = $CurrentVersionTag
}
else {
    Write-Host ""
    Write-Host "Skipping build..." -ForegroundColor Yellow
    
    # If skipping build, use current version from versioninfo
    $CurrentVersionInfo = @{
        Major = $Major
        Minor = $CurrentMinor
        Build = $Build
    }
    $VersionTag = Get-VersionTag -VersionInfo $CurrentVersionInfo -Build $Build
}

Write-Host ""
Write-Host "Version tag: $VersionTag" -ForegroundColor Cyan
Write-Host ""

$ExistingTag = git tag -l "$VersionTag"
if ($ExistingTag -and ($ExistingTag -ne "")) {
    Write-Host "Tag $VersionTag exists, deleting..." -ForegroundColor Yellow
    git tag --delete "$VersionTag" 2>&1 | Out-Null
}

Write-Host "Creating git tag: $VersionTag" -ForegroundColor Yellow
git tag "$VersionTag"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to create git tag"
    exit 1
}

$ReleaseDir = Join-Path $RepoRoot "releases\$VersionTag"
Write-Host ""
Write-Host "Creating release directory: $ReleaseDir" -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $ReleaseDir | Out-Null

Write-Host "Organizing release files..." -ForegroundColor Yellow

$BuildDir = Join-Path $RepoRoot "build"
$DeejReleaseExe = Join-Path $BuildDir "deej-release.exe"
$DeejDevExe = Join-Path $BuildDir "deej-dev.exe"
$DefaultConfig = Join-Path $RepoRoot "pkg\deej\scripts\misc\default-config.yaml"
$ReleaseNotes = Join-Path $RepoRoot "pkg\deej\scripts\misc\release-notes.txt"

if (-not $SkipBuild) {
    if (Test-Path $DeejReleaseExe) {
        Move-Item -Path $DeejReleaseExe -Destination (Join-Path $ReleaseDir "deej.exe") -Force
        Write-Host "  - deej-release.exe -> deej.exe" -ForegroundColor Green
    }

    if (Test-Path $DeejDevExe) {
        Move-Item -Path $DeejDevExe -Destination (Join-Path $ReleaseDir "deej-debug.exe") -Force
        Write-Host "  - deej-dev.exe -> deej-debug.exe" -ForegroundColor Green
    }
}

if (Test-Path $DefaultConfig) {
    Copy-Item -Path $DefaultConfig -Destination (Join-Path $ReleaseDir "config.yaml") -Force
    Write-Host "  - default-config.yaml -> config.yaml" -ForegroundColor Green
}

if (Test-Path $ReleaseNotes) {
    Copy-Item -Path $ReleaseNotes -Destination (Join-Path $ReleaseDir "notes.txt") -Force
    Write-Host "  - release-notes.txt -> notes.txt" -ForegroundColor Green
}

$EspHomeDir = Join-Path $RepoRoot "esphome"
if (Test-Path $EspHomeDir) {
    Copy-Item -Path "$EspHomeDir\*" -Destination $ReleaseDir -Recurse -Force
    Write-Host "  - esphome/* -> (release root)" -ForegroundColor Green
}

Write-Host ""
Write-Host "Incrementing Minor for next release..." -ForegroundColor Yellow
$Minor = $CurrentMinor + 1
Write-Host "New Minor: $Minor" -ForegroundColor Green

Write-Host ""
Write-Host "Updating versioninfo.cfg..." -ForegroundColor Yellow
$UpdatedVersionInfo = @{
    Major = $Major
    Minor = $Minor
    Build = $Build
}
$UpdatedVersionInfoJson = $UpdatedVersionInfo | ConvertTo-Json -Compress
$UpdatedVersionInfoJson = $UpdatedVersionInfoJson + "`n"
Set-Content -Path $VersionInfoFile -Value $UpdatedVersionInfoJson -NoNewline
Write-Host "Updated: Major=$Major, Minor=$Minor, Build=$Build" -ForegroundColor Green

Write-Host ""
Write-Host "Committing changes..." -ForegroundColor Yellow
git add "$VersionInfoFile"
git add -u

git commit -m "Bump version to $Major.$Minor.$Build"
if ($LASTEXITCODE -ne 0) {
    Write-Warning "Git commit failed (may be no changes to commit)"
}
else {
    Write-Host "Changes committed successfully" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Release prepared successfully!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Version tag: $VersionTag" -ForegroundColor White
Write-Host "Release directory: $ReleaseDir" -ForegroundColor White
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. Push tag: git push origin $VersionTag" -ForegroundColor White
Write-Host "  2. Push commits: git push" -ForegroundColor White
Write-Host "  3. Draft release on GitHub" -ForegroundColor White
Write-Host ""

if (Test-Path $ReleaseDir) {
    Write-Host "Opening release directory..." -ForegroundColor Yellow
    Start-Process explorer.exe -ArgumentList $ReleaseDir
}
