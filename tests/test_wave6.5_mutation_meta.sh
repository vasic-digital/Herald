#!/usr/bin/env bash
# tests/test_wave6.5_mutation_meta.sh — Paired §1.1 mutation test for
# Wave 6.5 (comprehensive ticket-lifecycle layer — §32.6 classifier,
# DocsIssueOpener, atomic Issues↔Fixed migration, outbound attachment
# fan-out).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILs when the property it claims to enforce is
# removed. A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107.y working-tree quiescence (lesson from commit 72e81ab — the
# 2026-05-21 JWT-bypass residue incident): this gate writes
# .git/MUTATION_IN_PROGRESS on entry, removes it via trap-on-exit, and
# refuses to start if the lockfile is already present (another gate in
# flight) or if the working tree is dirty. Trap-on-exit restores every
# mutated file even on `set -e` early-exit; final quiescence check greps
# the mutation targets for residual `MUTATED W65-M*` markers.
#
# Wave 6.5 covers FIVE mutations (one per production surface T1..T6):
#
#   M1. Blank §32.6 classifier in pherald/internal/inbound/classify.go:
#       inject `if true { return Classification{} }` at the head of
#       Classify(). All table-driven test cases assert specific
#       Type/Criticality/Confidence return values; an empty Classification
#       fails the very first case ("bug-basic" expects Type=="bug" got "").
#       Detector: TestClassify.
#
#   M2. Skip os.Rename in pherald/internal/inbound/issue_opener.go:
#       prependUnderOpen writes the temp file then no-ops the rename, so
#       docs/Issues.md is never updated. Detector:
#       TestDocsIssueOpener_OpenIssueAppends re-reads IssuesPath and
#       expects the new HRD-101 row → row absent → FAIL.
#
#   M3. Skip migrateRow in HandleDone (pherald/internal/inbound/commands.go):
#       inject `if false {` around the migrateRow call so HandleDone
#       short-circuits with a fake success reply but no fs change.
#       Detector: TestHandleDone_HappyPath asserts HRD-042 leaves
#       Issues.md → still present → FAIL.
#
#   M4. Skip migrateRow in HandleReopen (commands.go): same pattern as M3
#       on the symmetric handler. Detector: TestHandleReopen_HappyPath.
#
#   M5. Skip the attachment fan-out loop in
#       commons_messaging/channels/tgram/send.go: replace
#       `for i, att := range attachments {` with
#       `for i, att := range []commons.Attachment{} {` so SendReply emits
#       the text reply but ZERO multipart attachment calls. Detector:
#       TestSendReply_OnePhoto_TwoCalls expects 2 wire calls (1 text + 1
#       sendPhoto) — gets 1 → FAIL.
#
# Returns 0 only when every mutation causes its detector to FAIL AND
# every detector returns to PASS after restore AND no MUTATED W65-M*
# markers leaked. Non-zero on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

CLASSIFY_GO="${REPO_ROOT}/pherald/internal/inbound/classify.go"
ISSUE_OPENER_GO="${REPO_ROOT}/pherald/internal/inbound/issue_opener.go"
COMMANDS_GO="${REPO_ROOT}/pherald/internal/inbound/commands.go"
SEND_GO="${REPO_ROOT}/commons_messaging/channels/tgram/send.go"

# §107.y pre-flight: working-tree quiescence guard.
if git diff --quiet && git diff --cached --quiet; then : ; else
    echo "FAIL: working tree dirty before mutation gate; abort per §107.y"
    git status --short
    exit 1
fi

if [ -f .git/MUTATION_IN_PROGRESS ]; then
    echo "FAIL: another mutation gate already in flight (.git/MUTATION_IN_PROGRESS present)"
    exit 1
fi

# Pre-flight: refuse to start if production code already carries Wave
# 6.5 mutation residue (governance test files are exempt by virtue of
# file path).
for f in "${CLASSIFY_GO}" "${ISSUE_OPENER_GO}" "${COMMANDS_GO}" "${SEND_GO}"; do
    if grep -qE 'MUTATED W65-M' "${f}" 2>/dev/null; then
        echo "FAIL: production file ${f} carries pre-existing MUTATED W65-M marker"
        exit 1
    fi
done

touch .git/MUTATION_IN_PROGRESS

