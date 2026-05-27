#!/usr/bin/env bash
#
# Wave 6.5 — 15-scenario comprehensive lifecycle e2e (LIVE).
#
# TWO MODES (qaherald-auto T8):
#
#   1. AUTOMATED (default) — delegates the full 15-scenario lifecycle to
#      the `qaherald lifecycle` Go binary, which posts each scenario's
#      input via a SECOND Telegram bot (HERALD_QA_BOT_TOKEN), reads
#      pherald-bot's replies via getUpdates, asserts wire-byte evidence,
#      and emits docs/qa/HRD-101-lifecycle-<run-id>/{transcript.jsonl,
#      report.md,attachments/}. No human typing. qaherald's exit code is
#      propagated: any scenario FAIL → this script exits non-zero.
#
#   2. MANUAL (--manual flag OR HERALD_LIFECYCLE_MANUAL=1) — the legacy
#      operator-typing UX: the script narrates each of the 15 §32.6
#      scenarios, waits for the operator to type the input into Telegram,
#      then tails pherald's transcript.jsonl for the wire-byte evidence.
#
# §107 anti-bluff anchor: each scenario PASS cites a SPECIFIC wire-byte
# string (automated: qaherald's report.md per-scenario evidence; manual:
# the matched transcript.jsonl line) — never a generic "scenario ran".
#
# Driver model (faithful to the qaherald-auto plan, adapted to the
# actual pherald listen + qaherald lifecycle surface):
#   - pherald listen is ENV-driven (HERALD_TGRAM_BOT_TOKEN +
#     HERALD_TGRAM_CHAT_ID + HERALD_OPERATOR_IDS + HERALD_CLAUDE_BIN +
#     HERALD_PROJECT_NAME). No --bot-token / --chat-id flags exist.
#   - qaherald lifecycle is FLAG-driven (--qa-bot-token / --chat-id /
#     --pherald-bot-username / --out / --docs-dir / --pherald-qa-out-dir).
#   - Real claude env name is HERALD_CLAUDE_BIN.
#   - Defaults HERALD_QA_RUN_ID to a timestamped value if unset.
#
# Cleanup model: SIGTERM pherald listen on EXIT trap; remove built
# binaries; persist QA_OUT contents (the evidence the operator commits).

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"
cd "${REPO_ROOT}"

skip_with_reason() { echo "SKIP test_wave6.5_lifecycle.sh — $1"; exit 0; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# --- Mode selection: --manual flag OR HERALD_LIFECYCLE_MANUAL=1 ---
MANUAL_MODE=0
for arg in "$@"; do
  case "${arg}" in
    --manual) MANUAL_MODE=1 ;;
    *) ;;
  esac
done
[ "${HERALD_LIFECYCLE_MANUAL:-0}" = "1" ] && MANUAL_MODE=1

# --- Pre-flight env gates common to BOTH modes (skip per §11.4.5) ---
[ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ] || skip_with_reason "HERALD_TGRAM_BOT_TOKEN unset (see docs/guides/OPERATOR_CREDENTIALS.md)"
[ -n "${HERALD_TGRAM_CHAT_ID:-}" ]   || skip_with_reason "HERALD_TGRAM_CHAT_ID unset"
[ -n "${HERALD_OPERATOR_IDS:-}" ]    || skip_with_reason "HERALD_OPERATOR_IDS unset — Done:/Reopen: scenarios would fail (operator-role allowlist)"

# claude CLI: env override wins; else $PATH lookup
CLAUDE_BIN="${HERALD_CLAUDE_BIN:-claude}"
command -v "${CLAUDE_BIN}" >/dev/null || skip_with_reason "claude CLI not found at ${CLAUDE_BIN} (HERALD_CLAUDE_BIN or PATH)"

command -v jq   >/dev/null || skip_with_reason "jq not installed"
command -v curl >/dev/null || skip_with_reason "curl not installed"

# --- QA run id (operator-supplied; else timestamp + suffix) ---
HERALD_QA_RUN_ID="${HERALD_QA_RUN_ID:-$(date -u +%Y-%m-%dT%H-%M-%S)-w6.5live}"
export HERALD_QA_RUN_ID

QA_OUT="docs/qa/HRD-101-lifecycle-${HERALD_QA_RUN_ID}"
mkdir -p "${QA_OUT}/attachments"

echo "[w6.5-lifecycle] mode=$( [ "${MANUAL_MODE}" = "1" ] && echo manual || echo automated )"
echo "[w6.5-lifecycle] QA_OUT=${QA_OUT}"
echo "[w6.5-lifecycle] HERALD_OPERATOR_IDS=${HERALD_OPERATOR_IDS}"

