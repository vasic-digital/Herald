#!/usr/bin/env bash
#
# uninstall.sh — stop, disable, and remove the ATMOSphere↔Herald daemons
# installed by install.sh. Idempotent: tolerates already-removed units.
#
# It does NOT delete the workable DB, .env, docs/qa transcripts, or the pherald
# binary — only the service/agent definitions and their enabled state. Captured
# QA evidence under docs/qa/ is preserved (§107.x).
#
# Usage: deploy/atmosphere-herald/uninstall.sh [--dry-run]
#
set -euo pipefail

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

log() { printf '[uninstall] %s\n' "$*"; }
run() { if [ "$DRY_RUN" -eq 1 ]; then printf '[dry-run] %s\n' "$*"; else eval "$*"; fi; }

OS="$(uname -s)"

if [ "$OS" = "Linux" ]; then
    if ! command -v systemctl >/dev/null 2>&1; then
        log "systemctl not found — nothing to uninstall (systemd path)."; exit 0
    fi
    UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
    for unit in atmosphere-herald-watch.service atmosphere-herald-listen.service; do
        run "systemctl --user stop '$unit' 2>/dev/null || true"
        run "systemctl --user disable '$unit' 2>/dev/null || true"
        run "rm -f '$UNIT_DIR/$unit'"
        log "Removed $unit"
    done
    run "systemctl --user daemon-reload"
    run "systemctl --user reset-failed 2>/dev/null || true"
    log "systemd user units removed. (Lingering left enabled; disable manually with 'loginctl disable-linger \$(id -un)' if desired.)"

elif [ "$OS" = "Darwin" ]; then
    AGENT_DIR="$HOME/Library/LaunchAgents"
    for label in digital.vasic.herald.watch digital.vasic.herald.listen; do
        run "launchctl bootout gui/$(id -u)/$label 2>/dev/null || true"
        run "rm -f '$AGENT_DIR/$label.plist'"
        log "Removed $label"
    done
    log "launchd agents removed."
else
    log "unsupported OS '$OS' — nothing to do."
fi

log "Uninstall complete. (Workable DB, .env, and docs/qa transcripts were NOT touched.)"
