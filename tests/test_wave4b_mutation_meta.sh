#!/usr/bin/env bash
# tests/test_wave4b_mutation_meta.sh — Paired §1.1 mutation test for Wave 4b
# (TOON content negotiation, E56-E62).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILS when the property it claims to enforce is
# removed. A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107 covenant: this script MUST observe each mutation actually applies
# (grep + assert anchor in mutated file), MUST run the e2e probe and
# observe the targeted invariant FAIL on real wire behaviour, MUST restore
# the original file, MUST verify the restore worked (grep + assert
# original content present + post-flight e2e returns to baseline). A
# mutation gate that doesn't actually bite is itself a bluff.
#
# CRITICAL — working-tree quiescence check (lesson from commit 72e81ab):
# After every mutation/restore cycle, the script greps its OWN working tree
# for residual 'MUTATED' markers BEFORE declaring PASS. If a marker survives
# a restore, the script ABORTS — preventing accidental commit of mutation
# residue. The 2026-05-22 security incident captured a JWT-bypass mutation
# in a sibling commit because no such check existed.
#
# Wave 4b covers three mutations + one post-flight:
#
#   M1. Strip TOONMiddleware auto-wire from commons/cli/serve.go →
#       E57 MUST FAIL (response stays JSON despite Accept: application/toon
#       because the middleware that does the transcode is absent from the
#       chain).
#   M2. Mutate commons/cli/contentnego.go::MarshalChosen so the TOON branch
#       always falls back to json.Marshal (the EXACT shape of the 2026-05-17
#       PASS-bluff) → E60 MUST FAIL (body starts with '{' despite
#       Content-Type: application/toon — the wire-level bluff signature).
#   M3. Mutate pherald/internal/http/events.go::transcodeRequestBody to
#       no-op (don't transcode TOON request bodies) → the pherald events
#       handler returns 400 "event_parser: malformed JSON" on TOON request
#       bodies. Because the e2e battery doesn't directly hit pherald
#       /v1/events with TOON bodies (E37-E42 are PG-gated SKIPs by default
#       on a fresh shell), M3 is observed via the unit-test layer instead:
#       the pherald handler tests TestEventsHandler_ContentTypeTOON_RequestDecoded
#       MUST FAIL when transcodeRequestBody is no-op'd. SKIP-with-reason
#       documents the e2e provocation gap explicitly.
#
# Hardlink-backup-restore pattern lifted from test_wave4_mutation_meta.sh.
# Post-flight verifies the full e2e battery is still green after all
# mutations are restored AND the working-tree quiescence check passes.
#
# Returns 0 only when every expected mutation causes the targeted invariant
# to FAIL AND the post-flight passes AND no MUTATED markers leaked. Non-zero
# on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
E2E="${REPO_ROOT}/scripts/e2e_bluff_hunt.sh"

pass=0
fail=0

file_backup() { cp "$1" "$1.w4bmeta-backup"; }
restore() {
    if [ -f "$1.w4bmeta-backup" ]; then
        cat "$1.w4bmeta-backup" > "$1"
        rm -f "$1.w4bmeta-backup"
    fi
}

SERVE_GO="${REPO_ROOT}/commons/cli/serve.go"
CONTENTNEGO_GO="${REPO_ROOT}/commons/cli/contentnego.go"
EVENTS_GO="${REPO_ROOT}/pherald/internal/http/events.go"

cleanup_all() {
    restore "${SERVE_GO}"
    restore "${CONTENTNEGO_GO}"
    restore "${EVENTS_GO}"
    # 24793 (W4A_PORT) + 24794 (W4B_PORT) — kill any orphans.
    for port in 24793 24794; do
        lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
    done
}
trap cleanup_all EXIT

# Pre-flight: kill any orphan listeners on test ports.
for port in 24793 24794; do
    lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
done

# Working-tree quiescence guard. Returns 0 if NO MUTATED markers in tracked
# source files; non-zero (with name list) if a marker leaked. The check
# scopes to the three known mutation target files — broader greps would
# match legitimate prose ("MUTATED" in comments documenting the gate
# itself, e.g., in test_wave4_mutation_meta.sh or this file).
check_quiescence() {
    local label="$1"
    local leaks=0
    for f in "${SERVE_GO}" "${CONTENTNEGO_GO}" "${EVENTS_GO}"; do
        if grep -qE 'MUTATED W4B-M[0-9]+' "${f}" 2>/dev/null; then
            echo "ABORT  ${label}: MUTATED marker LEAKED in $(basename "${f}") — restore failed!"
            leaks=$((leaks+1))
        fi
    done
    return ${leaks}
}

