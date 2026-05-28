#!/usr/bin/env bash
# tests/test_wave7_mutation_meta.sh — Paired §1.1 mutation test for Wave 7
# (generic messenger-channel framework: registry + selffilter + Slack adapter).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILs when the property it claims to enforce is
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
# Hermetic detectors only (no live Telegram / Slack / Claude binary needed).
#
# Wave 7 covers three mutations:
#
#   M1. Blank IsSelfEcho in commons_messaging/channels/selffilter.go:
#       body → `return false` (every bot-own message kept → echo-loop
#       hazard). Detector: TestIsSelfEchoUsername — sub-assertion (a)
#       asserts bot-own IS self-echo; with the mutation it is not →
#       FAIL on the very first sub-assertion.
#
#   M2. Drop the slack channels.Register entry in
#       commons_messaging/channels/slack/slack.go: comment out the
#       channels.Register call body inside init(). Detector:
#       TestSlackRegisteredViaInit (added to slack_test.go in this task)
#       asserts channels.Names() contains "slack"; with the mutation the
#       registration is skipped → Names() lacks "slack" → FAIL.
#
#   M3. Drop thread_ts in commons_messaging/channels/slack/send.go:
#       remove the `opts = append(opts, slack.MsgOptionTS(replyToID))`
#       line. Detector: TestSlackSendReplyUsesThreadTS asserts
#       thread_ts=="1654.0001" on the wire; with the mutation the field
#       is absent → FAIL with got "" want "1654.0001".
#
# Returns 0 only when every mutation causes its detector to FAIL AND every
# detector returns to PASS after restore AND no MUTATED W7-M* markers leaked.
# Non-zero on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

SELFFILTER_GO="${REPO_ROOT}/commons_messaging/channels/selffilter.go"
SLACK_GO="${REPO_ROOT}/commons_messaging/channels/slack/slack.go"
SLACK_SEND_GO="${REPO_ROOT}/commons_messaging/channels/slack/send.go"

# §107.y pre-flight: working-tree quiescence guard.
if git diff --quiet -- "${SELFFILTER_GO}" "${SLACK_GO}" "${SLACK_SEND_GO}" \
    && git diff --cached --quiet -- "${SELFFILTER_GO}" "${SLACK_GO}" "${SLACK_SEND_GO}"; then : ; else
    echo "FAIL: target files dirty before mutation gate; abort per §107.y"
    git status --short -- "${SELFFILTER_GO}" "${SLACK_GO}" "${SLACK_SEND_GO}"
    exit 1
fi

if [ -f .git/MUTATION_IN_PROGRESS ]; then
    echo "FAIL: another mutation gate already in flight (.git/MUTATION_IN_PROGRESS present)"
    exit 1
fi

# Pre-flight: refuse to start if production code already carries mutation
# residue (governance test files are exempt by virtue of file path).
for f in "${SELFFILTER_GO}" "${SLACK_GO}" "${SLACK_SEND_GO}"; do
    if grep -qE '// always pass|MUTATED for paired|MUTATED W7-' "${f}" 2>/dev/null; then
        echo "FAIL: production file ${f} carries pre-existing mutation marker"
        exit 1
    fi
done

touch .git/MUTATION_IN_PROGRESS