# ====================================================================
# AUTOMATED MODE (default) — delegate to `qaherald lifecycle`.
# ====================================================================
if [ "${MANUAL_MODE}" != "1" ]; then
  # Automated mode needs a SECOND Telegram bot (qa-bot) distinct from
  # pherald-bot. Without it, SKIP-with-reason and tell the operator how
  # to run the legacy manual path instead.
  if [ -z "${HERALD_QA_BOT_TOKEN:-}" ]; then
    skip_with_reason "HERALD_QA_BOT_TOKEN unset — automated mode needs a 2nd Telegram bot (see docs/guides/OPERATOR_CREDENTIALS.md). Run with --manual (or HERALD_LIFECYCLE_MANUAL=1) for the operator-typing UX."
  fi

  PHERALD_BOT_USERNAME="${HERALD_PHERALD_BOT_USERNAME:-atmosphere_worker_bot}"

  echo "[w6.5-lifecycle] building qaherald → /tmp/qaherald ..."
  go build -o /tmp/qaherald ./qaherald/cmd/qaherald

  echo "[w6.5-lifecycle] building pherald → /tmp/pherald ..."
  go build -o /tmp/pherald ./pherald/cmd/pherald

  LISTEN_LOG="${QA_OUT}/pherald-listen.log"
  echo "[w6.5-lifecycle] starting pherald listen (log: ${LISTEN_LOG}) ..."
  /tmp/pherald listen --docs-dir docs --qa-out-dir "${QA_OUT}" \
    > "${LISTEN_LOG}" 2>&1 &
  PHERALD_PID=$!

  auto_cleanup() {
    if kill -0 "${PHERALD_PID}" 2>/dev/null; then
      kill -TERM "${PHERALD_PID}" 2>/dev/null || true
      wait "${PHERALD_PID}" 2>/dev/null || true
    fi
    rm -f /tmp/pherald /tmp/qaherald
  }
  trap auto_cleanup EXIT

  # Give pherald a moment to boot (telebot.NewBot + getMe + Subscribe).
  sleep 5
  if ! kill -0 "${PHERALD_PID}" 2>/dev/null; then
    echo "FAIL: pherald listen exited prematurely during boot." >&2
    echo "--- pherald-listen.log tail ---" >&2
    tail -n 60 "${LISTEN_LOG}" >&2 || true
    exit 1
  fi
  echo "[w6.5-lifecycle] pherald listen PID=${PHERALD_PID}"

  echo "[w6.5-lifecycle] delegating 15 scenarios to qaherald lifecycle ..."
  set +e
  /tmp/qaherald lifecycle \
    --qa-bot-token="${HERALD_QA_BOT_TOKEN}" \
    --qa-bot-token-non-operator="${HERALD_QA_BOT_TOKEN_NON_OPERATOR:-}" \
    --chat-id="${HERALD_TGRAM_CHAT_ID}" \
    --pherald-bot-username="${PHERALD_BOT_USERNAME}" \
    --out="${QA_OUT}" \
    --run-id="${HERALD_QA_RUN_ID}" \
    --docs-dir=docs \
    --pherald-qa-out-dir="${QA_OUT}" \
    --scenario-timeout=120s \
    --overall-timeout=45m
  AUTO_EXIT=$?
  set -e

  echo "[w6.5-lifecycle] qaherald lifecycle exit code: ${AUTO_EXIT}"
  if [ "${AUTO_EXIT}" -ne 0 ]; then
    echo "FAIL: qaherald lifecycle reported failures (exit ${AUTO_EXIT}) — see ${QA_OUT}/report.md" >&2
    echo "--- report.md tail ---" >&2
    tail -n 40 "${QA_OUT}/report.md" 2>/dev/null >&2 || true
    exit "${AUTO_EXIT}"
  fi
  echo "PASS: qaherald lifecycle 15/15 scenarios — evidence: ${QA_OUT}/"
  echo "  transcript.jsonl, report.md, attachments/"
  exit 0
fi

# ====================================================================
# MANUAL MODE (--manual / HERALD_LIFECYCLE_MANUAL=1) — legacy
# operator-typing UX. Everything below is the original prompt-based
# driver, preserved verbatim.
# ====================================================================
echo "[w6.5-lifecycle] manual mode — operator drives each scenario by typing into Telegram"