# ----------------------------------------------------------------------
# M1: Strip TOONMiddleware auto-wire from commons/cli/serve.go RunServe.
# Targeted invariant: E57 (Accept: application/toon → Content-Type:
# application/toon + body[0]!='{'). Without the middleware, the response
# stays JSON; E57 FAILs on both the Content-Type assertion AND the body[0]
# wire-byte assertion.
# ----------------------------------------------------------------------
echo "== M1: strip TOONMiddleware auto-wire from commons/cli/serve.go =="
file_backup "${SERVE_GO}"
perl -i -pe 's|^\tr\.Use\(TOONMiddleware\(\)\)|\t// MUTATED W4B-M1: TOONMiddleware auto-wire removed\n\t_ = TOONMiddleware|' "${SERVE_GO}"
if ! grep -q "MUTATED W4B-M1" "${SERVE_GO}"; then
    echo "FAIL  M1: perl mutation failed — anchor 'r.Use(TOONMiddleware())' not found in serve.go"
    fail=$((fail+1))
elif grep -qE '^\tr\.Use\(TOONMiddleware\(\)\)' "${SERVE_GO}"; then
    echo "FAIL  M1: mutation did not replace the original r.Use(TOONMiddleware()) call (still present)"
    fail=$((fail+1))
else
    bash "${E2E}" > /tmp/w4bmeta-m1.log 2>&1 || true
    if grep -qE "^FAIL  E57" /tmp/w4bmeta-m1.log; then
        echo "PASS  M1: stripping TOONMiddleware breaks E57 (gate proven — Accept:application/toon ignored, response stays JSON)"
        pass=$((pass+1))
    else
        echo "FAIL  M1: stripping TOONMiddleware did NOT break E57 — gate is a bluff"
        echo "      (last 20 lines of e2e output)"
        tail -20 /tmp/w4bmeta-m1.log | sed 's/^/      /'
        fail=$((fail+1))
    fi
fi
restore "${SERVE_GO}"

# ----------------------------------------------------------------------
# M2: Mutate MarshalChosen's TOON branch to call json.Marshal — the EXACT
# shape of the 2026-05-17 PASS-bluff (JSON bytes wearing TOON Content-Type).
# Targeted invariant: E60 (TOON response body first byte NOT '{' and
# NOT '['). With the bluff in place, the body becomes JSON bytes starting
# with '{' under a 'Content-Type: application/toon' header — E60 catches
# the mismatch.
# ----------------------------------------------------------------------
echo ""
echo "== M2: MarshalChosen TOON branch swapped to json.Marshal (recurrence of the 2026-05-17 PASS-bluff) =="
file_backup "${CONTENTNEGO_GO}"
# Replace the TOON case body in MarshalChosen with a json.Marshal fallback.
perl -i -0pe 's|case MediaTypeTOON:\n\t\tb, err := toon\.Marshal\(v\)\n\t\tif err != nil \{\n\t\t\treturn nil, fmt\.Errorf\("contentnego: toon marshal: %w", err\)\n\t\t\}\n\t\treturn b, nil|case MediaTypeTOON:\n\t\t// MUTATED W4B-M2: silently delegate to json.Marshal (2026-05-17 PASS-bluff recurrence)\n\t\tb, err := json.Marshal(v)\n\t\tif err != nil {\n\t\t\treturn nil, fmt.Errorf("contentnego: toon marshal: %w", err)\n\t\t}\n\t\treturn b, nil|' "${CONTENTNEGO_GO}"
if ! grep -q "MUTATED W4B-M2" "${CONTENTNEGO_GO}"; then
    echo "FAIL  M2: perl mutation failed — anchor 'case MediaTypeTOON: ... toon.Marshal' not found in contentnego.go"
    fail=$((fail+1))
