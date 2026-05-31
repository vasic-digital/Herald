#!/usr/bin/env bash
#
# helixqa_run.sh — Herald-side launcher for autonomous anti-bluff QA via
# the HelixQA framework (sibling repo at ~/Projects/helixqa, CONST-051
# flat-layout). This is Herald's autonomous-QA single-source-of-truth
# entry point: it brings up the real services, drives the real flavor
# binaries, runs the HelixQA banks under challenges/helixqa-banks/, and
# preserves the report + evidence under qa-results/helixqa/<run-id>/.
#
# Anti-bluff posture (Herald §107 / Helix §11.4):
#   - We FAIL LOUD if `claude` is not on PATH. The HelixQA autonomous
#     session degrades to a no-LLM observation-only mode when no provider
#     (API key OR a bridged CLI like claude) is present. An
#     observation-only run that prints a green line is exactly the
#     "absence-of-error PASS" the covenant forbids — so we refuse to
#     proceed rather than emit a bluff PASS.
#   - The api bank runs against a REAL `pherald serve` HTTPS listener
#     backed by a REAL Postgres (mirrors scripts/e2e_bluff_hunt.sh's
#     PG + Gin boot), not a mock.
#   - The desktop/cli bank runs the REAL compiled flavor binaries.
#   - Any FAIL → this script exits non-zero (release-gate composable).
#   - The launcher ATTEMPTS to boot Postgres (quickstart compose, mirroring
#     scripts/e2e_bluff_hunt.sh) before deciding whether the api bank can run.
#     When PG cannot be brought up, the api bank is SKIPPED as a DISTINCT,
#     clearly-non-green outcome — a CLI-only green MUST NOT masquerade as full
#     coverage. Exit-status contract:
#         0 = all selected banks ran AND passed (api bank included).
#         1 = a bank reported FAILURE (release-gate block).
#         2 = PARTIAL / UNPROVEN — CLI bank passed but the api bank was SKIPPED
#             (Postgres unreachable); the pherald /v1/* HTTP+security surface
#             was NEVER exercised this run.
#
# Usage:
#   scripts/helixqa_run.sh                 # banks-driven run (default)
#   HELIXQA_AUTONOMOUS=1 scripts/helixqa_run.sh   # also run the autonomous session
#
# Environment overrides:
#   HELIXQA_DIR        path to the helixqa sibling repo (default ~/Projects/helixqa)
#   HERALD_HTTP_PORT   pherald serve HTTPS port (default 24791)
#   HERALD_PG_DSN      Postgres DSN (default postgres://herald:herald_dev@127.0.0.1:24100/herald)
#   HERALD_PG_PORT     Postgres TCP port to probe (default 24100)
#
set -euo pipefail

# ── Paths ─────────────────────────────────────────────────────────────
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

HELIXQA_DIR="${HELIXQA_DIR:-${HOME}/Projects/helixqa}"
HELIXQA_BIN="${HELIXQA_DIR}/bin/helixqa"
BANKS_DIR="${REPO_ROOT}/challenges/helixqa-banks"
API_BANK="${BANKS_DIR}/herald-api-v1.yaml"
CLI_BANK="${BANKS_DIR}/herald-cli-flavors.yaml"

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${REPO_ROOT}/qa-results/helixqa/${RUN_ID}"
mkdir -p "${OUT_DIR}"

HERALD_HTTP_PORT="${HERALD_HTTP_PORT:-24791}"
HERALD_PG_PORT="${HERALD_PG_PORT:-24100}"
HERALD_PG_DSN="${HERALD_PG_DSN:-postgres://herald:herald_dev@127.0.0.1:${HERALD_PG_PORT}/herald}"
HERALD_BIN_DIR="${OUT_DIR}/bin"
mkdir -p "${HERALD_BIN_DIR}"

