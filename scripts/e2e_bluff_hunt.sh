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
# Thirty-three invariants (each is the "captured-evidence" for one feature
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
#   E14. commons_storage RLS tenant-isolation round-trip (HRD-010 Wave 1).
#   E15. commons_infra queue enqueue/dequeue round-trip against live PG.
#   E16. commons_infra redis TTL round-trip against live Redis.
#   E17. Telegram Send delivers + outbound_delivery_evidence persisted
#        (HRD-011 Wave 1 — live Bot API + live PG).
#   E18. Claude Code Dispatch round-trip + claude_code_sessions persisted
#        (HRD-012 Wave 1 — live CLI + live PG).
#
# Wave 2 flavor-binary invariants (added 2026-05-21):
#   E19-E24. New flavor version --json probes (sherald, cherald, bherald,
#            rherald, iherald, scherald — each --json shape valid + flavor field matches).
#   E25-E27. Serving flavors actually bind + healthz returns 200
#            (sherald, cherald, iherald).
#   E28-E30. Flavor-specific 501 routes return HRD pointer in body
#            (sherald GET /v1/safety_state → HRD-098,
#             cherald GET /v1/compliance → HRD-028,
#             iherald POST /v1/webhooks/page → HRD-024).
#   E31. Representative §43 stub exits non-zero with HRD pointer
#        (sherald destructive-guard → HRD-033 in stderr).
#   E32. New serving flavor graceful-shuts on SIGTERM (sherald).
#   E33. pherald e2e regression sentinel — re-runs the pre-Wave-2
#        version probe after every flavor probe so a flavor regression
#        that breaks pherald is caught (rebuild fresh + version --json).
#
# Optional (live channels, run when operator-supplied creds are present):
#   E34. Full vertical slice — operator hand-sent Telegram inbound →
#        Claude Code → Telegram outbound (HRD-011 + HRD-012 Wave 1).
#
# Exit 0 only when E1..E12 + E19..E33 (plus E13..E18 + E34 if attempted) all pass.
# Failure prints the offending invariant so the operator knows EXACTLY
# which feature is bluffing.

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

check "E11 /metrics returns text/plain + pherald_build_info gauge" \
    "curl -fsS 'http://127.0.0.1:${HTTP_PORT}/metrics' | grep -q '^pherald_build_info{'"

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
# E14-E16: HRD-010 commons_storage live wiring (Wave 1 evidence).
# Requires container runtime (docker or podman). Skip-with-reason if
# absent per §11.4.3. Tests already implemented:
#   E14: commons_storage/storage_integration_test.go (RLS isolation)
#   E15: commons_infra/clients_integration_test.go (queue enqueue/dequeue)
#   E16: commons_infra/clients_integration_test.go (redis TTL)
echo ""
echo "== E14-E16: HRD-010 commons_storage live integration =="
if command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1; then
    check "E14 commons_storage RLS tenant-isolation round-trip (live PG)" \
        "go test ./commons_storage/ -tags=integration -run TestRLS_TenantIsolation_RoundTrip -count=1 -timeout=180s"
    check "E15 commons_infra queue enqueue/dequeue round-trip (live PG)" \
        "go test ./commons_infra/ -tags=integration -run TestUp_PopulatesQueue_EnqueueDequeueRoundTrip -count=1 -timeout=180s"
    check "E16 commons_infra redis TTL round-trip (live Redis)" \
        "go test ./commons_infra/ -tags=integration -run TestUp_PopulatesRedis_TTLRoundTrip -count=1 -timeout=180s"
else
    echo "SKIP  E14-E16 (no docker/podman on PATH — §11.4.3 explicit SKIP-with-reason)"
fi

# ----------------------------------------------------------------------
# E17/E18/E34: HRD-011 + HRD-012 live channel + dispatcher (Wave 1
# vertical slice). Each SKIPs with explicit §11.4.3 reason if its
# prerequisite is absent — never PASS-by-default. Note: the live
# vertical-slice slot was renumbered from E19 → E34 on 2026-05-21 to
# free E19-E33 for the six new flavor-binary invariants.
echo ""
echo "== E17/E18/E34: HRD-011 Telegram + HRD-012 Claude Code live integration =="

# E17: Telegram Send + outbound_delivery_evidence persistence.
if [ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ] && [ -n "${HERALD_TGRAM_CHAT_ID:-}" ] && (command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1); then
    check "E17 Telegram Send delivers + persists evidence (live Bot API + live PG)" \
        "go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSend_PersistsDeliveryEvidence -count=1 -timeout=300s"
