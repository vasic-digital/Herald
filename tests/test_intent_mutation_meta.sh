#!/usr/bin/env bash
# tests/test_intent_mutation_meta.sh — Paired §1.1 mutation test for the
# three-tier intent-recognition layer (docs/design/INTENT_RECOGNITION.md
# §1/§2/§3/§6).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILS when the property it claims to enforce is removed.
# A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107 covenant: this script applies each mutation (grep + assert anchor in the
# mutated file), runs the targeted detector and observes it FAIL on the mutated
# build, restores the original file, verifies the restore is byte-for-byte
# clean (`git show HEAD:<path>`), and re-runs the detector expecting PASS.
#
# §107.y working-tree quiescence (lesson from commit 72e81ab): this gate writes
# .git/MUTATION_IN_PROGRESS on entry (resolved via `git rev-parse
# --git-common-dir` so it works in a `git worktree` checkout too), removes it
# via trap-on-exit, and refuses to start if the lockfile is already present or
# if a target file is dirty. Trap-on-exit restores every mutated file even on
# early exit.
#
# All detectors are hermetic unit tests already load-bearing in
# pherald/internal/inbound (command_recognizer_test.go + clarify_test.go) — no
# creds, no network, no live services.
#
# Two load-bearing mutations:
#
#   M1. TIER 1 confidence guard — neutralize the "no ATM target → decline"
#       guard (`if atmID == "" {` → `if false {`) in command_recognizer.go so a
#       vague, targetless message ("ok go ahead and close it") FALSE-MATCHES a
#       command instead of deferring to the LLM. Detector:
#       TestCommandRecognizer_TruthTable — its conservative negative
#       "vague: close it (no target)" now matches → FAIL.
#
#   M2. TIER 3 @sender tag — drop the @sender tag from the clarify reply body
#       (`body := tag + " " + q` → `body := q`) in dispatcher.go so the clarify
#       reply no longer tags the user. Detector:
#       TestClarify_E2E_TagsSenderAndAsks — asserts the recording-sink body is
#       EXACTLY "@carol <question>"; without the tag it is just "<question>" →
#       FAIL.
#
# Returns 0 only when every mutation causes its detector to FAIL AND every
# detector returns to PASS after restore AND no MUTATED markers leaked.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

RECOGNIZER_GO="${REPO_ROOT}/pherald/internal/inbound/command_recognizer.go"
DISPATCHER_GO="${REPO_ROOT}/pherald/internal/inbound/dispatcher.go"