# JWT verifier config — pherald serve REFUSES to start without a usable
# verifier. HMAC mode mirrors scripts/e2e_bluff_hunt.sh E7-E12.
export HERALD_AUTH_MODE="${HERALD_AUTH_MODE:-hmac}"
export HERALD_AUTH_HMAC_SECRET="${HERALD_AUTH_HMAC_SECRET:-test-secret-32-bytes-of-padding!!}"

PHERALD_PID=""
EXIT_CODE=0
# API_SKIPPED records the distinct "partial / unproven" outcome: the CLI bank
# ran but the pherald /v1/* HTTP+security surface was NEVER exercised this run
# (Postgres could not be brought up). It maps to a DEDICATED exit status (2)
# so a CLI-only green can never masquerade as full coverage. See the summary
# block + the final `exit` resolution at the bottom of this script.
API_SKIPPED=0
API_SKIP_REASON=""

log()  { printf '[helixqa_run] %s\n' "$*"; }
fail() { printf '[helixqa_run] FAIL: %s\n' "$*" >&2; EXIT_CODE=1; }

# ── pg_self_heal <password> — dev-container role-password normalization ──
# Mirrors scripts/e2e_bluff_hunt.sh's pg_self_heal: the shared herald-postgres
# dev container (host port ${HERALD_PG_PORT}) may carry an arbitrary
# POSTGRES_PASSWORD from a prior session / compose-up. `pherald serve` connects
# with the DSN password baked into HERALD_PG_DSN ("herald_dev" by default), so
# reconcile the role password to what THIS run needs right before connecting.
# The container's LOCAL unix socket uses trust auth, so the ALTER works via
# podman/docker exec regardless of the current password. No-op (returns 0) when
# the container or its runtime is unavailable — never hard-fails.
pg_self_heal() {
    local pw="$1" rt=""
    if command -v podman >/dev/null 2>&1; then rt="podman"
    elif command -v docker >/dev/null 2>&1; then rt="docker"
    else return 0; fi
    "${rt}" exec herald-postgres psql -U herald -d herald \
        -c "ALTER USER herald WITH PASSWORD '${pw}'" >/dev/null 2>&1 || true
    return 0
}

# ── boot_postgres — bring the real PG up before deciding API_READY ──────
# Mirrors scripts/e2e_bluff_hunt.sh's E13 PG boot: if PG is already reachable
# on :${HERALD_PG_PORT} this is a no-op; otherwise it boots the quickstart
# compose `postgres` service (podman preferred, docker fallback) and waits for
# reachability. Returns 0 when PG is reachable at the end, non-zero otherwise.
# When NO container runtime is present it falls through to a non-zero return so
# the caller emits the honest SKIP (never a fabricated PASS).
boot_postgres() {
    if command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 "${HERALD_PG_PORT}" 2>/dev/null; then
        log "Postgres already reachable on :${HERALD_PG_PORT} (no boot needed)"
        return 0
    fi
    # Pick a compose driver. podman-compose / docker compose both consume the
    # quickstart compose file used by e2e_bluff_hunt.sh.
    local composed=0
    if command -v podman-compose >/dev/null 2>&1; then
        log "booting Postgres via podman-compose (quickstart compose, project herald-e2e)"
        HERALD_DB_PASSWORD="herald_dev" \
        HERALD_REDIS_PASSWORD="test-redis-password-DO-NOT-USE-IN-PROD" \
        HERALD_PROJECT_NAME="Herald-HelixQA" \
        HERALD_TENANT_ID="00000000-0000-0000-0000-000000000099" \
            podman-compose -f "${REPO_ROOT}/quickstart/docker-compose.quickstart.yml" \
            --project-name herald-e2e up -d postgres >"${OUT_DIR}/pg_boot.log" 2>&1 && composed=1 || true
    elif command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        log "booting Postgres via docker compose (quickstart compose, project herald-e2e)"
        HERALD_DB_PASSWORD="herald_dev" \
        HERALD_REDIS_PASSWORD="test-redis-password-DO-NOT-USE-IN-PROD" \
        HERALD_PROJECT_NAME="Herald-HelixQA" \
        HERALD_TENANT_ID="00000000-0000-0000-0000-000000000099" \
            docker compose -f "${REPO_ROOT}/quickstart/docker-compose.quickstart.yml" \
            --project-name herald-e2e up -d postgres >"${OUT_DIR}/pg_boot.log" 2>&1 && composed=1 || true
    else
        log "no container runtime (podman-compose / docker compose) available — cannot boot Postgres"
        return 1
    fi
    if [ "${composed}" != 1 ]; then
        log "Postgres compose-up failed — see ${OUT_DIR}/pg_boot.log"
        return 1
    fi
    # Wait for reachability (max 30s), mirroring the e2e boot loop.
    for _ in $(seq 1 30); do
        if command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 "${HERALD_PG_PORT}" 2>/dev/null; then
            log "Postgres now reachable on :${HERALD_PG_PORT}"
            return 0
        fi
        sleep 1
    done
    log "Postgres never became reachable on :${HERALD_PG_PORT} within 30s"
    return 1
}

