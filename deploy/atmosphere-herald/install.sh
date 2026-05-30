#!/usr/bin/env bash
#
# install.sh — idempotent installer for the ATMOSphere↔Herald daemons
# (HRD-153 `pherald watch` + HRD-152 `pherald listen`, HRD-157 host deployment).
#
# What it does (in order):
#   1. Resolves paths (Herald home, pherald binary, .env, workable DB, docs dir).
#   2. Validates prerequisites — FAILS LOUD and exits non-zero on any missing
#      hard requirement (§107 fail-loud: a daemon that boots with a missing
#      token/DB is a PASS-bluff). Soft prerequisites (MTProto session) WARN only.
#   3. Builds pherald if no binary is found and --build is passed (or PHERALD_BIN
#      is unset and a go toolchain + workspace exist).
#   4. Renders the platform unit templates (systemd on Linux, launchd on macOS),
#      substituting the resolved paths, into the user unit directory.
#   5. Enables + (re)starts both daemons and prints the verify commands.
#
# Re-running is safe: it overwrites the rendered units, reloads, and restarts.
# Nothing here touches root — everything is per-user (systemctl --user /
# launchctl bootstrap gui/$UID). Enable lingering so the units survive logout.
#
# Usage:
#   deploy/atmosphere-herald/install.sh [--herald-home DIR] [--env-file FILE]
#                                       [--pherald-bin PATH] [--workable-db PATH]
#                                       [--docs-dir DIR] [--build] [--no-start]
#                                       [--dry-run]
#
# Env overrides (flags win over env, env wins over defaults):
#   HERALD_HOME, HERALD_ENV_FILE, PHERALD_BIN, HERALD_WORKABLE_DB, HERALD_DOCS_DIR
#
set -euo pipefail

# ── Resolve repo root (this script lives at deploy/atmosphere-herald/) ────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ── Defaults (overridable by env, then flags) ────────────────────────────────
HERALD_HOME="${HERALD_HOME:-$REPO_ROOT_DEFAULT}"
ENV_FILE="${HERALD_ENV_FILE:-}"            # resolved against HERALD_HOME below
PHERALD_BIN="${PHERALD_BIN:-}"             # autodetected below
WORKABLE_DB="${HERALD_WORKABLE_DB:-}"      # resolved against HERALD_HOME below
DOCS_DIR="${HERALD_DOCS_DIR:-}"            # resolved against HERALD_HOME below
DO_BUILD=0
DO_START=1
DRY_RUN=0

log()  { printf '[install] %s\n' "$*"; }
warn() { printf '[install] WARNING: %s\n' "$*" >&2; }
die()  { printf '[install] ERROR: %s\n' "$*" >&2; exit 1; }
run()  { if [ "$DRY_RUN" -eq 1 ]; then printf '[dry-run] %s\n' "$*"; else eval "$*"; fi; }

# ── Parse flags ──────────────────────────────────────────────────────────────
while [ $# -gt 0 ]; do
    case "$1" in
        --herald-home)  HERALD_HOME="$2"; shift 2 ;;
        --env-file)     ENV_FILE="$2"; shift 2 ;;
        --pherald-bin)  PHERALD_BIN="$2"; shift 2 ;;
        --workable-db)  WORKABLE_DB="$2"; shift 2 ;;
        --docs-dir)     DOCS_DIR="$2"; shift 2 ;;
        --build)        DO_BUILD=1; shift ;;
        --no-start)     DO_START=0; shift ;;
        --dry-run)      DRY_RUN=1; shift ;;
        -h|--help)
            sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) die "unknown argument: $1 (try --help)" ;;
    esac
done

HERALD_HOME="$(cd "$HERALD_HOME" 2>/dev/null && pwd || die "HERALD_HOME does not exist: $HERALD_HOME")"
: "${ENV_FILE:=$HERALD_HOME/.env}"
: "${WORKABLE_DB:=$HERALD_HOME/docs/workable_items.db}"
: "${DOCS_DIR:=$HERALD_HOME/docs}"
QA_OUT_DIR="$HERALD_HOME/docs/qa/atmosphere-listen-$(date +%Y%m%dT%H%M%S)"

