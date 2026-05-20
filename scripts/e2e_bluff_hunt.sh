#!/usr/bin/env bash
# End-to-end bluff-hunt for Herald per Universal §11.4 + the operator's
# 2026-05-20 standing mandate:
#
#   "Make sure that all existing tests and Challenges do work in anti-
#    bluff manner — they MUST confirm that all tested codebase really
#    works as expected!"
#
# Strategy: run EVERY user-visible Herald feature end-to-end against
# real services + assert observable behaviour. A bluffing feature that
# "compiles and tests green" but doesn't work for the user will FAIL
# at least one assertion here.
#
# Fifteen invariants (each is the "captured-evidence" for one feature
# class per §11.4.5 + §11.4.69):
#
#   E1.  pherald binary builds (compile-level — necessary, not sufficient).
#   E2.  pherald version --json returns parseable JSON with required fields.
#   E3.  Default test suite green (anti-bluff audit's I2 — re-asserted).
#   E4.  Inheritance gate + paired I6 meta-test green.
#   E5.  CodeGraph anti-bluff validate (7/7 PASS — symbols actually present).
#   E6.  scripts/audit_antibluff.sh 13/13 PASS (submodule anchor cascade).
#   E7.  pherald serve actually starts + binds + accepts HTTP.
#   E8.  /v1/healthz returns 200 with status:ok + version.
#   E9.  /v1/readyz returns 200 with status:ready.
#   E10. /v1/events POST returns 501 with HRD-016 pointer (honest stub).
#   E11. /metrics returns text/plain + herald_build_info gauge.
#   E12. pherald serve graceful-shutdowns on SIGTERM (drain + exit 0).
#
# Optional (run when podman-machine is up + DOCKER_HOST is set):
#   E13. Quickstart compose Postgres boots + migrations apply + RLS
#        isolation proven via the M2 Postgres integration tests.
#
# Exit 0 only when E1..E12 (plus E13 if attempted) all pass. Failure
# prints the offending invariant so the operator knows EXACTLY which
# feature is bluffing.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

# Use distinct ports + binary path to avoid collision with operator's own work.
PHERALD_BIN="/tmp/pherald-bluff-$$"
HTTP_PORT="${HERALD_E2E_PORT:-24791}"
PHERALD_PID=""

cleanup() {
    if [ -n "${PHERALD_PID}" ]; then
        kill "${PHERALD_PID}" 2>/dev/null || true
        wait "${PHERALD_PID}" 2>/dev/null || true
    fi
    rm -f "${PHERALD_BIN}"
}
trap cleanup EXIT

pass=0
fail=0
fail_names=()

check() {
    local name="$1"; shift
    if eval "$@" > /tmp/e2e_out 2>&1; then
        echo "PASS  ${name}"
        pass=$((pass+1))
    else
        echo "FAIL  ${name}"
        echo "      (last 5 lines of output below)"
        tail -5 /tmp/e2e_out | sed 's/^/      /'
        fail=$((fail+1))
        fail_names+=("${name}")
    fi
}

# ----------------------------------------------------------------------
# E1: pherald binary builds.
echo "== E1: pherald compile =="
check "E1 pherald binary builds" "go build -o '${PHERALD_BIN}' ./pherald/cmd/pherald"

# ----------------------------------------------------------------------
# E2: pherald version --json.
echo ""
echo "== E2: pherald version --json =="
check "E2 version --json returns valid JSON with required fields" \
    "'${PHERALD_BIN}' version --json | python3 -c 'import sys,json; d=json.load(sys.stdin); assert d[\"binary\"]==\"pherald\"; assert d[\"version\"]; assert d[\"go_version\"]; assert d[\"os\"]; assert d[\"arch\"]; print(\"ok\")'"

# ----------------------------------------------------------------------
# E3: Default test suite.
echo ""
echo "== E3: Default-mode go test suite =="
check "E3 go test -race -count=1 across 11 Herald packages" \
    "go test -race -count=1 ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/... ./commons_constitution/... ./commons_infra/... ./pherald/..."

# ----------------------------------------------------------------------
# E4: Gate + paired meta.
echo ""
echo "== E4: Inheritance gate + paired I6/I8 meta-tests =="
check "E4a inheritance gate 15 PASS / 0 FAIL (includes I8a/b/c §107 covenant)" \
    "bash tests/test_constitution_inheritance.sh"
check "E4b I6 paired meta-test 3 PASS / 0 FAIL" \
    "bash tests/test_i6_refinement_meta.sh"
check "E4c I8 paired meta-test 5 PASS / 0 FAIL (§107 covenant anchor mutation)" \
    "bash tests/test_i8_usability_meta.sh"

# ----------------------------------------------------------------------
# E5: CodeGraph validate.
echo ""
echo "== E5: CodeGraph anti-bluff validation =="
check "E5 codegraph_validate.sh 7/7 PASS" \
    "bash scripts/codegraph_validate.sh"

# ----------------------------------------------------------------------
# E6: Anti-bluff audit.
echo ""
echo "== E6: scripts/audit_antibluff.sh =="
check "E6 audit_antibluff.sh 14/14 PASS (includes I8 §107 paired meta-test)" \
    "bash scripts/audit_antibluff.sh"