cleanup_all() {
    echo "[wave6.5-mutation] cleanup: restoring mutated files + clearing lockfile"
    git checkout -- "${CLASSIFY_GO}" "${ISSUE_OPENER_GO}" "${COMMANDS_GO}" "${SEND_GO}" 2>/dev/null || true
    rm -f .git/MUTATION_IN_PROGRESS
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

# Quiescence check: scan the four mutation target files for residual
# MUTATED W65-M markers. The check scopes to known mutation target
# files — broader greps would match legitimate prose in this script
# itself.
check_quiescence() {
    local label="$1"
    local leaks=0
    for f in "${CLASSIFY_GO}" "${ISSUE_OPENER_GO}" "${COMMANDS_GO}" "${SEND_GO}"; do
        if grep -qE 'MUTATED W65-M[0-9]+' "${f}" 2>/dev/null; then
            echo "ABORT  ${label}: MUTATED W65-M marker LEAKED in $(basename "${f}") — restore failed!"
            leaks=$((leaks+1))
        fi
    done
    return ${leaks}
}

# run_paired applies a perl mutation, asserts the marker landed, runs
# the detector and EXPECTS it to FAIL (mutation observable), restores,
# verifies the restore is byte-for-byte clean, and re-runs the detector
# EXPECTING PASS (production code regression caught).
#
#   $1 — friendly name (M1/M2/M3/M4/M5 + property)
#   $2 — path to file to mutate
#   $3 — perl -i -0pe one-liner that applies the mutation
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
    if eval "${detector}" > "/tmp/w65meta-${name// /-}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/w65meta-${name// /-}.log" | sed 's/^/      /'
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
    if ! eval "${detector}" > "/tmp/w65meta-${name// /-}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/w65meta-${name// /-}-restored.log" | sed 's/^/      /'
        fail=$((fail+1))
        return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# ----------------------------------------------------------------------
# M1: Blank §32.6 classifier — Classify returns Classification{} always.
# Detector: TestClassify table-driven cases (bug-basic etc.) all assert
# specific Type/Confidence values; empty struct fails immediately.
# Mutation: inject `if true { return Classification{} // MUTATED W65-M1 }`
# at the head of Classify() body. The remaining code becomes unreachable
# but compiles (Go allows unreachable code after `if true { return }`).
# ----------------------------------------------------------------------
run_paired \
    "M1-blank-classifier" \
    "${CLASSIFY_GO}" \
    "perl -i -0pe 's|func Classify\(text string\) Classification \{\n\tt := strings\.TrimSpace\(text\)|func Classify(text string) Classification {\n\tif true { _ = text; return Classification{} } // MUTATED W65-M1: blank classifier (no prefix inspection)\n\tt := strings.TrimSpace(text)|' '${CLASSIFY_GO}'" \
    "go test -run 'TestClassify' -count=1 ./pherald/internal/inbound/..." \
    'MUTATED W65-M1'

# ----------------------------------------------------------------------
# M2: DocsIssueOpener.prependUnderOpen skips the os.Rename.
# Detector: TestDocsIssueOpener_OpenIssueAppends re-reads IssuesPath
# expecting the new HRD-101 row; without rename, the file is unchanged
# so the assertion `strings.Contains(string(data), "| HRD-101 |")` FAILs.
# Mutation: replace the `return os.Rename(tmp, o.IssuesPath)` final
# statement with `_ = tmp; return nil // MUTATED W65-M2`.
# ----------------------------------------------------------------------
run_paired \
    "M2-issue-opener-no-rename" \
    "${ISSUE_OPENER_GO}" \
    "perl -i -pe 's|\treturn os\.Rename\(tmp, o\.IssuesPath\)|\t_ = tmp; return nil // MUTATED W65-M2: rename skipped, Issues.md never updated|' '${ISSUE_OPENER_GO}'" \
    "go test -run 'TestDocsIssueOpener_OpenIssueAppends' -count=1 ./pherald/internal/inbound/..." \
    'MUTATED W65-M2'

# ----------------------------------------------------------------------
# M3: HandleDone skips migrateRow.
# Detector: TestHandleDone_HappyPath reads Issues.md after the call,
# expects HRD-042 row to be GONE (migrated); when migrateRow is skipped,
# the row is still there → assertion FAILs.
# Mutation: wrap the `if err := c.migrateRow(...)` call in `if false {`
# so it never executes; the success reply is still returned. The wrapped
# `if false { if err := ...` keeps the inner `}` matched and the Go
# parser happy.
# ----------------------------------------------------------------------
run_paired \
    "M3-handledone-skip-migrate" \
    "${COMMANDS_GO}" \
    "perl -i -0pe 's|\tif err := c\.migrateRow\(ctx, c\.IssuesPath, c\.FixedPath, ref, \"Closed \(Wave 6\.5\)\"\); err != nil \{\n\t\treturn \"\", nil, err\n\t\}\n\treturn fmt\.Sprintf\(\"Done: %s migrated|\tif false { // MUTATED W65-M3: HandleDone skips migrateRow — no fs change\n\t\tif err := c.migrateRow(ctx, c.IssuesPath, c.FixedPath, ref, \"Closed (Wave 6.5)\"); err != nil {\n\t\t\treturn \"\", nil, err\n\t\t}\n\t}\n\treturn fmt.Sprintf(\"Done: %s migrated|' '${COMMANDS_GO}'" \
    "go test -run 'TestHandleDone_HappyPath' -count=1 ./pherald/internal/inbound/..." \
    'MUTATED W65-M3'

# ----------------------------------------------------------------------
# M4: HandleReopen skips migrateRow.
# Symmetric to M3 — wraps the call in `if false {`. Detector:
# TestHandleReopen_HappyPath asserts HRD-NNN moves Fixed.md → Issues.md;
# without migrateRow no movement → FAIL.
# ----------------------------------------------------------------------
run_paired \
    "M4-handlereopen-skip-migrate" \
    "${COMMANDS_GO}" \
    "perl -i -0pe 's|\tif err := c\.migrateRow\(ctx, c\.FixedPath, c\.IssuesPath, ref, \"Reopened \(Wave 6\.5\)\"\); err != nil \{\n\t\treturn \"\", nil, err\n\t\}\n\treturn fmt\.Sprintf\(\"Reopen: %s migrated|\tif false { // MUTATED W65-M4: HandleReopen skips migrateRow — no fs change\n\t\tif err := c.migrateRow(ctx, c.FixedPath, c.IssuesPath, ref, \"Reopened (Wave 6.5)\"); err != nil {\n\t\t\treturn \"\", nil, err\n\t\t}\n\t}\n\treturn fmt.Sprintf(\"Reopen: %s migrated|' '${COMMANDS_GO}'" \
    "go test -run 'TestHandleReopen_HappyPath' -count=1 ./pherald/internal/inbound/..." \
    'MUTATED W65-M4'

# ----------------------------------------------------------------------
# M5: SendReply skips the attachment fan-out loop.
# Detector: TestSendReply_OnePhoto_TwoCalls expects EXACTLY 2 wire
# calls (1 sendMessage text + 1 sendPhoto media); empty loop yields 1
# call → FAIL.
# Mutation: replace `for i, att := range attachments {` with
# `for i, att := range []commons.Attachment{} {` so the loop runs zero
# iterations. The MUTATED W65-M5 marker is added as a trailing comment
# on the for line (Go accepts comments inside the parameter list of a
# for range, between the literal and the opening brace).
# ----------------------------------------------------------------------
run_paired \
    "M5-sendreply-skip-attachments" \
    "${SEND_GO}" \
    "perl -i -pe 's|\tfor i, att := range attachments \{|\tfor i, att := range []commons.Attachment{} { // MUTATED W65-M5: SendReply attachment loop skipped|' '${SEND_GO}'" \
    "go test -run 'TestSendReply_OnePhoto_TwoCalls' -count=1 ./commons_messaging/channels/tgram/..." \
    'MUTATED W65-M5'

# ----------------------------------------------------------------------
# §107.y working-tree quiescence — assert no MUTATED W65-M* markers
# leaked into the four target files post-restore.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED W65-M* markers =="
if check_quiescence "post-restore quiescence"; then
    echo "PASS  Quiescence: no MUTATED W65-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: at least one MUTATED W65-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Final tally.
# ----------------------------------------------------------------------
echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: wave6.5 mutation gate (5 paired)"
else
    echo "WAVE 6.5 META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