log "Herald home : $HERALD_HOME"
log "Env file    : $ENV_FILE"
log "Workable DB : $WORKABLE_DB"
log "Docs dir    : $DOCS_DIR"

# ── Locate / build the pherald binary ────────────────────────────────────────
if [ -z "$PHERALD_BIN" ]; then
    if [ -x "$HERALD_HOME/bin/pherald" ]; then
        PHERALD_BIN="$HERALD_HOME/bin/pherald"
    elif command -v pherald >/dev/null 2>&1; then
        PHERALD_BIN="$(command -v pherald)"
    fi
fi

if [ -z "$PHERALD_BIN" ] || [ ! -x "$PHERALD_BIN" ]; then
    if [ "$DO_BUILD" -eq 1 ] || { [ -z "$PHERALD_BIN" ] && command -v go >/dev/null 2>&1; }; then
        command -v go >/dev/null 2>&1 || die "no pherald binary and no 'go' toolchain to build one"
        log "Building pherald -> $HERALD_HOME/bin/pherald"
        run "mkdir -p '$HERALD_HOME/bin'"
        run "(cd '$HERALD_HOME' && go build -o bin/pherald ./pherald/cmd/pherald/)"
        PHERALD_BIN="$HERALD_HOME/bin/pherald"
    else
        die "pherald binary not found. Pass --pherald-bin PATH, or --build, or put it at $HERALD_HOME/bin/pherald"
    fi
fi
[ "$DRY_RUN" -eq 1 ] || [ -x "$PHERALD_BIN" ] || die "pherald binary not executable: $PHERALD_BIN"
log "pherald bin : $PHERALD_BIN"

# ── Prerequisite validation (HARD = die, SOFT = warn) ────────────────────────
log "Validating prerequisites …"

# HARD: .env present, readable, and carrying the daemon-critical vars.
[ -f "$ENV_FILE" ] || die ".env not found at $ENV_FILE — copy .env.example and populate it (see ATMOSPHERE_DAEMON_DEPLOY.md §Prerequisites)"
[ -r "$ENV_FILE" ] || die ".env not readable at $ENV_FILE"

# Read a var's value out of the .env without exporting the whole file (we only
# peek; the daemons load it themselves via EnvironmentFile / baked vars). A var
# already exported in the shell wins per Herald's resolution order, so accept
# either source.
env_value() {
    local key="$1" v=""
    v="$(printenv "$key" 2>/dev/null || true)"
    if [ -z "$v" ] && [ -f "$ENV_FILE" ]; then
        v="$(grep -E "^[[:space:]]*${key}=" "$ENV_FILE" | tail -n1 | cut -d= -f2- | sed 's/^["'\'']//; s/["'\'']$//')"
    fi
    printf '%s' "$v"
}

CHANNELS="$(env_value HERALD_CHANNELS)"
[ -n "$CHANNELS" ] || { CHANNELS="tgram"; warn "HERALD_CHANNELS unset — defaulting to 'tgram' (pherald's own default)"; }

# tgram is the ATMOSphere channel; if enabled, its token + chat id are HARD.
if printf '%s' "$CHANNELS" | tr ',' '\n' | grep -qx 'tgram'; then
    [ -n "$(env_value HERALD_TGRAM_BOT_TOKEN)" ] || die "HERALD_TGRAM_BOT_TOKEN empty in $ENV_FILE but channel 'tgram' is enabled — pherald watch/listen would fail at first getMe (§107 fail-loud)"
    [ -n "$(env_value HERALD_TGRAM_CHAT_ID)" ]   || die "HERALD_TGRAM_CHAT_ID empty in $ENV_FILE but channel 'tgram' is enabled"
fi

# HARD: HERALD_PROJECT_NAME should pin ATMOSphere for this deployment.
PROJECT_NAME="$(env_value HERALD_PROJECT_NAME)"
if [ "$PROJECT_NAME" != "ATMOSphere" ]; then
    warn "HERALD_PROJECT_NAME is '${PROJECT_NAME:-<unset>}', expected 'ATMOSphere' — the Claude Code session envelope will use that name. Set HERALD_PROJECT_NAME=ATMOSphere in $ENV_FILE for the ATMOSphere deployment."
