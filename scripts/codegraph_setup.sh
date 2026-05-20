#!/usr/bin/env bash
# Initialise + index CodeGraph for Herald per Universal §11.4.78.
#
# Idempotent: if .codegraph/ already exists, runs `sync` instead of `init`
# so we don't blow away any operator-tuned config.
#
# Usage:
#   scripts/codegraph_setup.sh           # init + index (or sync if already init'd)
#   scripts/codegraph_setup.sh status    # show index stats only

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

if ! command -v npx >/dev/null 2>&1; then
    echo "ERROR: npx not on PATH. Install Node.js 18+ first." >&2
    exit 1
fi

case "${1:-init-or-sync}" in
    status)
        npx -y @colbymchenry/codegraph status .
        ;;
    init-or-sync)
        if [ -d "${REPO_ROOT}/.codegraph" ]; then
            echo "CodeGraph already initialised — running sync..."
            npx -y @colbymchenry/codegraph sync .
        else
            echo "Initialising CodeGraph for Herald..."
            npx -y @colbymchenry/codegraph init .
            echo "Indexing (this can take a few minutes for the full repo)..."
            npx -y @colbymchenry/codegraph index .
        fi
        npx -y @colbymchenry/codegraph status .
        ;;
    *)
        echo "Usage: $0 [status|init-or-sync]" >&2
        exit 2
        ;;
esac
