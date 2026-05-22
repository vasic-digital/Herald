#!/usr/bin/env bash
# tests/test_wave6_mutation_meta.sh — Paired §1.1 mutation test for Wave 6
# (inbound runtime + Claude Code headless bridge + reply API).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILS when the property it claims to enforce is
# removed. A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107 covenant: this script MUST observe each mutation actually applies
# (grep + assert anchor in mutated file), MUST run the targeted detector
# and observe it FAIL on the mutated build, MUST restore the original file,
# MUST verify the restore worked (post-restore detector PASS + working-tree
# quiescence: byte-for-byte match against `git show HEAD:<path>`). A
# mutation gate that doesn't actually bite is itself a bluff.
#
# §107.y working-tree quiescence (lesson from commit 72e81ab — the
# 2026-05-21 JWT-bypass mutation residue incident): this gate writes
# .git/MUTATION_IN_PROGRESS on entry, removes it via trap-on-exit, and
# refuses to start if the lockfile is already present (another gate in
# flight) or if the working tree is dirty. Trap-on-exit restores every
# mutated file even on `set -e` early-exit.
#
# DESIGN ADJUSTMENT vs the plan T12 detectors (which require live
# Telegram + claude binary — neither reliably available in CI):
# Wave 6 uses hermetic unit-test detectors that are already load-bearing.
# Each mutation targets a specific production source line; each detector
# is a single `go test -run <TestName>` invocation against the unit test
# that pins that property. Hermetic — no creds, no network, no live
# services. Runs in any environment.
#
# Wave 6 covers three mutations:
#
#   M1. Blank the bot self-filter in commons_messaging/channels/tgram/subscribe.go:
#       shouldDropBotSelf body → `return false` (always keeps bot-own
#       messages → echo-loop hazard). Detector:
#       TestSubscribeBotSelfFilter — sub-assertion (a) asserts bot-own
#       message IS dropped; with the mutation it is NOT dropped → FAIL.
#
#   M2. Swap the Opus model literal in
#       commons_messaging/dispatch/claude_code/dispatch.go:
#       "claude-opus-4-7" → "claude-sonnet-4-6". Detector:
#       TestDispatchCommandIncludesOpusModel — inspects the *exec.Cmd.Args
#       slice and asserts contiguous [--model, claude-opus-4-7]; with the
#       mutation argv contains claude-sonnet-4-6 → FAIL.
#
#   M3. Drop opts.ReplyTo assignment in
#       commons_messaging/channels/tgram/send.go:
#       `opts.ReplyTo = &telebot.Message{ID: replyToID}` → no-op.
#       Detector: TestSendReplyEmitsReplyToMessageID — httptest decodes
#       sendMessage JSON body and asserts reply_to_message_id == "42";
#       with the mutation opts.ReplyTo stays nil, the field is absent
#       from the wire payload → FAIL.
#
# Returns 0 only when every mutation causes its detector to FAIL AND every
# detector returns to PASS after restore AND no MUTATED markers leaked.
# Non-zero on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

SUBSCRIBE_GO="${REPO_ROOT}/commons_messaging/channels/tgram/subscribe.go"
DISPATCH_GO="${REPO_ROOT}/commons_messaging/dispatch/claude_code/dispatch.go"
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

# Pre-flight: refuse to start if production code already carries mutation
# residue (governance test files are exempt by virtue of file path).
for f in "${SUBSCRIBE_GO}" "${DISPATCH_GO}" "${SEND_GO}"; do
    if grep -qE '// always pass|MUTATED for paired|MUTATED W6-' "${f}" 2>/dev/null; then
        echo "FAIL: production file ${f} carries pre-existing mutation marker"
        exit 1
    fi
done

touch .git/MUTATION_IN_PROGRESS

