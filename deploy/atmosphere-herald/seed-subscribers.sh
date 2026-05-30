#!/usr/bin/env bash
#
# seed-subscribers.sh — register the ATMOSphere notification recipients.
#
# IMPORTANT — current routing model (HRD-156 caveat):
#   `pherald watch` does NOT resolve recipients from the Postgres `subscribers`
#   table yet. It fans every change out to the per-channel configured Target
#   (the chat/channel id) directly — see WORKABLE_ITEMS_INTEGRATION.md §4.4 and
#   watch.go::buildWatchNotifier. So for the watch daemon, "registering the main
#   group as a subscriber" == setting HERALD_TGRAM_CHAT_ID to that group's id in
#   .env. PG-backed Subscriber resolution (per-subscriber preference + quiet
#   hours, HRD-154/HRD-156) is PENDING.
#
# This helper therefore does two things:
#   (1) PRIMARY (works today): verify .env pins HERALD_TGRAM_CHAT_ID to the
#       ATMOSphere main group, which is what watch/listen actually route to.
#   (2) FORWARD-COMPAT (no-op for watch today): when --pg is given AND psql +
#       DATABASE_URL are available, upsert a `subscribers` row + a
#       `subscriber_aliases` row mapping the main group to that subscriber, so
#       the PG path is pre-seeded for when HRD-156 lands. This requires the
#       app.tenant_id RLS setting (the subscribers table FORCEs row-level
#       security per migration 000003).
#
# Usage:
#   deploy/atmosphere-herald/seed-subscribers.sh                 # verify .env routing only
#   deploy/atmosphere-herald/seed-subscribers.sh --pg            # also pre-seed PG (needs DATABASE_URL + psql)
#   deploy/atmosphere-herald/seed-subscribers.sh --print-sql     # print the SQL and exit (no DB touch)
#
# Env: HERALD_ENV_FILE (default <repo>/.env), DATABASE_URL, HERALD_TENANT_ID
#      (UUID for the RLS tenant; default the all-zero UUID for a single-tenant host).
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ENV_FILE="${HERALD_ENV_FILE:-$REPO_ROOT/.env}"
TENANT_ID="${HERALD_TENANT_ID:-00000000-0000-0000-0000-000000000000}"

DO_PG=0
PRINT_SQL=0
while [ $# -gt 0 ]; do
    case "$1" in
        --pg)        DO_PG=1; shift ;;
        --print-sql) PRINT_SQL=1; shift ;;
        -h|--help)   sed -n '2,34p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) printf 'unknown arg: %s\n' "$1" >&2; exit 1 ;;
    esac
done

log()  { printf '[seed] %s\n' "$*"; }
warn() { printf '[seed] WARNING: %s\n' "$*" >&2; }
die()  { printf '[seed] ERROR: %s\n' "$*" >&2; exit 1; }

env_value() {
    local key="$1" v=""
    v="$(printenv "$key" 2>/dev/null || true)"
    if [ -z "$v" ] && [ -f "$ENV_FILE" ]; then
        v="$(grep -E "^[[:space:]]*${key}=" "$ENV_FILE" | tail -n1 | cut -d= -f2- | sed 's/^["'\'']//; s/["'\'']$//')"
    fi
    printf '%s' "$v"
}

CHAT_ID="$(env_value HERALD_TGRAM_CHAT_ID)"
DISPLAY="${HERALD_MAIN_GROUP_NAME:-ATMOSphere main group}"
HANDLE="${HERALD_MAIN_GROUP_HANDLE:-atmosphere-main}"

# ── The forward-compat SQL (subscriber + main-group alias) ───────────────────
# uuidv7() is provided by Herald's migrations (000001). The alias's UNIQUE
# (channel, channel_user_id) makes the upsert idempotent. RLS requires
# app.tenant_id to be SET in the same session (the SELECT set_config below).
read -r -d '' SEED_SQL <<SQL || true
BEGIN;
SELECT set_config('app.tenant_id', '${TENANT_ID}', true);
INSERT INTO subscribers (tenant_id, handle, display_name, kind, roles)
VALUES ('${TENANT_ID}', '${HANDLE}', '${DISPLAY}', 'human', ARRAY['operator'])
ON CONFLICT (tenant_id, handle) DO UPDATE SET display_name = EXCLUDED.display_name
RETURNING id \gset
INSERT INTO subscriber_aliases (subscriber_id, channel, channel_user_id, verified_at)
VALUES (:'id', 'tgram', '${CHAT_ID}', now())
ON CONFLICT (channel, channel_user_id)
  DO UPDATE SET subscriber_id = EXCLUDED.subscriber_id, verified_at = now();
COMMIT;
SQL

if [ "$PRINT_SQL" -eq 1 ]; then
    printf '%s\n' "$SEED_SQL"
    exit 0
fi

# ── (1) Verify the routing that actually works today ─────────────────────────
[ -f "$ENV_FILE" ] || die ".env not found at $ENV_FILE"
if [ -z "$CHAT_ID" ]; then
    die "HERALD_TGRAM_CHAT_ID is empty in $ENV_FILE — this IS the main-group routing target for pherald watch/listen today. Set it to the ATMOSphere main group's numeric chat id (see ATMOSPHERE_DAEMON_DEPLOY.md §Seed recipients)."
fi
log "OK: pherald watch/listen will route to HERALD_TGRAM_CHAT_ID=${CHAT_ID} (the ATMOSphere main group)."
log "    (PG-backed per-subscriber resolution is HRD-156 PENDING; the chat-id target is the live routing path.)"

# ── (2) Optional PG pre-seed (no-op for watch today; future HRD-156) ─────────
if [ "$DO_PG" -eq 1 ]; then
    command -v psql >/dev/null 2>&1 || die "--pg given but psql not found in PATH"
    [ -n "${DATABASE_URL:-}" ] || die "--pg given but DATABASE_URL is empty (export it or set in shell)"
    log "Pre-seeding Postgres subscribers + main-group alias (tenant ${TENANT_ID}) …"
    printf '%s\n' "$SEED_SQL" | psql "$DATABASE_URL" -v ON_ERROR_STOP=1
    log "PG pre-seed complete. (Not consumed by pherald watch until HRD-156 lands subscriber resolution.)"
else
    log "Skipping PG pre-seed (pass --pg to pre-seed the subscribers table for the future HRD-156 path)."
fi

log "Seed complete."