fi

# HARD for listen: docs/Issues.md must exist (pherald listen refuses to boot otherwise).
[ -f "$DOCS_DIR/Issues.md" ] || die "$DOCS_DIR/Issues.md not found — pherald listen REFUSES to boot without it. Pass --docs-dir or fix HERALD_HOME."

# HARD for watch: the workable DB's parent dir must exist (the DB itself may be
# created empty by pherald, but the directory must be writable).
WORKABLE_DB_DIR="$(dirname "$WORKABLE_DB")"
[ -d "$WORKABLE_DB_DIR" ] || die "workable DB directory does not exist: $WORKABLE_DB_DIR"
if [ -f "$WORKABLE_DB" ]; then
    log "workable DB present: $WORKABLE_DB"
else
    warn "workable DB not yet materialized at $WORKABLE_DB — pherald watch will open an empty schema-only DB (no items to diff until HRD-150/HRD-155 lands the populated DB). See ATMOSPHERE_DAEMON_DEPLOY.md §Troubleshooting."
fi

# SOFT: MTProto session (only the qaherald QA harness needs it; the daemons do
# NOT). Warn so the operator knows live QA round-trips need the bootstrap, but
# never block daemon install on it.
MTPROTO_SESSION="$(env_value HERALD_MTPROTO_SESSION_FILE)"
: "${MTPROTO_SESSION:=$HOME/.config/herald/mtproto.session}"
if [ -f "$MTPROTO_SESSION" ]; then
    log "MTProto session present: $MTPROTO_SESSION (qaherald live-QA enabled)"
else
    warn "MTProto session absent at $MTPROTO_SESSION — the watch/listen daemons do NOT need it, but qaherald live round-trip QA (HRD-156) requires a one-time 'qaherald mtproto login'. See ATMOSPHERE_DAEMON_DEPLOY.md §One-time MTProto login."
fi

log "Prerequisite validation complete."

# ── Render + install the platform units ──────────────────────────────────────
OS="$(uname -s)"

render() { # render <template> <dest> ; applies the @PLACEHOLDER@ substitutions
    local tpl="$1" dest="$2"
    [ -f "$tpl" ] || die "template missing: $tpl"
    if [ "$DRY_RUN" -eq 1 ]; then
        printf '[dry-run] render %s -> %s\n' "$tpl" "$dest"; return 0
    fi
    sed -e "s#@HERALD_HOME@#${HERALD_HOME}#g" \
        -e "s#@PHERALD_BIN@#${PHERALD_BIN}#g" \
        -e "s#@ENV_FILE@#${ENV_FILE}#g" \
        -e "s#@WORKABLE_DB@#${WORKABLE_DB}#g" \
        -e "s#@DOCS_DIR@#${DOCS_DIR}#g" \
        -e "s#@QA_OUT_DIR@#${QA_OUT_DIR}#g" \
        "$tpl" > "$dest"
}

if [ "$OS" = "Linux" ]; then
    command -v systemctl >/dev/null 2>&1 || die "systemctl not found — this host is not systemd. Use the launchd path or run the daemons under your own supervisor (see ATMOSPHERE_DAEMON_DEPLOY.md §Without systemd)."
    UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
    run "mkdir -p '$UNIT_DIR'"
    render "$SCRIPT_DIR/systemd/atmosphere-herald-watch.service"  "$UNIT_DIR/atmosphere-herald-watch.service"
    render "$SCRIPT_DIR/systemd/atmosphere-herald-listen.service" "$UNIT_DIR/atmosphere-herald-listen.service"
    log "Installed systemd user units to $UNIT_DIR"

    # Enable lingering so the units survive logout (best-effort; needs the user
    # to be allowed loginctl — warn rather than die if not).
    if command -v loginctl >/dev/null 2>&1; then
        run "loginctl enable-linger '$(id -un)' || true"
    fi

    run "systemctl --user daemon-reload"
    run "systemctl --user enable atmosphere-herald-watch.service atmosphere-herald-listen.service"
    if [ "$DO_START" -eq 1 ]; then
        run "mkdir -p '$QA_OUT_DIR'"
        run "systemctl --user restart atmosphere-herald-watch.service atmosphere-herald-listen.service"
        log "Daemons started. Verify with:"
        log "  systemctl --user status atmosphere-herald-watch atmosphere-herald-listen"
        log "  journalctl --user -u atmosphere-herald-watch -u atmosphere-herald-listen -f"
    else
        log "Units enabled (not started — --no-start). Start with: systemctl --user start atmosphere-herald-watch atmosphere-herald-listen"
    fi