cleanup_all() {
    echo "[wave6-mutation] cleanup: restoring mutated files + clearing lockfile"
    git checkout -- "${SUBSCRIBE_GO}" "${DISPATCH_GO}" "${SEND_GO}" 2>/dev/null || true
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

# Quiescence check: scan the three target files for residual MUTATED W6-
# markers. The check scopes to known mutation target files — broader greps
# would match legitimate prose in test_wave*_mutation_meta.sh.
check_quiescence() {
    local label="$1"
    local leaks=0
    for f in "${SUBSCRIBE_GO}" "${DISPATCH_GO}" "${SEND_GO}"; do
        if grep -qE 'MUTATED W6-M[0-9]+' "${f}" 2>/dev/null; then
            echo "ABORT  ${label}: MUTATED marker LEAKED in $(basename "${f}") — restore failed!"
            leaks=$((leaks+1))
        fi
    done
    return ${leaks}
}

# run_paired applies a perl mutation, asserts the marker landed, runs the
# detector and EXPECTS it to FAIL (mutation observable), restores, verifies
# the restore is byte-for-byte clean, and re-runs the detector EXPECTING
# PASS (production code regression caught).
#
#   $1 — friendly name (M1/M2/M3 + property)
#   $2 — path to file to mutate
#   $3 — perl -i -0pe one-liner that applies the mutation (MUST inject the
#        canonical MUTATED W6-M<n> anchor comment)
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
    if eval "${detector}" > "/tmp/w6meta-${name// /-}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/w6meta-${name// /-}.log" | sed 's/^/      /'
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
    if ! eval "${detector}" > "/tmp/w6meta-${name// /-}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/w6meta-${name// /-}-restored.log" | sed 's/^/      /'
        fail=$((fail+1))
        return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# ----------------------------------------------------------------------
# M1: Blank bot self-filter — shouldDropBotSelf must return false always.
# Detector: TestSubscribeBotSelfFilter asserts bot-own message IS dropped.
# Mutation makes the filter return false unconditionally → bot-own NOT
# dropped → test sub-assertion (a) FAILs.
# ----------------------------------------------------------------------
run_paired \
    "M1-bot-self-filter" \
    "${SUBSCRIBE_GO}" \
    "perl -i -0pe 's|func shouldDropBotSelf\(msg \*telebot\.Message, selfUsername string\) bool \{\n\tif msg == nil \|\| msg\.Sender == nil \{\n\t\treturn false\n\t\}\n\tif \!msg\.Sender\.IsBot \{\n\t\treturn false\n\t\}\n\treturn msg\.Sender\.Username == selfUsername\n\}|func shouldDropBotSelf(msg *telebot.Message, selfUsername string) bool {\n\t// MUTATED W6-M1: bot self-filter disabled — every bot-own message kept (echo-loop hazard)\n\treturn false\n}|' '${SUBSCRIBE_GO}'" \
    "go test -run 'TestSubscribeBotSelfFilter' -count=1 ./commons_messaging/channels/tgram/..." \
    'MUTATED W6-M1'

# ----------------------------------------------------------------------
# M2: Opus → Sonnet model literal swap.
# Detector: TestDispatchCommandIncludesOpusModel asserts argv contains
# contiguous [--model, claude-opus-4-7]. Mutation swaps the literal to
# claude-sonnet-4-6 → test FAILs at the "argv missing contiguous" branch.
# ----------------------------------------------------------------------
run_paired \
    "M2-opus-pin" \
    "${DISPATCH_GO}" \
    "perl -i -pe 's|\"claude-opus-4-7\", // Wave 6 operator-locked: Opus always\.|\"claude-sonnet-4-6\", // MUTATED W6-M2: Opus pin removed (Sonnet substituted)|' '${DISPATCH_GO}'" \
    "go test -run 'TestDispatchCommandIncludesOpusModel' -count=1 ./commons_messaging/dispatch/claude_code/..." \
    'MUTATED W6-M2'

# ----------------------------------------------------------------------
# M3: Drop opts.ReplyTo assignment in SendReply.
# Detector: TestSendReplyEmitsReplyToMessageID asserts wire-byte body
# contains reply_to_message_id == "42". Mutation no-ops the ReplyTo
# assignment so the field is absent from the wire payload → test FAILs
# with got "" want "42".
# ----------------------------------------------------------------------
run_paired \
    "M3-reply-to" \
    "${SEND_GO}" \
    "perl -i -pe 's|\t\topts\.ReplyTo = &telebot\.Message\{ID: replyToID\}|\t\t// MUTATED W6-M3: opts.ReplyTo assignment dropped (reply degrades to fresh message)\n\t\t_ = replyToID|' '${SEND_GO}'" \
    "go test -run 'TestSendReplyEmitsReplyToMessageID' -count=1 ./commons_messaging/channels/tgram/..." \
    'MUTATED W6-M3'

# ----------------------------------------------------------------------
# §107.y working-tree quiescence — assert no MUTATED W6-M* markers leaked.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED W6-M* markers =="
if check_quiescence "post-restore quiescence"; then
    echo "PASS  Quiescence: no MUTATED W6-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: at least one MUTATED W6-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Final tally.
# ----------------------------------------------------------------------
echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: wave6 mutation gate (3 paired)"
else
    echo "WAVE 6 META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
