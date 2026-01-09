#!/bin/sh

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$SCRIPT_DIR/../../../pkg/deej/cmd" || { echo "Failure: Could not enter target directory"; exit 1; }

echo "Building Deej ESP32 for Windows..."
echo "Working directory: $(pwd)"

go mod tidy
go mod vendor

echo 'Build Deej ESP32 GUI version...'
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod vendor -a -o ./deej-amd64.exe -ldflags='-H=windowsgui -s -w -extldflags "-static"'

echo 'Build Deej ESP32 debug version...'
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod vendor -a -o ./deej-debug-amd64.exe -ldflags='-extldflags "-static"'