else
    bash "${E2E}" > /tmp/w4bmeta-m2.log 2>&1 || true
    if grep -qE "^FAIL  E60" /tmp/w4bmeta-m2.log; then
        echo "PASS  M2: swapping toon.Marshal → json.Marshal breaks E60 (gate proven — JSON-bytes-wearing-TOON-Content-Type bluff caught at wire byte 0)"
        pass=$((pass+1))
    else
        echo "FAIL  M2: TOON→JSON marshal swap did NOT break E60 — gate is a bluff (the 2026-05-17 PASS-bluff signature was NOT detected!)"
        echo "      (last 25 lines of e2e output)"
        tail -25 /tmp/w4bmeta-m2.log | sed 's/^/      /'
        fail=$((fail+1))
    fi
fi
restore "${CONTENTNEGO_GO}"

# ----------------------------------------------------------------------
# M3: Mutate transcodeRequestBody to no-op (don't transcode TOON request
# bodies). Targeted invariant: pherald handler test
# TestEventsHandler_ContentTypeTOON_RequestDecoded (the e2e doesn't directly hit
# /v1/events with TOON bodies on a fresh shell — E37-E42 SKIP without PG).
# Observed via `go test ./pherald/internal/http/`.
# ----------------------------------------------------------------------
echo ""
echo "== M3: transcodeRequestBody no-op (no TOON→JSON request transcode) =="
file_backup "${EVENTS_GO}"
# Replace the transcodeRequestBody body so it always returns the body
# unchanged regardless of Content-Type. Keep the function signature intact.
perl -i -0pe 's|func transcodeRequestBody\(c \*gin\.Context, body \[\]byte\) \(\[\]byte, error\) \{\n\tif len\(body\) == 0 \{\n\t\treturn body, nil\n\t\}|func transcodeRequestBody(c *gin.Context, body []byte) ([]byte, error) {\n\t// MUTATED W4B-M3: no-op transcode (TOON request bodies pass through unchanged)\n\treturn body, nil\n\tif len(body) == 0 {\n\t\treturn body, nil\n\t}|' "${EVENTS_GO}"
if ! grep -q "MUTATED W4B-M3" "${EVENTS_GO}"; then
    echo "FAIL  M3: perl mutation failed — anchor 'func transcodeRequestBody' not found in events.go"
    fail=$((fail+1))
else
    # Drive via the pherald handler unit test, not the e2e (e2e E37-E42
    # SKIPs on a fresh shell without PG). The handler test directly POSTs
    # a TOON CloudEvent body; without transcoding it surfaces as
    # event_parser: malformed JSON → 400.
    if go test -count=1 -run 'TestEventsHandler_ContentTypeTOON_RequestDecoded' ./pherald/internal/http/ > /tmp/w4bmeta-m3.log 2>&1; then
        echo "FAIL  M3: no-op'ing transcodeRequestBody did NOT break TestEventsHandler_ContentTypeTOON_RequestDecoded — gate is a bluff"
        tail -20 /tmp/w4bmeta-m3.log | sed 's/^/      /'
        fail=$((fail+1))
    else
        echo "PASS  M3: no-op'ing transcodeRequestBody breaks TestEventsHandler_ContentTypeTOON_RequestDecoded (gate proven — Runner's EventParser rejects raw TOON bytes)"
        pass=$((pass+1))
    fi
fi
restore "${EVENTS_GO}"

# ----------------------------------------------------------------------
# Working-tree quiescence — assert no MUTATED markers leaked.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED markers =="
if check_quiescence "post-restore quiescence"; then
    echo "PASS  Quiescence: no MUTATED W4B-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: at least one MUTATED W4B-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Post-flight: the full e2e battery must still be green.
# ----------------------------------------------------------------------
echo ""
echo "== Post-flight: full e2e green after restores =="
bash "${E2E}" > /tmp/w4bmeta-postflight.log 2>&1
postflight_ec=$?
if [ "${postflight_ec}" = 0 ] && grep -q "All Herald user-visible features verified" /tmp/w4bmeta-postflight.log; then
    echo "PASS  post-flight (e2e battery green after restores)"
    pass=$((pass+1))
else
    echo "FAIL  post-flight: e2e battery still has FAILs after restores (ec=${postflight_ec})"
    tail -15 /tmp/w4bmeta-postflight.log | sed 's/^/      /'
    fail=$((fail+1))
fi

echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "WAVE 4B META-TEST PASS"
else
    echo "WAVE 4B META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