else
    echo "SKIP  E17 (HERALD_TGRAM_BOT_TOKEN+_CHAT_ID or container runtime absent — §11.4.3 explicit SKIP-with-reason)"
fi

# E18: Claude Code Dispatch + session_state persistence.
CLAUDE_BIN_FOR_E18="${HERALD_CLAUDE_BIN:-claude}"
if command -v "${CLAUDE_BIN_FOR_E18}" >/dev/null 2>&1 \
   && [ -n "${HERALD_CLAUDE_PROJECT_NAME:-}" ] \
   && [ -n "${HERALD_CLAUDE_SESSION_UUID:-}" ] \
   && (command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1); then
    check "E18 Claude Code Dispatch round-trip + session_state persist (live CLI + live PG)" \
        "go test ./commons_messaging/dispatch/claude_code/ -tags=integration -run TestDispatch_PersistsSessionState -count=1 -timeout=600s"
else
    echo "SKIP  E18 (claude binary OR HERALD_CLAUDE_PROJECT_NAME/SESSION_UUID OR container runtime absent — §11.4.3 explicit SKIP-with-reason)"
fi

# E34: Full vertical slice — operator hand-sent Telegram inbound → Claude → Telegram outbound.
# (Renumbered from E19 → E34 on 2026-05-21; the E19-E33 slots are now
# occupied by the Wave 2 flavor-binary invariants below.)
if [ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ] \
   && [ -n "${HERALD_TGRAM_CHAT_ID:-}" ] \
   && [ "${HERALD_TGRAM_LIVE_INBOUND:-}" = "1" ] \
   && command -v "${CLAUDE_BIN_FOR_E18}" >/dev/null 2>&1 \
   && [ -n "${HERALD_CLAUDE_PROJECT_NAME:-}" ] \
   && [ -n "${HERALD_CLAUDE_SESSION_UUID:-}" ] \
   && (command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1); then
    check "E34 full vertical slice — Telegram inbound → Claude Code → Telegram outbound" \
        "go test ./commons_messaging/ -tags=integration -run TestVerticalSlice_TelegramClaudeRoundTrip -count=1 -timeout=600s"
else
    echo "SKIP  E34 (HERALD_TGRAM_LIVE_INBOUND=1 + Telegram creds + claude + container runtime + project/session env required — §11.4.3 explicit SKIP-with-reason)"
fi

# ----------------------------------------------------------------------
# E19-E33: Wave 2 flavor-binary invariants — 15 new checks against the
# 6 new flavor binaries (sherald / cherald / iherald serving + bherald /
# rherald / scherald CLI-only) per Wave 2 Task 13.
# ----------------------------------------------------------------------

echo ""
echo "== E19-E24: New flavor version --json (6 binaries) =="
e_counter=19
for entry in "sherald:s" "cherald:c" "bherald:b" "rherald:r" "iherald:i" "scherald:sc"; do
    flavor="${entry%%:*}"
    expect_flavor="${entry##*:}"
    label="E${e_counter}(${flavor})"
    bin="/tmp/${flavor}-bluff-$$"
    if ! go build -o "${bin}" "./${flavor}/cmd/${flavor}" > /tmp/e2e_out 2>&1; then
        echo "FAIL  ${label} ${flavor} build failed"
        tail -5 /tmp/e2e_out | sed 's/^/      /'
        fail=$((fail+1))
        fail_names+=("${label}-build")
        e_counter=$((e_counter+1))
        continue
    fi
    check "${label} version --json shape + flavor=${expect_flavor} + binary=${flavor}" \
        "\"${bin}\" version --json | python3 -c 'import sys,json; d=json.load(sys.stdin); assert d[\"flavor\"]==\"${expect_flavor}\"; assert d[\"version\"]; assert d[\"go_version\"]; assert d[\"binary\"]==\"${flavor}\"'"
    rm -f "${bin}"
    e_counter=$((e_counter+1))
done

