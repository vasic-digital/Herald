#!/usr/bin/env bash
# tests/test_hrd156_mutation_meta.sh — Paired §1.1 mutation test for HRD-156
# (workable-items → notify outbound flow: commons_workable.Diff →
# workflow.Notifier → runner.ChannelDispatcher → commons.Channel.Send).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILs when the property it claims to enforce is removed.
# A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107.y working-tree quiescence: writes .git/MUTATION_IN_PROGRESS on entry,
# removes it via trap-on-exit, refuses to start if the lockfile is present or
# any target file is dirty. Trap restores every mutated file even on early-exit.
# FOREGROUND-ONLY (operator mandate feedback_no_concurrent_mutation_gates):
# never run while any other process touches the checkout.
#
# Hermetic detectors only — all three detector tests run against in-process
# fakes / temp SQLite (no Postgres, no Telegram, no network); deterministic.
#
# Three mutations across two source files:
#
#   M1 (workflow.go). Disable the receipts-failure check in Notifier.Notify
#      (`if r.Error != "" || r.Evidence == commons.DeliveryUnknown {` → `if false {`)
#      → the Notifier silently swallows undelivered notifications →
#      TestNotifier_SurfacesSendFailure (faultingChannel) FAILs.
#   M2 (changefeed.go). Break relocation detection (replace the
#      detectRelocations call with an empty map) → a relocated item degrades
#      into spurious delete+create → TestDiff_Relocated_IssuesToFixed FAILs.
#   M3 (workflow.go). Break RenderChange's KindStatusChanged case (drop the
#      "→ new" half) → the rendered status diff is wrong →
#      TestRenderChange/status FAILs.
#
# Returns 0 only when every mutation causes its detector to FAIL AND the file
# returns to HEAD byte-for-byte after restore AND no MUTATED HRD156-M* markers
# leaked into either target file.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

WF_GO="${REPO_ROOT}/pherald/internal/workflow/workflow.go"
CF_GO="${REPO_ROOT}/commons_workable/changefeed.go"
TARGETS=("${WF_GO}" "${CF_GO}")

DET_WF_FAILSURFACE="go test -count=1 -run 'TestNotifier_SurfacesSendFailure' ./pherald/internal/workflow/..."
DET_WF_RENDER="go test -count=1 -run 'TestRenderChange/status' ./pherald/internal/workflow/..."
DET_CF_RELOCATE="go test -count=1 -run 'TestDiff_Relocated_IssuesToFixed' ./commons_workable/..."

# §107.y pre-flight: working-tree quiescence guard — every target must be clean.
for f in "${TARGETS[@]}"; do
    if git diff --quiet -- "${f}" && git diff --cached --quiet -- "${f}"; then : ; else
        echo "FAIL: ${f} dirty before mutation gate; abort per §107.y"
        git status --short -- "${f}"
        exit 1
    fi
done
if [ -f .git/MUTATION_IN_PROGRESS ]; then
    echo "FAIL: another mutation gate already in flight (.git/MUTATION_IN_PROGRESS present)"
    exit 1
fi
for f in "${TARGETS[@]}"; do
    if grep -qE 'MUTATED HRD156-M' "${f}" 2>/dev/null; then
        echo "FAIL: ${f} carries a pre-existing MUTATED HRD156-M marker"
        exit 1
    fi
done

touch .git/MUTATION_IN_PROGRESS
cleanup_all() {
    echo "[hrd156-mutation] cleanup: restoring targets + clearing lockfile"
    git checkout -- "${WF_GO}" "${CF_GO}" 2>/dev/null || true
    rm -f .git/MUTATION_IN_PROGRESS
}
trap cleanup_all EXIT

assert_restored() {
    local target="$1"
    local rel="${target#${REPO_ROOT}/}"
    if diff -q <(git show "HEAD:${rel}") "${target}" >/dev/null 2>&1; then
        return 0
    fi
    echo "FAIL: ${rel} does NOT match HEAD byte-for-byte after restore (§107.y residue!)"
    return 1
}

# run_paired NAME TARGET_FILE MUTATE_CMD ANCHOR DETECTOR_CMD
run_paired() {
    local name="$1" target="$2" mutate_cmd="$3" anchor="$4" detector="$5"
    echo ""
    echo "== ${name} =="
    echo "  → applying mutation to ${target#${REPO_ROOT}/}"
    eval "${mutate_cmd}"
    if ! grep -qE "${anchor}" "${target}"; then
        echo "FAIL  ${name}: mutation did not apply — anchor ${anchor} not found"
        git checkout -- "${target}" 2>/dev/null || true
        fail=$((fail+1)); return 1
    fi
    echo "  → mutation marker present; running detector (expect FAIL)"
    if eval "${detector}" > "/tmp/hrd156meta-${name}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        tail -15 "/tmp/hrd156meta-${name}.log" | sed 's/^/      /'
        git checkout -- "${target}" 2>/dev/null || true
        fail=$((fail+1)); return 1
    fi
    echo "  ✓ detector FAILed as expected on mutated build"
    git checkout -- "${target}" 2>/dev/null
    if ! assert_restored "${target}"; then fail=$((fail+1)); return 1; fi
    echo "  → post-restore detector (expect PASS)"
    if ! eval "${detector}" > "/tmp/hrd156meta-${name}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        tail -15 "/tmp/hrd156meta-${name}-restored.log" | sed 's/^/      /'
        fail=$((fail+1)); return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# M1 — disable the receipts-failure surface in Notifier.Notify.
run_paired \
    "M1-no-receipt-surface" \
    "${WF_GO}" \
    "perl -i -pe 's|if r\.Error != \"\" \|\| r\.Evidence == commons\.DeliveryUnknown \{|if false { // MUTATED HRD156-M1: receipts-failure check disabled|' '${WF_GO}'" \
    'MUTATED HRD156-M1' \
    "${DET_WF_FAILSURFACE}"

# M2 — break relocation detection (force an empty relocation map).
run_paired \
    "M2-no-relocation" \
    "${CF_GO}" \
    "perl -i -pe 's|relocated := detectRelocations\(prevByKey, currByKey\)|relocated := map[string]relocation{} // MUTATED HRD156-M2: detectRelocations disabled\n\t_ = detectRelocations|' '${CF_GO}'" \
    'MUTATED HRD156-M2' \
    "${DET_CF_RELOCATE}"

# M3 — break RenderChange's KindStatusChanged case (drop the "→ new" half).
run_paired \
    "M3-render-status-broken" \
    "${WF_GO}" \
    "perl -i -pe 's|return \"🔄 \" \+ c\.AtmID \+ \" status: \" \+ c\.Old \+ \" → \" \+ c\.New|return \"🔄 \" + c.AtmID + \" status: \" + c.Old /* MUTATED HRD156-M3: dropped → new */|' '${WF_GO}'" \
    'MUTATED HRD156-M3' \
    "${DET_WF_RENDER}"

echo ""
echo "== Quiescence: working tree free of MUTATED HRD156-M* markers =="
leaked=0
for f in "${TARGETS[@]}"; do
    if grep -qE 'MUTATED HRD156-M[0-9]' "${f}" 2>/dev/null; then
        echo "FAIL  Quiescence: a MUTATED HRD156-M marker survived restore in ${f#${REPO_ROOT}/} — DO NOT COMMIT"
        leaked=1
    fi
done
if [ "${leaked}" -eq 0 ]; then
    echo "PASS  Quiescence: no MUTATED HRD156-M markers leaked"
    pass=$((pass+1))
else
    fail=$((fail+1))
fi

echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: hrd156 mutation gate (3 paired + quiescence)"
else
    echo "HRD-156 META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