elif [ "$OS" = "Darwin" ]; then
    command -v launchctl >/dev/null 2>&1 || die "launchctl not found on Darwin host (unexpected)"
    AGENT_DIR="$HOME/Library/LaunchAgents"
    run "mkdir -p '$AGENT_DIR' '$HERALD_HOME/qa-results'"

    # launchd has no EnvironmentFile: bake the HERALD_* vars from .env into the
    # plist <EnvironmentVariables> dict at install time.
    env_xml() {
        # Emit "<key>K</key><string>V</string>" lines for every HERALD_* var
        # found in .env (already-set values, comments, and blanks skipped).
        grep -E '^[[:space:]]*HERALD_[A-Z0-9_]+=' "$ENV_FILE" 2>/dev/null | while IFS='=' read -r k v; do
            k="$(printf '%s' "$k" | tr -d '[:space:]')"
            v="$(printf '%s' "$v" | sed 's/^["'\'']//; s/["'\'']$//')"
            [ -n "$v" ] || continue
            # XML-escape & < >
            v="$(printf '%s' "$v" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g')"
            printf '        <key>%s</key>\n        <string>%s</string>\n' "$k" "$v"
        done
    }

    render_plist() { # render_plist <template> <dest>
        local tpl="$1" dest="$2" envblock
        [ -f "$tpl" ] || die "template missing: $tpl"
        if [ "$DRY_RUN" -eq 1 ]; then printf '[dry-run] render %s -> %s\n' "$tpl" "$dest"; return 0; fi
        envblock="$(env_xml)"
        # Two-stage: path placeholders via sed, then the multi-line env block via
        # awk (sed chokes on the newlines in $envblock).
        sed -e "s#@HERALD_HOME@#${HERALD_HOME}#g" \
            -e "s#@PHERALD_BIN@#${PHERALD_BIN}#g" \
            -e "s#@ENV_FILE@#${ENV_FILE}#g" \
            -e "s#@WORKABLE_DB@#${WORKABLE_DB}#g" \
            -e "s#@DOCS_DIR@#${DOCS_DIR}#g" \
            -e "s#@QA_OUT_DIR@#${QA_OUT_DIR}#g" \
            "$tpl" \
        | awk -v block="$envblock" '{ if ($0 ~ /@ENVIRONMENT_VARIABLES@/) print block; else print }' \
        > "$dest"
    }

    render_plist "$SCRIPT_DIR/launchd/digital.vasic.herald.watch.plist"  "$AGENT_DIR/digital.vasic.herald.watch.plist"
    render_plist "$SCRIPT_DIR/launchd/digital.vasic.herald.listen.plist" "$AGENT_DIR/digital.vasic.herald.listen.plist"
    log "Installed launchd agents to $AGENT_DIR"

    if [ "$DO_START" -eq 1 ]; then
        run "mkdir -p '$QA_OUT_DIR'"
        for label in digital.vasic.herald.watch digital.vasic.herald.listen; do
            run "launchctl bootout gui/$(id -u)/$label 2>/dev/null || true"
            run "launchctl bootstrap gui/$(id -u) '$AGENT_DIR/$label.plist'"
            run "launchctl enable gui/$(id -u)/$label"
        done
        log "Daemons bootstrapped. Verify with:"
        log "  launchctl print gui/$(id -u)/digital.vasic.herald.watch"
        log "  tail -f '$HERALD_HOME/qa-results/atmosphere-herald-'*.log"
    else
        log "Plists installed (not started — --no-start)."
    fi
else
    die "unsupported OS '$OS' — only Linux (systemd) and Darwin (launchd) are supported. Run the daemons under your own supervisor."
fi

log "Install complete."
