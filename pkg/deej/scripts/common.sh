#!/bin/sh

find_repo_root() {
    START_PATH="${1:-$(pwd)}"
    CURRENT_PATH="$START_PATH"
    MAX_DEPTH=10
    DEPTH=0
    
    while [ $DEPTH -lt $MAX_DEPTH ]; do
        if [ -f "$CURRENT_PATH/versioninfo.cfg" ]; then
            echo "$CURRENT_PATH"
            return 0
        fi
        
        PARENT="$(dirname "$CURRENT_PATH")"
        if [ "$PARENT" = "$CURRENT_PATH" ]; then
            break
        fi
        CURRENT_PATH="$PARENT"
        DEPTH=$((DEPTH + 1))
    done
    
    echo "Error: Could not find repository root (versioninfo.cfg not found)" >&2
    exit 1
}

reset_versioninfo() {
    git checkout -- "versioninfo.cfg" 2>/dev/null
}

get_version_major_minor() {
    if [ ! -f "versioninfo.cfg" ]; then
        echo "1.0"
        return
    fi
    
    MAJOR_MINOR=$(python3 -c "import json; import os; f=open('versioninfo.cfg'); j=json.load(f); f.close(); print(str(j['Major']) + '.' + str(j['Minor']))" 2>/dev/null)
    
    if [ -z "$MAJOR_MINOR" ]; then
        MAJOR_MINOR=$(python -c "import json; import os; f=open('versioninfo.cfg'); j=json.load(f); f.close(); print(str(j['Major']) + '.' + str(j['Minor']))" 2>/dev/null)
    fi
    
    if [ -z "$MAJOR_MINOR" ]; then
        MAJOR_MINOR="1.0"
    fi
    
    echo "$MAJOR_MINOR"
}

get_git_build_count() {
    BUILD=$(git rev-list --count HEAD 2>/dev/null)
    if [ -z "$BUILD" ]; then
        BUILD=0
    fi
    echo "$BUILD"
}

get_git_commit() {
    COMMIT=$(git rev-list -1 --abbrev-commit HEAD 2>/dev/null)
    if [ -z "$COMMIT" ]; then
        COMMIT="unknown"
    fi
    echo "$COMMIT"
}
