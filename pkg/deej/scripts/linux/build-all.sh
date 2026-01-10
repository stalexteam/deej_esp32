#!/bin/sh

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

echo 'Building deej (all)...'

"$SCRIPT_DIR/build-dev.sh" && "$SCRIPT_DIR/build-release.sh"
