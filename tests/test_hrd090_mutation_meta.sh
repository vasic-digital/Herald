#!/usr/bin/env bash
# tests/test_hrd090_mutation_meta.sh — Paired §1.1 mutation test for HRD-090
# (commons_infra.pgxTaskRepository.MoveToDeadLetter dead-letter move + emit).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILs when the property it claims to enforce is removed.
# A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107.y working-tree quiescence: writes .git/MUTATION_IN_PROGRESS on entry,
# removes it via trap-on-exit, refuses to start if the lockfile is present or
# the target file is dirty. Trap restores the mutated file even on early-exit.
# FOREGROUND-ONLY (operator mandate feedback_no_concurrent_mutation_gates):
# never run while any other process touches the checkout.
#
# Hermetic detector only — the unit test TestMoveToDeadLetter_AtomicMoveSQLAndEmit
# (recording fake db.Database + recordingEmitter) pins the move's SQL shape +
# emit. No Postgres needed; deterministic.
#
# Three mutations on commons_infra/task_repository.go:
#
#   M1. Drop the dead_letter_tasks INSERT tx.Exec → only 1 tx Exec runs →
#       detector "want 2 tx Execs (INSERT dlq + UPDATE task)" FAILs.
#   M2. Change the terminal status mark 'dead_letter' → 'failed' → the UPDATE
#       SQL no longer contains "dead_letter" → detector "second tx Exec must
#       UPDATE background_tasks status to dead_letter" FAILs.
#   M3. Suppress the .queue.dead_letter emit (`if r.emitter != nil` → `if false`)
#       → 0 emits → detector "want exactly 1 DeadLetter emit, got 0" FAILs.
#
# Returns 0 only when every mutation causes the detector to FAIL AND the
# detector returns to PASS after restore AND no MUTATED HRD090-M* markers leaked.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

TR_GO="${REPO_ROOT}/commons_infra/task_repository.go"
DETECTOR="go test -count=1 -run 'TestMoveToDeadLetter_AtomicMoveSQLAndEmit' ./commons_infra/..."

# §107.y pre-flight: working-tree quiescence guard.
if git diff --quiet -- "${TR_GO}" && git diff --cached --quiet -- "${TR_GO}"; then : ; else
    echo "FAIL: ${TR_GO} dirty before mutation gate; abort per §107.y"
    git status --short -- "${TR_GO}"
    exit 1
fi
if [ -f .git/MUTATION_IN_PROGRESS ]; then
    echo "FAIL: another mutation gate already in flight (.git/MUTATION_IN_PROGRESS present)"
    exit 1
fi
if grep -qE 'MUTATED HRD090-M' "${TR_GO}" 2>/dev/null; then
    echo "FAIL: ${TR_GO} carries a pre-existing MUTATED HRD090-M marker"
    exit 1
fi

touch .git/MUTATION_IN_PROGRESS
cleanup_all() {
    echo "[hrd090-mutation] cleanup: restoring ${TR_GO} + clearing lockfile"
    git checkout -- "${TR_GO}" 2>/dev/null || true
    rm -f .git/MUTATION_IN_PROGRESS
}
trap cleanup_all EXIT

assert_restored() {
    local rel="${TR_GO#${REPO_ROOT}/}"
    if diff -q <(git show "HEAD:${rel}") "${TR_GO}" >/dev/null 2>&1; then
        return 0
    fi
    echo "FAIL: ${rel} does NOT match HEAD byte-for-byte after restore (§107.y residue!)"
    return 1
}

run_paired() {
    local name="$1" mutate_cmd="$2" anchor="$3"
    echo ""
    echo "== ${name} =="
    echo "  → applying mutation"
    eval "${mutate_cmd}"
    if ! grep -qE "${anchor}" "${TR_GO}"; then
        echo "FAIL  ${name}: mutation did not apply — anchor ${anchor} not found"
        git checkout -- "${TR_GO}" 2>/dev/null || true
        fail=$((fail+1)); return 1
    fi
    echo "  → mutation marker present; running detector (expect FAIL)"
    if eval "${DETECTOR}" > "/tmp/hrd090meta-${name}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        tail -15 "/tmp/hrd090meta-${name}.log" | sed 's/^/      /'
        git checkout -- "${TR_GO}" 2>/dev/null || true
        fail=$((fail+1)); return 1
    fi
    echo "  ✓ detector FAILed as expected on mutated build"
    git checkout -- "${TR_GO}" 2>/dev/null
    if ! assert_restored; then fail=$((fail+1)); return 1; fi
    echo "  → post-restore detector (expect PASS)"
    if ! eval "${DETECTOR}" > "/tmp/hrd090meta-${name}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        tail -15 "/tmp/hrd090meta-${name}-restored.log" | sed 's/^/      /'
        fail=$((fail+1)); return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# M1 — drop the dead_letter_tasks INSERT tx.Exec block.
run_paired \
    "M1-drop-dlq-insert" \
    "perl -i -0pe 's|\tif _, err := tx\.Exec\(ctx, insertSQL,\n\t\tuuid\.NewString\(\), task\.ID, string\(snapshot\), reason, task\.RetryCount,\n\t\); err != nil \{\n\t\treturn fmt\.Errorf\(\"commons_infra\.MoveToDeadLetter: insert dlq: %w\", err\)\n\t\}|\t// MUTATED HRD090-M1: dead_letter_tasks INSERT dropped (snapshot lost)\n\t_ = insertSQL|' '${TR_GO}'" \
    'MUTATED HRD090-M1'

# M2 — change the terminal status mark away from 'dead_letter'.
run_paired \
    "M2-status-not-deadletter" \
    "perl -i -pe \"s|SET status = 'dead_letter',|SET status = 'failed', /* MUTATED HRD090-M2 */|\" '${TR_GO}'" \
    'MUTATED HRD090-M2'

# M3 — suppress the .queue.dead_letter emit.
run_paired \
    "M3-suppress-emit" \
    "perl -i -pe 's|\tif r\.emitter != nil \{|\tif false { // MUTATED HRD090-M3: emit suppressed|' '${TR_GO}'" \
    'MUTATED HRD090-M3'

echo ""
echo "== Quiescence: working tree free of MUTATED HRD090-M* markers =="
if grep -qE 'MUTATED HRD090-M[0-9]' "${TR_GO}" 2>/dev/null; then
    echo "FAIL  Quiescence: a MUTATED HRD090-M marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
else
    echo "PASS  Quiescence: no MUTATED HRD090-M markers leaked"
    pass=$((pass+1))
fi

echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: hrd090 mutation gate (3 paired + quiescence)"
else
    echo "HRD-090 META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