cleanup_all() {
    echo "[wave7-mutation] cleanup: restoring mutated files + clearing lockfile"
    git checkout -- "${SELFFILTER_GO}" "${SLACK_GO}" "${SLACK_SEND_GO}" 2>/dev/null || true
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

# Quiescence check: scan the three target files for residual MUTATED W7-
# markers. The check scopes to known mutation target files — broader greps
# would match legitimate prose in test_wave*_mutation_meta.sh.
check_quiescence() {
    local label="$1"
    local leaks=0
    for f in "${SELFFILTER_GO}" "${SLACK_GO}" "${SLACK_SEND_GO}"; do
        if grep -qE 'MUTATED W7-M[0-9]+' "${f}" 2>/dev/null; then
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
#   $3 — perl -i one-liner that applies the mutation (MUST inject the
#        canonical MUTATED W7-M<n> anchor comment)
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
    if eval "${detector}" > "/tmp/w7meta-${name// /-}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/w7meta-${name// /-}.log" | sed 's/^/      /'
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
    if ! eval "${detector}" > "/tmp/w7meta-${name// /-}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/w7meta-${name// /-}-restored.log" | sed 's/^/      /'
        fail=$((fail+1))
        return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# ----------------------------------------------------------------------
# M1: Blank IsSelfEcho — body → `return false` (every bot-own message kept).
# Detector: TestIsSelfEchoUsername asserts bot-own IS self-echo (line 28
# of selffilter_test.go: `if !channels.IsSelfEcho(...) { t.Fatal(...) }`).
# Mutation makes IsSelfEcho return false unconditionally → bot-own NOT
# detected → first sub-assertion FAILs.
# ----------------------------------------------------------------------
run_paired \
    "M1-self-echo-filter" \
    "${SELFFILTER_GO}" \
    "perl -i -0pe 's|func IsSelfEcho\(ev commons\.InboundEvent, self SelfIdentity\) bool \{\n\tif self\.Value == \"\" \{\n\t\treturn false\n\t\}\n\tif ev\.Raw == nil \{\n\t\treturn false\n\t\}\n\tisBot, _ := ev\.Raw\[RawSenderIsBot\]\.\(bool\)\n\tif \!isBot \{\n\t\treturn false\n\t\}\n\tkind, _ := ev\.Raw\[RawSenderIdentityKnd\]\.\(string\)\n\tid, _ := ev\.Raw\[RawSenderIdentity\]\.\(string\)\n\treturn IdentityKind\(kind\) == self\.Kind && id == self\.Value\n\}|func IsSelfEcho(ev commons.InboundEvent, self SelfIdentity) bool {\n\t// MUTATED W7-M1: self-echo filter disabled — every bot-own message kept (echo-loop hazard)\n\treturn false\n}|' '${SELFFILTER_GO}'" \
    "go test -race -count=1 -run 'TestIsSelfEchoUsername' ./commons_messaging/channels/..." \
    'MUTATED W7-M1'

# ----------------------------------------------------------------------
# M2: Drop the slack channels.Register entry in init().
# Detector: TestSlackRegisteredViaInit asserts channels.Names() contains
# "slack". With Register commented out the registration is skipped →
# Names() lacks "slack" → FAIL with "slack not registered via init()".
# ----------------------------------------------------------------------
run_paired \
    "M2-slack-registry" \
    "${SLACK_GO}" \
    "perl -i -0pe 's|func init\(\) \{\n\tchannels\.Register\(string\(commons\.ChannelSlack\), func\(cfg channels\.Config\) \(channels\.Channel, error\) \{\n\t\tif cfg\.Token == \"\" \{\n\t\t\treturn nil, fmt\.Errorf\(\"slack: cfg\.Token \\(xoxb- bot token\\) required\"\)\n\t\t\}\n\t\treturn NewWithBaseURL\(cfg\.Token, cfg\.AppToken, cfg\.Target, cfg\.BaseURL\), nil\n\t\}\)\n\}|func init() {\n\t// MUTATED W7-M2: slack channels.Register call removed (resolution by name BROKEN)\n\t_ = commons.ChannelSlack\n}|' '${SLACK_GO}'" \
    "go test -race -count=1 -run 'TestSlackRegisteredViaInit' ./commons_messaging/channels/slack/..." \
    'MUTATED W7-M2'

# ----------------------------------------------------------------------
# M3: Drop thread_ts append in SendReply.
# Detector: TestSlackSendReplyUsesThreadTS asserts the wire payload's
# `thread_ts` form value is "1654.0001". With the MsgOptionTS append
# line removed, opts never carries thread_ts → the chat.postMessage
# body omits the field → FAIL with `thread_ts="" want 1654.0001`.
# ----------------------------------------------------------------------
run_paired \
    "M3-thread-ts" \
    "${SLACK_SEND_GO}" \
    "perl -i -pe 's|\t\topts = append\(opts, slack\.MsgOptionTS\(replyToID\)\)|\t\t// MUTATED W7-M3: thread_ts append dropped — reply degrades to top-level message\n\t\t_ = replyToID|' '${SLACK_SEND_GO}'" \
    "go test -race -count=1 -run 'TestSlackSendReplyUsesThreadTS' ./commons_messaging/channels/slack/..." \
    'MUTATED W7-M3'

# ----------------------------------------------------------------------
# §107.y working-tree quiescence — assert no MUTATED W7-M* markers leaked.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED W7-M* markers =="
if check_quiescence "post-restore quiescence"; then
    echo "PASS  Quiescence: no MUTATED W7-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: at least one MUTATED W7-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Final tally.
# ----------------------------------------------------------------------
echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: wave7 mutation gate (3 paired)"
else
    echo "WAVE 7 META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