cleanup() {
    if [ -n "${PHERALD_PID}" ] && kill -0 "${PHERALD_PID}" 2>/dev/null; then
        log "stopping pherald serve (pid ${PHERALD_PID})"
        kill -TERM "${PHERALD_PID}" 2>/dev/null || true
        wait "${PHERALD_PID}" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

# ── Preflight: claude on PATH (anti-bluff hard gate) ──────────────────
if ! command -v claude >/dev/null 2>&1; then
    cat >&2 <<'EOF'
[helixqa_run] FAIL: `claude` is not on PATH.
  HelixQA's LLM/Vision bridge (BridgedCLIProvider) needs the `claude` CLI
  to drive the autonomous session and to evaluate prose-step banks. With no
  LLM provider the session degrades to observation-only, which produces a
  green line WITHOUT real anti-bluff verification — a §11.4 PASS-bluff.
  Install Claude Code and ensure `claude` resolves, then re-run.
EOF
    exit 1
fi
log "claude found at: $(command -v claude)"

# ── Preflight: required banks exist ───────────────────────────────────
for b in "${API_BANK}" "${CLI_BANK}"; do
    if [ ! -f "${b}" ]; then
        fail "bank not found: ${b}"
        exit 1
    fi
done

# ── Build the helixqa binary if missing ───────────────────────────────
if [ ! -x "${HELIXQA_BIN}" ]; then
    if [ ! -d "${HELIXQA_DIR}" ]; then
        fail "HelixQA sibling repo not found at ${HELIXQA_DIR} (CONST-051 flat layout). Clone it alongside Herald."
        exit 1
    fi
    log "building helixqa binary (${HELIXQA_BIN} missing)"
    if ! ( cd "${HELIXQA_DIR}" && go build -o "${HELIXQA_BIN}" ./cmd/helixqa ) >"${OUT_DIR}/helixqa_build.log" 2>&1; then
        fail "helixqa build failed — see ${OUT_DIR}/helixqa_build.log"
        exit 1
    fi
fi
log "helixqa: $("${HELIXQA_BIN}" version 2>/dev/null || echo '(version unavailable)')"

# ── Build all 8 Herald flavor binaries for the CLI bank ───────────────
log "building Herald flavor binaries into ${HERALD_BIN_DIR}"
for flavor in pherald sherald cherald bherald rherald iherald scherald qaherald; do
    if ! go build -o "${HERALD_BIN_DIR}/${flavor}" "./${flavor}/cmd/${flavor}" >>"${OUT_DIR}/flavor_build.log" 2>&1; then
        fail "go build failed for flavor ${flavor} — see ${OUT_DIR}/flavor_build.log"
    fi
done
export HERALD_BIN_DIR

# ── Boot a real pherald serve for the api bank ────────────────────────
# pherald serve REQUIRES a reachable Postgres (Wave 3b). We now ATTEMPT to
# bring PG up first (mirroring scripts/e2e_bluff_hunt.sh's E13 PG boot via the
# quickstart compose) rather than passively skipping when PG happens to be
# down. Only when NO container runtime can boot PG do we fall through to the
# honest, clearly-non-green SKIP (API_SKIPPED=1 → distinct exit status 2).
API_READY=0
if boot_postgres; then
    # Reconcile the dev container role password to the DSN the serve probe uses
    # ("herald_dev"), exactly like e2e_bluff_hunt.sh's pg_self_heal before E7-E12.
    pg_self_heal "herald_dev"
    log "Postgres reachable on :${HERALD_PG_PORT} — booting pherald serve on :${HERALD_HTTP_PORT}"
    HERALD_AUTH_MODE="${HERALD_AUTH_MODE}" \
    HERALD_AUTH_HMAC_SECRET="${HERALD_AUTH_HMAC_SECRET}" \
    HERALD_PG_DSN="${HERALD_PG_DSN}" \
        "${HERALD_BIN_DIR}/pherald" serve --http-port "${HERALD_HTTP_PORT}" \
        >"${OUT_DIR}/pherald_serve.log" 2>&1 &
    PHERALD_PID=$!

    # Wave 4a: the TCP listener is HTTPS — probe with curl -k.
    for _ in $(seq 1 20); do
        if curl -k -fsS "https://127.0.0.1:${HERALD_HTTP_PORT}/v1/healthz" >/dev/null 2>&1; then
            API_READY=1
            break
        fi
        sleep 0.5
    done
    if [ "${API_READY}" = 1 ]; then
        log "pherald serve is accepting HTTPS on :${HERALD_HTTP_PORT}"
        # Capture a live healthz body as positive runtime evidence.
        curl -k -fsS "https://127.0.0.1:${HERALD_HTTP_PORT}/v1/healthz" \
            >"${OUT_DIR}/healthz_evidence.json" 2>/dev/null || true
        # Point the api bank's shell:curl steps at the REAL listener. The api
        # bank reads HELIXQA_HTTP_BASE_URL as its base URL (falling back to
        # https://127.0.0.1:${HERALD_HTTP_PORT}); exporting it here is the
        # single source of truth so the curl gates hit the live self-signed
        # HTTPS listener.
        export HELIXQA_HTTP_BASE_URL="https://127.0.0.1:${HERALD_HTTP_PORT}"
        log "exported HELIXQA_HTTP_BASE_URL=${HELIXQA_HTTP_BASE_URL}"
    else
        API_SKIPPED=1
        API_SKIP_REASON="pherald serve never accepted HTTPS within 10s (PG was up; see pherald_serve.log)"
        log "API bank SKIPPED — ${API_SKIP_REASON}; the pherald /v1/* HTTP+security surface is UNPROVEN this run"
        echo "API bank SKIPPED at ${RUN_ID}: ${API_SKIP_REASON}" >"${OUT_DIR}/api_serve_skip.txt"
    fi
else
    API_SKIPPED=1
    API_SKIP_REASON="Postgres :${HERALD_PG_PORT} unreachable and no container runtime could boot it (§11.4.3 SKIP-with-reason)"
    log "API bank SKIPPED — ${API_SKIP_REASON}; the pherald /v1/* HTTP+security surface is UNPROVEN this run"
    echo "API bank SKIPPED at ${RUN_ID}: ${API_SKIP_REASON}" >"${OUT_DIR}/api_serve_skip.txt"
fi
export HERALD_HTTP_PORT

# ── Run the banks ─────────────────────────────────────────────────────
# Select which banks to run: always the CLI bank; add the API bank only
# when the live serve came up so we never claim API coverage we did not
# actually exercise.
BANKS="${CLI_BANK}"
if [ "${API_READY}" = 1 ]; then
    BANKS="${API_BANK},${CLI_BANK}"
fi

log "running helixqa banks: ${BANKS}"
set +e
# Platform: the CLI bank is [desktop]; --platform all spreads it across
# android/androidtv/web where the [desktop] cases SKIP (noise). Run the CLI
# bank on desktop. (The api bank's [api] platform is not a `helixqa run`
# platform — android|web|desktop|all — so the api plane is driven separately;
# see HELIXQA_INTEGRATION.md "api bank wiring" follow-up.)
HQ_PLATFORM="desktop"
"${HELIXQA_BIN}" run \
    --banks "${BANKS}" \
    --platform "${HQ_PLATFORM}" \
    --output "${OUT_DIR}" \
    --verbose \
    >"${OUT_DIR}/helixqa_run.log" 2>&1
RUN_RC=$?
set -e
log "helixqa run exited ${RUN_RC} — report + evidence under ${OUT_DIR}"
if [ "${RUN_RC}" != 0 ]; then
    fail "helixqa run reported failures (rc=${RUN_RC}) — inspect ${OUT_DIR}/helixqa_run.log"
fi

# ── Optional: autonomous LLM-driven session ───────────────────────────
# Off by default because it needs the full live env (claude + platform
# targets). Enable with HELIXQA_AUTONOMOUS=1.
if [ "${HELIXQA_AUTONOMOUS:-0}" = 1 ]; then
    log "running helixqa autonomous session (project=${REPO_ROOT})"
    set +e
    "${HELIXQA_BIN}" autonomous \
        --project "${REPO_ROOT}" \
        --platforms api,desktop \
        --output "${OUT_DIR}/autonomous" \
        --report markdown,html,json \
        --verbose \
        >"${OUT_DIR}/helixqa_autonomous.log" 2>&1
    AUTO_RC=$?
    set -e
    log "helixqa autonomous exited ${AUTO_RC} — see ${OUT_DIR}/helixqa_autonomous.log"
    if [ "${AUTO_RC}" != 0 ]; then
        fail "helixqa autonomous session reported failures (rc=${AUTO_RC})"
    fi
fi

# ── Summary ───────────────────────────────────────────────────────────
# Resolve the exit status:
#   1 = a bank reported FAILURE (release-gate block, set via fail()).
#   2 = PARTIAL / UNPROVEN — the CLI bank ran but the api bank was SKIPPED
#       (PG could not be brought up), so the pherald /v1/* HTTP+security
#       surface was NEVER exercised. A CLI-only green MUST NOT masquerade as
#       full coverage, so this is a DISTINCT non-zero status.
#   0 = all selected banks ran AND passed (full coverage, api included).
# FAIL (1) dominates PARTIAL (2): a real failure outranks an unproven surface.
FINAL_RC="${EXIT_CODE}"
if [ "${FINAL_RC}" = 0 ] && [ "${API_SKIPPED}" = 1 ]; then
    FINAL_RC=2
fi

log "=========================================================="
log "HelixQA run ${RUN_ID} complete"
log "  banks run:    ${BANKS}"
if [ "${API_READY}" = 1 ]; then
    log "  api bank:     RAN + passed (live pherald /v1/* HTTP+security surface exercised)"
else
    log "  api bank:     SKIPPED — Postgres unreachable; the pherald /v1/* HTTP+security surface is UNPROVEN this run"
    log "                reason: ${API_SKIP_REASON}"
    log "                NOTE: this run is PARTIAL — CLI bank only. NOT full coverage."
fi
log "  evidence dir: ${OUT_DIR}"
case "${FINAL_RC}" in
    0) log "  result:       PASSED — all selected banks ran AND passed (api included)";;
    2) log "  result:       PARTIAL/UNPROVEN (exit 2) — CLI bank passed but api bank was SKIPPED";;
    *) log "  result:       FAILED (exit ${FINAL_RC}) — a bank reported failures";;
esac
log "=========================================================="

exit "${FINAL_RC}"