# Resolve the real git common dir so the lockfile lands in both a normal
# checkout (<root>/.git) and a `git worktree` checkout (where <root>/.git is a
# FILE pointing at the shared gitdir).
GIT_COMMON_DIR="$(git rev-parse --git-common-dir 2>/dev/null)"
case "${GIT_COMMON_DIR}" in
    /*) : ;;
    *)  GIT_COMMON_DIR="${REPO_ROOT}/${GIT_COMMON_DIR}" ;;
esac
LOCKFILE="${GIT_COMMON_DIR}/MUTATION_IN_PROGRESS"

# §107.y pre-flight: refuse to start if a target file is dirty (another change
# in flight → abort to avoid sweeping residue into someone else's commit).
for f in "${RECOGNIZER_GO}" "${DISPATCHER_GO}"; do
    if git diff --quiet -- "${f}" && git diff --cached --quiet -- "${f}"; then : ; else
        echo "FAIL: target file ${f} dirty before mutation gate; abort per §107.y"
        git status --short -- "${f}"
        exit 1
    fi
done

if [ -f "${LOCKFILE}" ]; then
    echo "FAIL: another mutation gate already in flight (${LOCKFILE} present)"
    exit 1
fi

# Pre-flight: refuse to start if a production file already carries residue.
for f in "${RECOGNIZER_GO}" "${DISPATCHER_GO}"; do
    if grep -qE '// always pass|MUTATED for paired|MUTATED IR-' "${f}" 2>/dev/null; then
        echo "FAIL: production file ${f} carries a pre-existing mutation marker"
        exit 1
    fi
done

touch "${LOCKFILE}"

cleanup_all() {
    echo "[intent-mutation] cleanup: restoring mutated files + clearing lockfile"
    git checkout -- "${RECOGNIZER_GO}" "${DISPATCHER_GO}" 2>/dev/null || true
    rm -f "${LOCKFILE}"
}
trap cleanup_all EXIT

# Verify a file matches HEAD byte-for-byte after restore (§107.y).
assert_restored() {
    local path rel
    path="$1"
    rel="${path#${REPO_ROOT}/}"
    if diff -q <(git show "HEAD:${rel}") "${path}" >/dev/null 2>&1; then
        return 0
    fi
    echo "FAIL: ${rel} does NOT match HEAD byte-for-byte after restore (§107.y residue!)"
    diff <(git show "HEAD:${rel}") "${path}" | head -20 | sed 's/^/      /'
    return 1
}

# Quiescence: scan a file for residual MUTATED IR- markers.
check_quiescence() {
    local file="$1" label="$2"
    if grep -qE 'MUTATED IR-M[0-9]+' "${file}" 2>/dev/null; then
        echo "ABORT  ${label}: MUTATED marker LEAKED in $(basename "${file}") — restore failed!"
        return 1
    fi
    return 0
}

# run_paired applies a perl mutation, asserts the marker landed, runs the
# detector and EXPECTS it to FAIL, restores, verifies byte-for-byte clean, and
# re-runs the detector EXPECTING PASS.
#
#   $1 — friendly name (M1/M2 + property)
#   $2 — file to mutate
#   $3 — perl one-liner that applies the mutation (MUST inject MUTATED IR-M<n>)
#   $4 — detector command (go test invocation that pins the property)
#   $5 — anchor regex that MUST be findable in the file post-mutation
run_paired() {
    local name="$1" file="$2" mutate_cmd="$3" detector="$4" anchor="$5"
    echo ""
    echo "== ${name} =="
    echo "  → applying mutation"
    eval "${mutate_cmd}"
    if ! grep -qE "${anchor}" "${file}"; then
        echo "FAIL  ${name}: mutation did not apply — anchor regex ${anchor} not found in $(basename "${file}")"
        git checkout -- "${file}" 2>/dev/null || true
        fail=$((fail+1))
        return 1
    fi
    echo "  → mutation marker present; running detector (expect FAIL)"
    if eval "${detector}" > "/tmp/irmeta-${name// /-}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        echo "      (last 20 lines of detector output)"
        tail -20 "/tmp/irmeta-${name// /-}.log" | sed 's/^/      /'
        git checkout -- "${file}" 2>/dev/null || true
        fail=$((fail+1))
        return 1
    else
        echo "  ✓ detector FAILed as expected on mutated build"
    fi
    echo "  → restoring tree"
    git checkout -- "${file}" 2>/dev/null
    if ! assert_restored "${file}"; then
        fail=$((fail+1))
        return 1
    fi
    echo "  → post-restore detector (expect PASS)"
    if ! eval "${detector}" > "/tmp/irmeta-${name// /-}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        echo "      (last 20 lines of detector output)"
        tail -20 "/tmp/irmeta-${name// /-}-restored.log" | sed 's/^/      /'
        fail=$((fail+1))
        return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# ----------------------------------------------------------------------
# M1: TIER 1 confidence guard. Neutralize "no ATM target → decline" so a vague
#     targetless message false-matches a command. Detector:
#     TestCommandRecognizer_TruthTable (the "vague: close it" negative now
#     matches → FAIL).
# ----------------------------------------------------------------------
run_paired \
    "M1-tier1-confidence-guard" \
    "${RECOGNIZER_GO}" \
    "perl -i -pe 's|\tif atmID == \"\" \{|\tif false { // MUTATED IR-M1: confidence guard removed (vague messages false-match)|' '${RECOGNIZER_GO}'" \
    "go test -run '^TestCommandRecognizer_TruthTable\$' -count=1 ./pherald/internal/inbound/" \
    'MUTATED IR-M1'

# ----------------------------------------------------------------------
# M2: TIER 3 @sender tag. Drop the @sender tag from the clarify reply body.
#     Detector: TestClarify_E2E_TagsSenderAndAsks (body must be
#     "@carol <question>"; without the tag → FAIL).
# ----------------------------------------------------------------------
run_paired \
    "M2-tier3-sender-tag" \
    "${DISPATCHER_GO}" \
    "perl -i -pe 's|\tbody := tag \+ \" \" \+ q|\tbody := q // MUTATED IR-M2: @sender tag dropped from clarify reply|' '${DISPATCHER_GO}'" \
    "go test -run '^TestClarify_E2E_TagsSenderAndAsks\$' -count=1 ./pherald/internal/inbound/" \
    'MUTATED IR-M2'

# ----------------------------------------------------------------------
# §107.y working-tree quiescence — assert no MUTATED IR-M* markers leaked.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED IR-M* markers =="
q_ok=1
check_quiescence "${RECOGNIZER_GO}" "post-restore quiescence (recognizer)" || q_ok=0
check_quiescence "${DISPATCHER_GO}" "post-restore quiescence (dispatcher)" || q_ok=0
if [ "${q_ok}" -eq 1 ]; then
    echo "PASS  Quiescence: no MUTATED IR-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: a MUTATED IR-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Final tally.
# ----------------------------------------------------------------------
echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: intent-recognition mutation gate (2 paired + quiescence)"
else
    echo "INTENT META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
