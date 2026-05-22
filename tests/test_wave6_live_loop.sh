#!/usr/bin/env bash
#
# Wave 6 closed-loop e2e — §107 watershed for the inbound runtime.
#
# Flow (per operator architecture mandate 2026-05-22):
#   1. Subscriber (operator) types a message in the configured Telegram chat
#   2. pherald listen polls getUpdates, filters bot-self, dispatches to CC
#   3. CC (Opus, headless, session = HERALD_PROJECT_NAME) processes envelope
#   4. Reply lands in chat via tgram.SendReply with reply_to_message_id
#      pointing at the original subscriber message
#
# §107 anti-bluff: every assertion is against real Telegram getUpdates bytes
# + real CC subprocess + real pherald process state. Cannot be faked.
#
# SKIP-with-reason if creds absent — never fabricates evidence.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"
cd "${REPO_ROOT}"

skip_with_reason() { echo "SKIP: $1"; exit 0; }
fail() { echo "FAIL: $*"; exit 1; }

# --- Pre-flight gates ---
[ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ]  || skip_with_reason "HERALD_TGRAM_BOT_TOKEN unset"
[ -n "${HERALD_TGRAM_CHAT_ID:-}" ]    || skip_with_reason "HERALD_TGRAM_CHAT_ID unset"

# claude CLI: env override wins; else $PATH lookup
CLAUDE_BIN="${HERALD_CLAUDE_BIN:-claude}"
command -v "${CLAUDE_BIN}" >/dev/null || skip_with_reason "claude CLI not found at ${CLAUDE_BIN}"

command -v jq   >/dev/null || skip_with_reason "jq not installed (required for JSON parsing)"
command -v curl >/dev/null || skip_with_reason "curl not installed"

# --- Build pherald ---
echo "[wave6-live-loop] building pherald..."
PHERALD_BIN="$(mktemp -t pherald-w6.XXXXXX)"
trap 'rm -f "${PHERALD_BIN}"' EXIT
go build -o "${PHERALD_BIN}" ./pherald/cmd/pherald

# --- Capture pre-existing update offset (so we don't pick up stale messages) ---
echo "[wave6-live-loop] capturing baseline getUpdates offset..."
BASE_RESP=$(curl -sS --max-time 10 \
  "https://api.telegram.org/bot${HERALD_TGRAM_BOT_TOKEN}/getUpdates?limit=100" || true)
BASE_OK=$(echo "${BASE_RESP}" | jq -r '.ok // false')
[ "${BASE_OK}" = "true" ] || fail "getUpdates baseline returned ok=false (resp tail: $(echo "${BASE_RESP}" | tail -c 200))"
BASELINE_MAX=$(echo "${BASE_RESP}" | jq -r '[.result[].update_id] | (max // 0)')

echo "[wave6-live-loop] baseline max update_id=${BASELINE_MAX}"
echo
echo "================================================================"
echo " PRE-CONDITION: operator MUST type a single message in the chat "
echo "                (chat_id=${HERALD_TGRAM_CHAT_ID}) NOW."
echo "                Script polls for 60s."
echo "================================================================"
echo

