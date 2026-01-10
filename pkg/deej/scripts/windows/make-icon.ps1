param(
    [Parameter(Mandatory=$true)]
    [string]$IcoFile
)

$ErrorActionPreference = "Stop"

if (-not $env:GOPATH) {
    Write-Error "GOPATH not set"
    exit 1
}

$ToolPath = Join-Path $env:GOPATH "bin\2goarray.exe"

if (-not (Test-Path $ToolPath)) {
    Write-Host "Installing 2goarray..." -ForegroundColor Yellow
    & go install github.com/cratonica/2goarray@latest
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to install 2goarray"
        exit 1
    }
    $ToolPath = Join-Path $env:GOPATH "bin\2goarray.exe"
}

if (-not (Test-Path $IcoFile)) {
    Write-Error "File not found: $IcoFile"
    exit 1
}

Write-Host "Creating iconwin.go..." -ForegroundColor Yellow

& cmd.exe /c "TYPE `"$IcoFile`" | `"$ToolPath`" Data icon > iconwin.go"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Done." -ForegroundColor Green
} else {
    Write-Error "Failed to create iconwin.go"
    exit 1
}
