#!/usr/bin/env bash
# tests/test_wave3_mutation_meta.sh — Paired §1.1 mutation test for Wave 3
# invariants — 3a fragment.
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILS when the property it claims to enforce is
# removed. A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# Wave 3a covers three mutations (M2/M3/M4 land in Wave 3b alongside
# pherald Runner):
#
#   M1. Strip JWT verification from commons_auth/middleware.go → E35 MUST FAIL.
#   M5. Mutate sherald Aggregator.Snapshot to return zero mem → E47 MUST FAIL.
#   M6. SKIP-with-reason — cross-binary cherald compliance integration
#       requires Runner; deferred to Wave 3b (paired with E45 SKIP).
#
# Hardlink-backup-restore pattern lifted from tests/test_i8_usability_meta.sh.
# Post-flight verifies the full e2e battery is still green after restores.
#
# Returns 0 only when both expected mutations cause the gate to FAIL on
# the expected invariant AND the post-flight passes. Non-zero on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
E2E="${REPO_ROOT}/scripts/e2e_bluff_hunt.sh"

pass=0
fail=0

# Backup uses cp (NOT ln) because the mutations below use `cat > file`
# heredoc which truncates the inode — a hardlink would be truncated too.
# Restore uses `cat backup > file` to preserve the inode while overwriting
# contents, which works regardless of how the file got mutated.
file_backup() { cp "$1" "$1.w3meta-backup"; }
restore() {
    if [ -f "$1.w3meta-backup" ]; then
        cat "$1.w3meta-backup" > "$1"
        rm -f "$1.w3meta-backup"
    fi
}

# Safety net — if the test trips, restore every backup we know about and
# kill any orphan flavor processes left binding the test ports (24791..24994).
# Orphans accumulate if a previous e2e was SIGKILL'd; they'd carry the
# mutated binary into the next run and produce a false PASS-bluff.
cleanup_all() {
    restore "${REPO_ROOT}/commons_auth/middleware.go"
    restore "${REPO_ROOT}/sherald/internal/safety/aggregator.go"
    restore "${REPO_ROOT}/cherald/internal/compliance/handler.go"
    for port in 24791 24992 24993 24994; do
        lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
    done
}
trap cleanup_all EXIT

# Pre-flight: kill any orphan flavor processes from a previous interrupted run.
for port in 24791 24992 24993 24994; do
    lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
done

# ----------------------------------------------------------------------
# M1: strip JWT verification from commons_auth/middleware.go.
# Replace the Verify call with a no-op that ALWAYS succeeds. With the
# mutation in place, E35(cherald) + E35(sherald) MUST FAIL (no-auth
# requests must continue to be 401-blocked; the mutation makes them 200).
# ----------------------------------------------------------------------
MIDDLEWARE="${REPO_ROOT}/commons_auth/middleware.go"
echo "== M1: strip JWT verification from commons_auth/middleware.go =="
file_backup "${MIDDLEWARE}"
# Replace the entire GinMiddleware body with a no-op that always calls Next.
# This is a structural mutation (not just sed substring replace) so it survives
# whitespace/quoting drift.
cat > "${MIDDLEWARE}" <<'EOF'
package commons_auth

import (
	"github.com/gin-gonic/gin"
)

const ContextKeyClaims = "herald.auth.claims"

// MUTATED for paired §1.1 anti-bluff test (test_wave3_mutation_meta.sh M1).
// This bypasses Verifier entirely. If you see this in production, the
// gate failed and the test is itself a bluff.
func GinMiddleware(v Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ContextKeyClaims, map[string]any{"tenant": "00000000-0000-0000-0000-000000000000"})
		c.Next()
	}
}
EOF
# Capture e2e output to a file BEFORE the grep — pipefail would otherwise
# clobber the pipeline exit code with the e2e's non-zero (caused by the
# mutation), masking the grep's actual match status.
bash "${E2E}" > /tmp/w3meta-m1.log 2>&1 || true
if grep -qE "FAIL  E35" /tmp/w3meta-m1.log; then
    echo "PASS  M1: stripping verify breaks E35 (gate proven)"
    pass=$((pass+1))
else
    echo "FAIL  M1: stripping verify did NOT break E35 — gate is a bluff"
    fail=$((fail+1))
fi
restore "${MIDDLEWARE}"

# ----------------------------------------------------------------------
# M5: mutate sherald Aggregator.Snapshot to return zero memory.
# Force CurrentMemPercent: 0 in the return. E47 asserts current_mem_percent
# > 0, so the mutation MUST cause E47 to FAIL.
# ----------------------------------------------------------------------
AGG="${REPO_ROOT}/sherald/internal/safety/aggregator.go"
echo "== M5: mutate sherald Aggregator.Snapshot to return zero mem =="
file_backup "${AGG}"
sed -i.tmp 's|CurrentMemPercent: a.lastMemPercent,|CurrentMemPercent: 0,|' "${AGG}"
rm -f "${AGG}.tmp"
if ! grep -q "CurrentMemPercent: 0," "${AGG}"; then
    echo "FAIL  M5: sed mutation failed — anchor not present in aggregator.go"
    fail=$((fail+1))
else
    bash "${E2E}" > /tmp/w3meta-m5.log 2>&1 || true
    if grep -qE "FAIL  E47" /tmp/w3meta-m5.log; then
        echo "PASS  M5: zero mem breaks E47 (gate proven)"
        pass=$((pass+1))
    else
        echo "FAIL  M5: zero mem did NOT break E47 — gate is a bluff"
        fail=$((fail+1))
    fi
fi
restore "${AGG}"

# ----------------------------------------------------------------------
# M6: cherald compliance cross-binary integration mutation.
# Deferred to Wave 3b alongside pherald Runner (the e2e invariant E45 it
# would target is currently SKIP for the same reason). SKIP-with-reason
# per §11.4.3 — explicit, not a bluff.
# ----------------------------------------------------------------------
echo "== M6: cherald compliance cross-binary mutation =="
echo "SKIP  M6 (cross-binary integration — runs in Wave 3b alongside pherald Runner)"

# ----------------------------------------------------------------------
# Post-flight: the full e2e battery must still be green after every
# mutation has been restored. A bluffing restore (where the mutation
# persisted) would surface here.
# ----------------------------------------------------------------------
echo "== Post-flight: full e2e green after restores =="
bash "${E2E}" > /tmp/w3meta-postflight.log 2>&1
postflight_ec=$?
if [ "${postflight_ec}" = 0 ] && grep -q "All Herald user-visible features verified" /tmp/w3meta-postflight.log; then
    echo "PASS  post-flight (e2e battery green after restores)"
    pass=$((pass+1))
else
    echo "FAIL  post-flight: e2e battery still has FAILs after restores (ec=${postflight_ec})"
    tail -10 /tmp/w3meta-postflight.log | sed 's/^/      /'
    fail=$((fail+1))
fi

echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
[ "${fail}" -eq 0 ]
