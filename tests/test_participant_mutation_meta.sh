#!/usr/bin/env bash
# tests/test_participant_mutation_meta.sh — Paired §1.1 mutation test for the
# participant identity / notification-tagging layer
# (docs/design/PARTICIPANT_ATTRIBUTION.md §1/§3/§6).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILS when the property it claims to enforce is removed.
# A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107 covenant: this script observes each mutation actually applies (grep +
# assert anchor in the mutated file), runs the targeted detector and observes
# it FAIL on the mutated build, restores the original file, verifies the
# restore worked (byte-for-byte match against `git show HEAD:<path>`), and
# re-runs the detector expecting PASS. A mutation gate that doesn't actually
# bite is itself a bluff.
#
# §107.y working-tree quiescence (lesson from commit 72e81ab — the 2026-05-21
# JWT-bypass mutation residue incident): this gate writes
# .git/MUTATION_IN_PROGRESS on entry, removes it via trap-on-exit, and refuses
# to start if the lockfile is already present (another gate in flight) or if
# the working tree is dirty. Trap-on-exit restores every mutated file even on
# early-exit.
#
# All detectors are hermetic unit tests already load-bearing in
# commons/participant_test.go — no creds, no network, no live services.
#
# The participant/tagging layer covers three load-bearing mutations, all in
# the §3 tagging matrix (commons/tagging.go):
#
#   M1. Remove the operator-skip guard (`if handle == operatorHandle`):
#       the operator would then be @-tagged (self-ping) — exactly the leak the
#       §3 matrix forbids. Detector: TestMentionsFor_MutationWouldFail asserts
#       the operator NEVER appears even when both fields are the operator; with
#       the mutation the operator leaks → FAIL. The stress/chaos adversarial
#       suite (TestStressChaos_MentionsFor_AdversarialInputs) also catches it.
#
#   M2. Remove the SystemAgentHandle ("Claude") skip from the first guard:
#       Claude would then be tagged. Detector: TestStressChaos_SkipGuards_Isolated
#       gives Claude a REAL tgram alias so the alias filter cannot mask the
#       missing skip guard; with the mutation the aliased Claude leaks → FAIL.
#
#   M3. Drop the `add(assignedTo)` call: the assignee branch is removed so a
#       human assignee is never tagged. Detector:
#       TestMentionsFor_MutationWouldFail asserts the assignee IS tagged when
#       it is a non-operator human; with the mutation it is dropped → FAIL.
#
# Returns 0 only when every mutation causes its detector to FAIL AND every
# detector returns to PASS after restore AND no MUTATED markers leaked.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

TAGGING_GO="${REPO_ROOT}/commons/tagging.go"

