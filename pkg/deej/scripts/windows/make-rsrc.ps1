. "$PSScriptRoot\tools.ps1"

$ErrorActionPreference = "Stop"

if (-not $env:GOPATH) {
    Write-Error "GOPATH not set"
    exit 1
}

$RepoRoot = Get-RepoRoot
$ToolPath = Join-Path $env:GOPATH "bin\rsrc.exe"

if (-not (Test-Path $ToolPath)) {
    Write-Host "Installing rsrc..." -ForegroundColor Yellow
    & go install github.com/akavel/rsrc@latest
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to install rsrc"
        exit 1
    }
    $ToolPath = Join-Path $env:GOPATH "bin\rsrc.exe"
}

$ManifestPath = Join-Path $RepoRoot "pkg\deej\assets\deej.manifest"
$IconPath = Join-Path $RepoRoot "pkg\deej\assets\logo.ico"
$OutputPath = Join-Path $RepoRoot "pkg\deej\cmd\rsrc_windows.syso"

Write-Host "Creating rsrc_windows.syso..." -ForegroundColor Yellow
& $ToolPath -manifest $ManifestPath -ico $IconPath -o $OutputPath

if ($LASTEXITCODE -eq 0) {
    Write-Host "Done." -ForegroundColor Green
} else {
    Write-Error "Failed to create rsrc_windows.syso"
    exit 1
}
