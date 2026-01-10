function Get-RepoRoot {
    param(
        [string]$StartPath = $null
    )
    
    if ([string]::IsNullOrWhiteSpace($StartPath)) {
        $StartPath = Get-Location
    }
    
    $currentPath = Resolve-Path $StartPath
    $maxDepth = 10
    $depth = 0
    
    while ($depth -lt $maxDepth) {
        $versionInfoFile = Join-Path $currentPath "versioninfo.cfg"
        if (Test-Path $versionInfoFile) {
            return $currentPath
        }
        
        $parent = Split-Path -Parent $currentPath
        if ([string]::IsNullOrWhiteSpace($parent) -or $parent -eq $currentPath) {
            break
        }
        $currentPath = $parent
        $depth++
    }
    
    Write-Error "Could not find repository root (versioninfo.cfg not found)"
    exit 1
}

function Get-VersionInfo {
    param(
        [string]$RepoRoot
    )
    
    $VersionInfoFile = Join-Path $RepoRoot "versioninfo.cfg"
    
    if (-not (Test-Path $VersionInfoFile)) {
        Write-Warning "versioninfo.cfg not found"
        return @{
            Major = 1
            Minor = 0
            Build = 0
        }
    }
    
    try {
        $VersionInfoJson = Get-Content $VersionInfoFile -Raw | ConvertFrom-Json
        return @{
            Major = $VersionInfoJson.Major
            Minor = $VersionInfoJson.Minor
            Build = if ($VersionInfoJson.Build) { $VersionInfoJson.Build } else { 0 }
        }
    }
    catch {
        Write-Error "Failed to parse versioninfo.cfg: $_"
        exit 1
    }
}

function Get-GitBuildCount {
    try {
        $count = git rev-list --count HEAD 2>$null
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($count)) {
            return 0
        }
        return [int]$count
    }
    catch {
        return 0
    }
}

function Get-VersionTag {
    param(
        [hashtable]$VersionInfo,
        [int]$Build = -1
    )
    
    if ($Build -eq -1) {
        $Build = if ($VersionInfo.Build) { $VersionInfo.Build } else { 0 }
    }
    
    return "v$($VersionInfo.Major).$($VersionInfo.Minor).$Build"
}

function Get-GitCommit {
    try {
        $commit = git rev-list -1 --abbrev-commit HEAD 2>$null
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($commit)) {
            return "unknown"
        }
        return $commit
    }
    catch {
        return "unknown"
    }
}

function Invoke-Build {
    param(
        [string]$RepoRoot,
        [string]$BuildType,
        [string]$OutputFile,
        [string]$VersionTag,
        [string]$GitCommit,
        [switch]$NoExit = $false
    )
    
    $cmdPath = Join-Path $RepoRoot "pkg\deej\cmd"
    $buildDir = Join-Path $RepoRoot "build"
    
    if (-not (Test-Path $buildDir)) {
        New-Item -ItemType Directory -Path $buildDir | Out-Null
    }
    
    $fullOutput = Join-Path $buildDir $OutputFile
    
    $baseFlags = @(
        "-X main.gitCommit=$GitCommit",
        "-X main.versionTag=$VersionTag",
        "-X main.buildType=$BuildType"
    )
    
    $ldFlags = $baseFlags -join " "
    
    if ($BuildType -eq "release") {
        $ldFlags = "-H=windowsgui -s -w " + $ldFlags
    }
    
    & go build -o $fullOutput -ldflags $ldFlags $cmdPath
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Build failed!"
        if (-not $NoExit) {
            exit 1
        }
        return $false
    }
    
    Write-Host "Done." -ForegroundColor Green
    return $true
}