# Resolve the real git common dir. In a normal checkout this is `<root>/.git`;
# in a `git worktree` checkout `<root>/.git` is a FILE pointing at the shared
# gitdir, so `touch <root>/.git/<lock>` fails with "Not a directory". Using
# `git rev-parse --git-common-dir` makes the lockfile land in both topologies.
GIT_COMMON_DIR="$(git rev-parse --git-common-dir 2>/dev/null)"
case "${GIT_COMMON_DIR}" in
    /*) : ;; # already absolute
    *)  GIT_COMMON_DIR="${REPO_ROOT}/${GIT_COMMON_DIR}" ;;
esac
LOCKFILE="${GIT_COMMON_DIR}/MUTATION_IN_PROGRESS"

# §107.y pre-flight: working-tree quiescence guard (only the target file matters
# for restore safety, but a globally-dirty tree means another change is in
# flight — abort to avoid sweeping residue into someone else's commit).
if git diff --quiet -- "${TAGGING_GO}" && git diff --cached --quiet -- "${TAGGING_GO}"; then : ; else
    echo "FAIL: target file ${TAGGING_GO} dirty before mutation gate; abort per §107.y"
    git status --short -- "${TAGGING_GO}"
    exit 1
fi

if [ -f "${LOCKFILE}" ]; then
    echo "FAIL: another mutation gate already in flight (${LOCKFILE} present)"
    exit 1
fi

# Pre-flight: refuse to start if the production file already carries residue.
if grep -qE '// always pass|MUTATED for paired|MUTATED PA-' "${TAGGING_GO}" 2>/dev/null; then
    echo "FAIL: production file ${TAGGING_GO} carries a pre-existing mutation marker"
    exit 1
fi

touch "${LOCKFILE}"

cleanup_all() {
    echo "[participant-mutation] cleanup: restoring mutated file + clearing lockfile"
    git checkout -- "${TAGGING_GO}" 2>/dev/null || true
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

# Quiescence: scan the target file for residual MUTATED PA- markers.
check_quiescence() {
    local label="$1"
    if grep -qE 'MUTATED PA-M[0-9]+' "${TAGGING_GO}" 2>/dev/null; then
        echo "ABORT  ${label}: MUTATED marker LEAKED in $(basename "${TAGGING_GO}") — restore failed!"
        return 1
    fi
    return 0
}

# run_paired applies a perl mutation, asserts the marker landed, runs the
# detector and EXPECTS it to FAIL (mutation observable), restores, verifies the
# restore is byte-for-byte clean, and re-runs the detector EXPECTING PASS.
#
#   $1 — friendly name (M1/M2/M3 + property)
#   $2 — perl one-liner that applies the mutation (MUST inject MUTATED PA-M<n>)
#   $3 — detector command (go test invocation that pins the property)
#   $4 — anchor regex that MUST be findable in the file post-mutation
run_paired() {
    local name="$1" mutate_cmd="$2" detector="$3" anchor="$4"
    local file="${TAGGING_GO}"
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
    if eval "${detector}" > "/tmp/pameta-${name// /-}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/pameta-${name// /-}.log" | sed 's/^/      /'
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
    if ! eval "${detector}" > "/tmp/pameta-${name// /-}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/pameta-${name// /-}-restored.log" | sed 's/^/      /'
        fail=$((fail+1))
        return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# ----------------------------------------------------------------------
# M1: Remove the operator-skip guard. The operator must NEVER be tagged.
#     Mutation neutralizes `if handle == operatorHandle {` to `if false {`
#     (operator now passes through) → operator leaks into mentions.
# Detector: TestMentionsFor_MutationWouldFail (operator self-ping case FAILs).
# ----------------------------------------------------------------------
run_paired \
    "M1-operator-skip" \
    "perl -i -pe 's|\tif handle == operatorHandle \{|\tif false { // MUTATED PA-M1: operator-skip guard removed (self-ping leak)|' '${TAGGING_GO}'" \
    "go test -run '^(TestMentionsFor_MutationWouldFail|TestStressChaos_SkipGuards_Isolated)\$' -count=1 ./commons/" \
    'MUTATED PA-M1'

# ----------------------------------------------------------------------
# M2: Remove the SystemAgentHandle skip from the first guard. Claude must
#     NEVER be tagged. Mutation drops the `|| handle == SystemAgentHandle`
#     clause so only the empty-string check remains → Claude leaks.
# Detector: TestStressChaos_SkipGuards_Isolated — it gives Claude a REAL tgram
#     alias so the alias filter cannot mask the missing skip guard (the default
#     roster's Claude has no alias, so TestMentionsFor_MutationWouldFail does NOT
#     catch this mutation — a finding from the first gate run, 2026-05-31).
# ----------------------------------------------------------------------
run_paired \
    "M2-claude-skip" \
    "perl -i -pe 's{\t\tif handle == \"\" \|\| handle == SystemAgentHandle \{}{\t\tif handle == \"\" \{ // MUTATED PA-M2: Claude-skip removed (system agent tagged)}' '${TAGGING_GO}'" \
    "go test -run '^TestStressChaos_SkipGuards_Isolated\$' -count=1 ./commons/" \
    'MUTATED PA-M2'

# ----------------------------------------------------------------------
# M3: Drop the assigned_to branch. The assignee must be tagged when it is a
#     non-operator human. Mutation no-ops `add(assignedTo)` → assignee dropped.
# Detector: TestMentionsFor_MutationWouldFail (assignee-tagged case FAILs).
# ----------------------------------------------------------------------
run_paired \
    "M3-assignee-branch" \
    "perl -i -pe 's|\tadd\(assignedTo\)|\t_ = assignedTo // MUTATED PA-M3: assigned_to branch dropped (assignee never tagged)|' '${TAGGING_GO}'" \
    "go test -run '^TestMentionsFor_MutationWouldFail\$' -count=1 ./commons/" \
    'MUTATED PA-M3'

# ----------------------------------------------------------------------
# §107.y working-tree quiescence — assert no MUTATED PA-M* markers leaked.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED PA-M* markers =="
if check_quiescence "post-restore quiescence"; then
    echo "PASS  Quiescence: no MUTATED PA-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: a MUTATED PA-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Final tally.
# ----------------------------------------------------------------------
echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: participant mutation gate (3 paired)"
else
    echo "PARTICIPANT META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