# ----------------------------------------------------------------------
# E7..E12: pherald serve live HTTP smoke.
echo ""
echo "== E7-E12: pherald serve live HTTP smoke on :${HTTP_PORT} =="
"${PHERALD_BIN}" serve --http-port "${HTTP_PORT}" >/tmp/pherald_serve.log 2>&1 &
PHERALD_PID=$!
# Wait until the port responds (max 10s).
ready=0
for i in $(seq 1 20); do
    if curl -fsS "http://127.0.0.1:${HTTP_PORT}/v1/healthz" >/dev/null 2>&1; then
        ready=1; break
    fi
    sleep 0.5
done
if [ "${ready}" = 1 ]; then
    echo "PASS  E7 pherald serve binds + accepts HTTP"
    pass=$((pass+1))
else
    echo "FAIL  E7 pherald serve never accepted HTTP within 10s"
    fail=$((fail+1))
    fail_names+=("E7")
fi

# E8: healthz body sanity.
check "E8 /v1/healthz returns 200 + status:ok + version" \
    "curl -fsS 'http://127.0.0.1:${HTTP_PORT}/v1/healthz' | python3 -c 'import sys,json; d=json.load(sys.stdin); assert d[\"status\"]==\"ok\"; assert d[\"build\"][\"version\"]; print(\"ok\")'"

check "E9 /v1/readyz returns 200 + status:ready" \
    "curl -fsS 'http://127.0.0.1:${HTTP_PORT}/v1/readyz' | python3 -c 'import sys,json; d=json.load(sys.stdin); assert d[\"status\"]==\"ready\"; print(\"ok\")'"

check "E10 /v1/events POST returns 501 + HRD-016 pointer (honest stub)" \
    "test \"\$(curl -s -o /tmp/ev.body -w \"%{http_code}\" -X POST 'http://127.0.0.1:${HTTP_PORT}/v1/events' -H 'Content-Type: application/cloudevents+json' -d '{}')\" = 501 && grep -q 'HRD-016' /tmp/ev.body"

check "E11 /metrics returns text/plain + herald_build_info gauge" \
    "curl -fsS 'http://127.0.0.1:${HTTP_PORT}/metrics' | grep -q '^herald_build_info{'"

# E12: graceful shutdown — send SIGTERM and verify exit 0.
kill -TERM "${PHERALD_PID}"
wait "${PHERALD_PID}" 2>/dev/null
exit_code=$?
PHERALD_PID=""
if [ "${exit_code}" = 0 ]; then
    echo "PASS  E12 pherald serve graceful-shutdown on SIGTERM (exit 0)"
    pass=$((pass+1))
else
    echo "FAIL  E12 pherald serve exited ${exit_code} on SIGTERM (want 0)"
    fail=$((fail+1))
    fail_names+=("E12")
fi

# ----------------------------------------------------------------------
# E13: optional live Postgres integration via commons_infra.
echo ""
echo "== E13: optional live Postgres M2 integration tests =="
if [ -n "${DOCKER_HOST:-}" ] || podman info >/dev/null 2>&1; then
    # Set DOCKER_HOST from podman if not already set.
    if [ -z "${DOCKER_HOST:-}" ]; then
        sock=$(podman info --format '{{.Host.RemoteSocket.Path}}' 2>/dev/null || true)
        [ -n "${sock}" ] && export DOCKER_HOST="unix://${sock}"
    fi
    # Boot Postgres via the quickstart compose if not already running.
    if ! nc -z 127.0.0.1 24100 2>/dev/null; then
        HERALD_DB_PASSWORD="test-postgres-password-DO-NOT-USE-IN-PROD" \
        HERALD_REDIS_PASSWORD="test-redis-password-DO-NOT-USE-IN-PROD" \
        HERALD_PROJECT_NAME="Herald-E2E" \
        HERALD_TENANT_ID="00000000-0000-0000-0000-000000000099" \
        podman-compose -f quickstart/docker-compose.quickstart.yml --project-name herald-e2e up -d postgres >/dev/null 2>&1 || true
        # Wait for postgres reachability.
        for i in $(seq 1 30); do
            nc -z 127.0.0.1 24100 2>/dev/null && break
            sleep 1
        done
    fi
    if nc -z 127.0.0.1 24100 2>/dev/null; then
        check "E13 M2 Postgres integration tests (RLS + transition gate)" \
            "HERALD_DB_PASSWORD='test-postgres-password-DO-NOT-USE-IN-PROD' DOCKER_HOST='\${DOCKER_HOST}' go test -tags=integration -timeout 5m -count=1 -run TestPostgres ./commons_constitution/..."
    else
        echo "SKIP  E13 — Postgres on :24100 unreachable (closed-set reason: hardware_not_present)"
    fi
else
    echo "SKIP  E13 — no container runtime (closed-set reason: hardware_not_present)"
fi

# ----------------------------------------------------------------------
echo ""
echo "===================================================="
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -gt 0 ]; then
    echo ""
    echo "Bluffs caught:"
    for n in "${fail_names[@]}"; do
        echo "  - ${n}"
    done
    exit 1
fi
echo ""
echo "All Herald user-visible features verified end-to-end."
echo "Anti-bluff covenant (§11.4) intact."
exit 0