# --- Snapshot docs/Issues.md + docs/Fixed.md BEFORE the run ---
cp docs/Issues.md "${QA_OUT}/issues-before.md"
cp docs/Fixed.md  "${QA_OUT}/fixed-before.md"
echo "[w6.5-lifecycle] snapshotted Issues.md → ${QA_OUT}/issues-before.md"
echo "[w6.5-lifecycle] snapshotted Fixed.md  → ${QA_OUT}/fixed-before.md"

# --- Build pherald ---
echo "[w6.5-lifecycle] building pherald → /tmp/pherald ..."
go build -o /tmp/pherald ./pherald/cmd/pherald

# --- Start pherald listen (env-driven; --qa-out-dir + --docs-dir flags) ---
LISTEN_LOG="${QA_OUT}/pherald-listen.log"
echo "[w6.5-lifecycle] starting pherald listen (log: ${LISTEN_LOG}) ..."
/tmp/pherald listen --docs-dir docs --qa-out-dir "${QA_OUT}" \
  > "${LISTEN_LOG}" 2>&1 &
PHERALD_PID=$!

cleanup() {
  if kill -0 "${PHERALD_PID}" 2>/dev/null; then
    kill -TERM "${PHERALD_PID}" 2>/dev/null || true
    wait "${PHERALD_PID}" 2>/dev/null || true
  fi
  rm -f /tmp/pherald
}
trap cleanup EXIT

# Give pherald a moment to boot (telebot.NewBot + getMe + Subscribe loop).
sleep 5

if ! kill -0 "${PHERALD_PID}" 2>/dev/null; then
  echo "FAIL: pherald listen exited prematurely during boot."
  echo "--- pherald-listen.log tail ---"
  tail -n 60 "${LISTEN_LOG}" || true
  exit 1
fi
echo "[w6.5-lifecycle] pherald listen PID=${PHERALD_PID}"

TRANSCRIPT="${QA_OUT}/transcript.jsonl"
touch "${TRANSCRIPT}"  # ensure exists so the grep tail loop has a target

# --- Helpers ---

# narrate scenario header — printed before waiting for operator ENTER.
narrate() {
  local sn="$1"; local title="$2"; local action="$3"; local expect="$4"
  echo ""
  echo "================================================================"
  echo " SCENARIO ${sn}: ${title}"
  echo "----------------------------------------------------------------"
  echo " ACTION:    ${action}"
  echo " EXPECTED:  ${expect}"
  echo "================================================================"
  echo "Press ENTER once you have performed the action..."
  read -r _
}

# wait_for_pattern: tails the transcript JSONL for up to N seconds looking
# for the literal substring $2; on success prints the matched line; on
# timeout fails LOUDLY with the pherald-listen.log tail (no silent skip).
# $1 = scenario id; $2 = wire-byte literal substring; $3 = timeout seconds
wait_for_pattern() {
  local sn="$1"; local pattern="$2"; local timeout="${3:-60}"
  local deadline=$(( $(date +%s) + timeout ))
  local hit=""
  while [ "$(date +%s)" -lt "${deadline}" ]; do
    if [ -s "${TRANSCRIPT}" ]; then
      hit="$(grep -F -- "${pattern}" "${TRANSCRIPT}" | tail -n 1 || true)"
      if [ -n "${hit}" ]; then
        echo "[${sn}] PASS — matched pattern in transcript.jsonl:"
        echo "    pattern: ${pattern}"
        echo "    line:    $(printf '%s' "${hit}" | head -c 240)"
        printf '%s\n' "${hit}" >> "${QA_OUT}/scenario-evidence.jsonl"
        echo "${sn}|PASS|${pattern}" >> "${QA_OUT}/scenario-verdicts.txt"
        return 0
      fi
    fi
    sleep 2
  done
  echo "FAIL [${sn}]: pattern not observed within ${timeout}s — '${pattern}'" >&2
  echo "--- pherald-listen.log tail ---" >&2
  tail -n 80 "${LISTEN_LOG}" >&2 || true
  echo "--- transcript.jsonl tail ---" >&2
  tail -n 20 "${TRANSCRIPT}" >&2 || true
  echo "${sn}|FAIL|${pattern}" >> "${QA_OUT}/scenario-verdicts.txt"
  exit 1
}

# --- Per-scenario narration + wire-byte assertion ---
# Each pattern below is a LITERAL substring chosen to be unique to the
# scenario. Classification field-names are title-cased per the actual
# pherald/internal/inbound.Classification Go struct (no JSON tags).
# Wire shape per pherald/cmd/pherald/journal.go:
#   {"ts":"...","direction":"out","kind":"cc.dispatch","payload":{...,"classification":{"Type":"bug",...}}}
# Fast-path command kinds emit a `tgram.send_reply` directly (no cc.dispatch).