# --- Wait for new subscriber message ---
ORIGINAL_MSG_ID=""
ORIGINAL_TEXT=""
NEW_OFFSET=$((BASELINE_MAX + 1))
deadline=$(( $(date +%s) + 60 ))
while [ "$(date +%s)" -lt "${deadline}" ]; do
  resp=$(curl -sS --max-time 10 \
    "https://api.telegram.org/bot${HERALD_TGRAM_BOT_TOKEN}/getUpdates?offset=${NEW_OFFSET}&timeout=5&allowed_updates=%5B%22message%22%5D" \
    || true)
  ok=$(echo "${resp}" | jq -r '.ok // false')
  if [ "${ok}" != "true" ]; then
    sleep 2; continue
  fi
  # Pick the FIRST message from a non-bot sender in our configured chat.
  MATCH=$(echo "${resp}" | jq -c \
    --arg cid "${HERALD_TGRAM_CHAT_ID}" \
    '.result[]
     | select(.message != null)
     | select((.message.chat.id|tostring) == $cid)
     | select((.message.from.is_bot // false) == false)
     | {update_id, message_id: .message.message_id, text: (.message.text // .message.caption // "")}' \
    | head -n1)
  if [ -n "${MATCH}" ] && [ "${MATCH}" != "null" ]; then
    ORIGINAL_MSG_ID=$(echo "${MATCH}" | jq -r '.message_id')
    ORIGINAL_TEXT=$(echo "${MATCH}"  | jq -r '.text')
    OBSERVED_UPDATE_ID=$(echo "${MATCH}" | jq -r '.update_id')
    echo "[wave6-live-loop] observed subscriber message_id=${ORIGINAL_MSG_ID} (update_id=${OBSERVED_UPDATE_ID})"
    echo "[wave6-live-loop] text preview: $(printf '%s' "${ORIGINAL_TEXT}" | head -c 80)"
    # NOTE: we do NOT ack this update_id ourselves — pherald listen will do that.
    break
  fi
  sleep 2
done

[ -n "${ORIGINAL_MSG_ID}" ] || fail "no subscriber-typed message observed in 60s (chat_id=${HERALD_TGRAM_CHAT_ID})"

# --- Start pherald listen ---
echo "[wave6-live-loop] starting pherald listen..."
LISTEN_LOG="$(mktemp -t pherald-w6.log.XXXXXX)"
"${PHERALD_BIN}" listen > "${LISTEN_LOG}" 2>&1 &
LISTEN_PID=$!
trap 'kill -TERM "${LISTEN_PID}" 2>/dev/null || true; rm -f "${PHERALD_BIN}" "${LISTEN_LOG}"' EXIT

# Give pherald a moment to boot (telebot.NewBot + getMe).
sleep 3

if ! kill -0 "${LISTEN_PID}" 2>/dev/null; then
  echo "FAIL: pherald listen exited prematurely. Log tail:"
  tail -n 40 "${LISTEN_LOG}"
  exit 1
fi

echo "[wave6-live-loop] pherald listen PID=${LISTEN_PID}, polling for reply..."
echo "[wave6-live-loop] waiting up to 120s for a bot message with reply_to_message_id=${ORIGINAL_MSG_ID}..."

# --- Wait for reply ---
REPLY_OK=0
REPLY_MSG_ID=""
deadline=$(( $(date +%s) + 120 ))
while [ "$(date +%s)" -lt "${deadline}" ]; do
  resp=$(curl -sS --max-time 10 \
    "https://api.telegram.org/bot${HERALD_TGRAM_BOT_TOKEN}/getUpdates?limit=100" \
    || true)
  ok=$(echo "${resp}" | jq -r '.ok // false')
  if [ "${ok}" != "true" ]; then sleep 3; continue; fi

  # The reply must:
  #   1. live in our chat
  #   2. have reply_to_message.message_id == ORIGINAL_MSG_ID
  #   3. come from a bot (Telegram's API marks bot messages with from.is_bot=true)
  MATCH=$(echo "${resp}" | jq -c \
    --arg cid "${HERALD_TGRAM_CHAT_ID}" \
    --argjson orig "${ORIGINAL_MSG_ID}" \
    '.result[]
     | select(.message != null)
     | select((.message.chat.id|tostring) == $cid)
     | select((.message.reply_to_message.message_id // 0) == $orig)
     | select((.message.from.is_bot // false) == true)
     | {message_id: .message.message_id, text: (.message.text // "")}' \
    | head -n1)
  if [ -n "${MATCH}" ] && [ "${MATCH}" != "null" ]; then
    REPLY_MSG_ID=$(echo "${MATCH}" | jq -r '.message_id')
    REPLY_TEXT=$(echo "${MATCH}"   | jq -r '.text')
    REPLY_OK=1
    break
  fi
  sleep 3
done

# --- Verdict ---
kill -TERM "${LISTEN_PID}" 2>/dev/null || true
wait "${LISTEN_PID}" 2>/dev/null || true

echo
if [ "${REPLY_OK}" = "1" ]; then
  echo "PASS: Wave 6 closed-loop"
  echo "PASS-evidence: subscriber message_id=${ORIGINAL_MSG_ID}"
  echo "PASS-evidence: bot reply message_id=${REPLY_MSG_ID}"
  echo "PASS-evidence: bot reply text preview: $(printf '%s' "${REPLY_TEXT}" | head -c 120)"
  echo "PASS-evidence: pherald log tail:"
  tail -n 30 "${LISTEN_LOG}"
  exit 0
else
  echo "FAIL: no reply with reply_to_message_id=${ORIGINAL_MSG_ID} observed within 120s"
  echo "pherald log tail:"
  tail -n 60 "${LISTEN_LOG}"
  exit 1
fi
