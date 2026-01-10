#!/bin/sh

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/../common.sh"

REPO_ROOT=$(find_repo_root)
cd "$REPO_ROOT" || { echo "Failure: Could not enter repository root"; exit 1; }

echo "Building deej for Windows (development)..."

reset_versioninfo

MAJOR_MINOR=$(get_version_major_minor)
BUILD=$(get_git_build_count)
VERSION_TAG="v${MAJOR_MINOR}.${BUILD}"

GIT_COMMIT=$(get_git_commit)

BUILD_TYPE=dev
echo "Embedding: gitCommit=$GIT_COMMIT, versionTag=$VERSION_TAG, buildType=$BUILD_TYPE"

mkdir -p build

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o build/deej-dev.exe -ldflags "-X main.gitCommit=$GIT_COMMIT -X main.versionTag=$VERSION_TAG -X main.buildType=$BUILD_TYPE" ./pkg/deej/cmd
if [ $? -eq 0 ]; then
    echo 'Done.'
    echo 'Output: build/deej-dev.exe'
else
    echo 'Build failed!'
    exit 1
fi