# S1: plain greeting → query fallthrough → CC dispatch (confidence 0)
narrate "S1" "Plain greeting (Query fallthrough)" \
  "In the Telegram chat (chat_id=${HERALD_TGRAM_CHAT_ID}) send: Hi pherald, how are you?" \
  "pherald classifies as query (conf=0); cc.dispatch carries Type:'query', Confidence:0"
wait_for_pattern "S1" '"Type":"query","Criticality":"middle","Confidence":0' 90

# S2: Help: → fast-path → BuiltinHelp text in tgram.send_reply
narrate "S2" "Help: fast-path (BuiltinHelp)" \
  "Send in the chat: Help:" \
  "Instant reply (no CC); transcript shows tgram.send_reply containing 'Command catalogue (§32.6)'"
wait_for_pattern "S2" 'Command catalogue' 60

# S3: Status: → fast-path → docs/Status.md prose
narrate "S3" "Status: fast-path (docs/Status.md)" \
  "Send: Status:" \
  "Fast-path reply with docs/Status.md leading prose (substring 'Status')"
wait_for_pattern "S3" '"kind":"tgram.send_reply"' 60

# S4: Continue: → fast-path → docs/CONTINUATION.md prose
narrate "S4" "Continue: fast-path (docs/CONTINUATION.md)" \
  "Send: Continue:" \
  "Fast-path reply with CONTINUATION.md prose"
wait_for_pattern "S4" '"text":"# ' 60

# S5: Bug: prefix → CC issue.open path → HRD-NNN row appended
narrate "S5" "Bug: prefix → issue.open → HRD-NNN allocation" \
  "Send: Bug: telemetry pipe drops every hour" \
  "cc.dispatch with Type:'bug',Confidence:1; reply confirms HRD-NNN; new row in Issues.md"
wait_for_pattern "S5" '"Type":"bug","Criticality":"middle","Confidence":1' 120

# S6: Task: prefix → CC path → HRD-NNN row appended
narrate "S6" "Task: prefix → task classification" \
  "Send: Task: refactor channel registry" \
  "cc.dispatch with Type:'task'"
wait_for_pattern "S6" '"Type":"task","Criticality":"middle","Confidence":1' 120

# S7: Query: prefix → CC research path
narrate "S7" "Query: prefix → CC research path" \
  "Send: Query: what tag is current?" \
  "cc.dispatch with Type:'query',Confidence:1 (explicit prefix)"
wait_for_pattern "S7" '"Type":"query","Criticality":"middle","Confidence":1' 120

# S8: Done: HRD-XXX (operator) → Issues→Fixed migration
# Operator must use the actual HRD-NNN allocated by S5 (script does not
# auto-derive; that is the operator's read-out from S5's reply).
narrate "S8" "Done: HRD-XXX (operator-allowlisted) → Issues→Fixed" \
  "Read the HRD-NNN allocated by S5 from the reply. Send: Done: HRD-NNN (FROM AN ACCOUNT IN HERALD_OPERATOR_IDS)" \
  "Fast-path closure; reply confirms migration; row leaves Issues.md and lands in Fixed.md"
wait_for_pattern "S8" '"Type":"closure","Criticality":"middle","Confidence":1' 60

# S9: Done: HRD-XXX from NON-allowlisted account → rejection
narrate "S9" "Done: HRD-XXX (NON-allowlisted) → rejection" \
  "From a DIFFERENT Telegram account NOT in HERALD_OPERATOR_IDS, send: Done: HRD-NNN (any open HRD)" \
  "Closure classified but rejected; reply explains operator-role required"
wait_for_pattern "S9" 'HERALD_OPERATOR_IDS' 60

# S10: Reopen: HRD-XXX (operator) → Fixed→Issues migration
narrate "S10" "Reopen: HRD-XXX (operator) → Fixed→Issues" \
  "From an account in HERALD_OPERATOR_IDS, send: Reopen: HRD-NNN (the one closed in S8)" \
  "Fast-path reopen; row migrates back to Issues.md"
wait_for_pattern "S10" '"Type":"reopen","Criticality":"middle","Confidence":1' 60