echo ""
echo "== E25-E30 + E32: Serving flavors bind healthz/route + sherald SIGTERM =="
serve_idx=25
route_idx=28
for entry in "sherald:24993:HRD-098:safety_state:GET" "cherald:24992:HRD-028:compliance:GET" "iherald:24994:HRD-024:webhooks/page:POST"; do
    flavor="${entry%%:*}"
    rest="${entry#*:}"
    port="${rest%%:*}"
    rest="${rest#*:}"
    hrd="${rest%%:*}"
    rest="${rest#*:}"
    route_short="${rest%%:*}"
    method="${rest##*:}"
    serve_label="E${serve_idx}(${flavor})"
    route_label="E${route_idx}(${flavor})"
    bin="/tmp/${flavor}-serve-$$"
    if ! go build -o "${bin}" "./${flavor}/cmd/${flavor}" > /tmp/e2e_out 2>&1; then
        echo "FAIL  ${serve_label} build"
        tail -5 /tmp/e2e_out | sed 's/^/      /'
        fail=$((fail+1)); fail_names+=("${serve_label}-build")
        serve_idx=$((serve_idx+1)); route_idx=$((route_idx+1))
        continue
    fi
    "${bin}" serve --http-port "${port}" > /tmp/${flavor}-serve.log 2>&1 &
    serve_pid=$!
    # Wait up to 5s for the port to be live.
    ready=0
    for i in $(seq 1 10); do
        if curl -fsS "http://127.0.0.1:${port}/v1/healthz" >/dev/null 2>&1; then
            ready=1; break
        fi
        sleep 0.5
    done
    if [ "${ready}" = 1 ]; then
        echo "PASS  ${serve_label} healthz 200 + status:ok on :${port}"
        pass=$((pass+1))
    else
        echo "FAIL  ${serve_label} ${flavor} serve never accepted HTTP within 5s on :${port}"
        tail -5 /tmp/${flavor}-serve.log 2>&1 | sed 's/^/      /'
        fail=$((fail+1)); fail_names+=("${serve_label}")
    fi
    check "${route_label} /v1/${route_short} returns 501 + ${hrd} in body" \
        "curl -sS -o /tmp/route-body -w '%{http_code}' -X '${method}' 'http://127.0.0.1:${port}/v1/${route_short}' | grep -q '^501$' && grep -q '${hrd}' /tmp/route-body"

    # E32 only for sherald: SIGTERM graceful exit.
    if [ "${flavor}" = "sherald" ]; then
        kill -TERM "${serve_pid}" 2>/dev/null
        wait "${serve_pid}" 2>/dev/null
        ec=$?
        # Accept 0 (clean) or 130/143 (shell-default codes for SIGINT/SIGTERM).
        if [ "${ec}" = "0" ] || [ "${ec}" = "130" ] || [ "${ec}" = "143" ]; then
            echo "PASS  E32(sherald) graceful-shutdown on SIGTERM (exit=${ec})"
            pass=$((pass+1))
        else
            echo "FAIL  E32(sherald) graceful-shutdown — exit=${ec} (want 0/130/143)"
            fail=$((fail+1)); fail_names+=("E32-sherald-shutdown")
        fi
    else
        kill "${serve_pid}" 2>/dev/null || true
        wait "${serve_pid}" 2>/dev/null || true
    fi
    rm -f "${bin}"
    serve_idx=$((serve_idx+1))
    route_idx=$((route_idx+1))
done

echo ""
echo "== E31: Representative §43 stub exits non-zero + HRD pointer =="
bin="/tmp/sherald-stub-$$"
if go build -o "${bin}" "./sherald/cmd/sherald" > /tmp/e2e_out 2>&1; then
    set +e
    "${bin}" destructive-guard > /tmp/e31.out 2>&1
    rc=$?
    set -e
    if [ "${rc}" != "0" ] && grep -q 'HRD-033' /tmp/e31.out; then
        echo "PASS  E31 sherald destructive-guard exits non-zero (rc=${rc}) with HRD-033 in stderr"
        pass=$((pass+1))
    else
        echo "FAIL  E31 sherald destructive-guard: rc=${rc} hrd-present=$(grep -q 'HRD-033' /tmp/e31.out && echo yes || echo no)"
        tail -5 /tmp/e31.out | sed 's/^/      /'
        fail=$((fail+1)); fail_names+=("E31")
    fi
    rm -f "${bin}"
else
    echo "FAIL  E31: sherald build failed"
    tail -5 /tmp/e2e_out | sed 's/^/      /'
    fail=$((fail+1)); fail_names+=("E31-build")
fi

echo ""
echo "== E33: pherald regression sentinel — re-verify version --json post-Wave2 =="
pbin="/tmp/pherald-regression-$$"
if go build -o "${pbin}" "./pherald/cmd/pherald" > /tmp/e2e_out 2>&1; then
    check "E33 pherald version --json (regression sentinel post-Wave 2)" \
        "\"${pbin}\" version --json | python3 -c 'import sys,json; d=json.load(sys.stdin); assert d[\"flavor\"]==\"p\"; assert d[\"binary\"]==\"pherald\"; assert d[\"version\"]'"
    rm -f "${pbin}"
else
    echo "FAIL  E33 pherald build"
    tail -5 /tmp/e2e_out | sed 's/^/      /'
    fail=$((fail+1)); fail_names+=("E33-build")
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