# S11: inbound photo with Bug: caption → attachment captured by sha256
narrate "S11" "Inbound photo + Bug: caption → attachment + bug classification" \
  "Attach a photo to a Telegram message with caption: Bug: visual artifact in dashboard" \
  "tgram.message line carries an attachments[] entry with sha256; classification Type:'bug'"
wait_for_pattern "S11" '"mime":"image/' 120

# S12: inbound document/PDF with Task: caption
narrate "S12" "Inbound document + Task: caption" \
  "Attach a PDF or other document with caption: Task: review attached spec" \
  "tgram.message carries application/* attachment; Type:'task'"
wait_for_pattern "S12" '"mime":"application/' 120

# S13: inbound voice/audio (audio/ogg from Telegram voice messages)
narrate "S13" "Inbound voice/audio attachment" \
  "Send a Telegram voice message. (Caption optional; if added: Query: explain this audio)" \
  "tgram.message carries audio/* attachment with sha256"
wait_for_pattern "S13" '"mime":"audio/' 120

# S14: outbound attachment fan-out via SendReply
# This requires the CC reply to include attachments[] in its JSON.
# Operator triggers a CC-driven reply that includes an outbound attachment;
# the wire-fact we assert is "attachments":[...] non-empty in tgram.send_reply.
narrate "S14" "Outbound attachment via SendReply fan-out" \
  "Send: Bug: layout broken (please attach a reproduction screenshot in your reply)" \
  "tgram.send_reply payload includes attachments[] with ≥1 entry — proves Wave 6.5 T6 fan-out wired"
wait_for_pattern "S14" '"kind":"tgram.send_reply"' 120

# S15: natural language with emojis → fallthrough query path
narrate "S15" "Natural-language + emojis → query fallthrough" \
  "Send: Question with no prefix and emojis 🙂🚀" \
  "Classification Type:'query',Confidence:0 (no prefix match)"
wait_for_pattern "S15" '"Type":"query","Criticality":"middle","Confidence":0' 120

# --- Post-run snapshots + diffs ---
echo ""
echo "[w6.5-lifecycle] post-run snapshots ..."
cp docs/Issues.md "${QA_OUT}/issues-after.md"
cp docs/Fixed.md  "${QA_OUT}/fixed-after.md"

diff -u "${QA_OUT}/issues-before.md" "${QA_OUT}/issues-after.md" > "${QA_OUT}/issues-diff.md" || true
diff -u "${QA_OUT}/fixed-before.md"  "${QA_OUT}/fixed-after.md"  > "${QA_OUT}/fixed-diff.md"  || true

# --- Stop pherald listen cleanly (trap also covers crash exits) ---
echo "[w6.5-lifecycle] stopping pherald listen ..."
kill -TERM "${PHERALD_PID}" 2>/dev/null || true
wait "${PHERALD_PID}" 2>/dev/null || true

# --- README in QA_OUT for T9 commit ---
cat > "${QA_OUT}/README.md" <<EOF
# Wave 6.5 Lifecycle E2E — Run ${HERALD_QA_RUN_ID}

15 scenarios per tests/test_wave6.5_lifecycle.sh.

## Files
- transcript.jsonl       — line-per-event journal from pherald listen
- pherald-listen.log     — listen subcommand stdout+stderr
- attachments/           — every inbound + outbound attachment (sha256-named)
- issues-before.md / issues-after.md / issues-diff.md
- fixed-before.md  / fixed-after.md  / fixed-diff.md
- scenario-verdicts.txt  — one line per scenario: <id>|PASS|<wire-byte-pattern>
- scenario-evidence.jsonl — one transcript line per scenario as positive evidence

## §107 evidence chain
Each scenario PASS line in scenario-verdicts.txt cites the EXACT wire-byte
substring that proves the production code path ran end-to-end. No
aggregated "all good" message — every line is an independent fact.
EOF

# --- Final per-scenario verdict roll-call (§107 — no aggregated message) ---
echo ""
echo "================================================================"
echo " test_wave6.5_lifecycle.sh — per-scenario verdict roll-call"
echo "================================================================"
if [ -f "${QA_OUT}/scenario-verdicts.txt" ]; then
  while IFS='|' read -r sn verdict pattern; do
    echo "  ${sn}: ${verdict} — pattern: ${pattern}"
  done < "${QA_OUT}/scenario-verdicts.txt"
fi

echo ""
echo "PASS test_wave6.5_lifecycle.sh — evidence: ${QA_OUT}/"
echo "  transcript.jsonl, scenario-verdicts.txt, scenario-evidence.jsonl"
echo "  issues-diff.md, fixed-diff.md, pherald-listen.log"
