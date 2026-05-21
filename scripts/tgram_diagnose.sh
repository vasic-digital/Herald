#!/usr/bin/env bash
# tgram_diagnose.sh — Telegram setup diagnostic for Herald HRD-011.
#
# Runs three Telegram Bot API checks and reports findings in a format
# operators can act on directly:
#
#   1. getMe          — proves the bot token is valid + surfaces the
#                       Privacy Mode setting (load-bearing: with Privacy
#                       Mode ON, the bot only sees commands + @-mentions
#                       + replies; plain group chat messages are invisible).
#   2. getWebhookInfo — confirms no webhook is intercepting updates that
#                       getUpdates would otherwise return. A webhook +
#                       getUpdates can't coexist; if a webhook is set,
#                       delete it via the printed deleteWebhook URL OR
#                       switch to webhook-mode reception (HRD-NNN, future).
#   3. getUpdates     — pulls the bot's pending updates and extracts the
#                       chat_id + chat type + chat name from each. Empty
#                       result with Privacy Mode ON is the canonical "I
#                       added the bot to a group but messages don't reach
#                       it" failure mode — see TELEGRAM.md §"Step 2.5".
#
# Requires: HERALD_TGRAM_BOT_TOKEN in env. The script reads ONLY from
# the env var — never from a positional arg or file — so the token does
# not leak into shell history / process listings (modulo the env-var-leak
# the OS already permits).
#
# Per Universal §11.4.10: this script never writes the token to disk.
# Per §107: empty getUpdates returns are surfaced with the diagnostic
# message telling operators exactly what's wrong (Privacy Mode) and how
# to fix it — not "ok we polled, nothing happened."
#
# Usage:
#   export HERALD_TGRAM_BOT_TOKEN='...'   # already in .zshrc per OPERATOR_CREDENTIALS.md
#   bash scripts/tgram_diagnose.sh

set -uo pipefail

if [ -z "${HERALD_TGRAM_BOT_TOKEN:-}" ]; then
    echo "ERROR: HERALD_TGRAM_BOT_TOKEN env var not set." >&2
    echo "       See docs/guides/messengers/TELEGRAM.md §\"Step 3\" for setup." >&2
    exit 2
fi

if ! command -v curl >/dev/null 2>&1; then
    echo "ERROR: curl not on PATH." >&2
    exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
    echo "ERROR: python3 not on PATH (used to parse JSON)." >&2
    exit 2
fi

TOKEN="${HERALD_TGRAM_BOT_TOKEN}"
API="https://api.telegram.org/bot${TOKEN}"

echo "=== 1. getMe — token validity + Privacy Mode setting ==="
me_json="$(curl -s "${API}/getMe")"
echo "${me_json}" | python3 -c '
import sys, json
d = json.load(sys.stdin)
if not d.get("ok"):
    print("  ERROR: " + d.get("description","unknown")); sys.exit(1)
me = d["result"]
print("  bot username:                @" + me["username"])
print("  bot id:                      " + str(me["id"]))
print("  bot name:                    " + me["first_name"])
print("  can_join_groups:             " + str(me.get("can_join_groups", False)))
print("  can_read_all_group_messages: " + str(me.get("can_read_all_group_messages", False)))
if not me.get("can_read_all_group_messages", False):
    print()
    print("  ! Privacy Mode is ON. In groups, the bot only sees:")
    print("    - commands (/...)")
    print("    - @-mentions of itself")
    print("    - replies to its own messages")
    print("    Plain chat messages from group members are NOT delivered.")
    print("    See TELEGRAM.md §\"Step 2.5\" to disable Privacy Mode in @BotFather.")
'
echo ""

echo "=== 2. getWebhookInfo — confirm getUpdates is the live channel ==="
curl -s "${API}/getWebhookInfo" | python3 -c '
import sys, json
d = json.load(sys.stdin)
if not d.get("ok"):
    print("  ERROR: " + d.get("description","unknown")); sys.exit(1)
r = d.get("result", {})
url = r.get("url", "")
pending = r.get("pending_update_count", 0)
if url:
    print("  webhook configured -> " + url[:60] + ("..." if len(url) > 60 else ""))
    print("  ! webhook + getUpdates cannot coexist. Either:")
    print("    (a) delete the webhook via:")
    print("        https://api.telegram.org/bot<TOKEN>/deleteWebhook")
    print("    (b) switch Herald to webhook-mode (HRD-NNN, future).")
else:
    print("  no webhook configured (good — getUpdates path is clear)")
print("  pending_update_count: " + str(pending))
'
echo ""

echo "=== 3. getUpdates — list of chats the bot has received updates from ==="
curl -s "${API}/getUpdates" | python3 -c '
import sys, json
d = json.load(sys.stdin)
if not d.get("ok"):
    print("  ERROR: " + d.get("description","unknown")); sys.exit(1)
results = d.get("result", [])
print("  updates received: " + str(len(results)))
if not results:
    print()
    print("  EMPTY. The bot has not received any messages it can see.")
    print()
    print("  Common causes + fixes:")
    print("    (a) Privacy Mode is ON (see getMe output above) AND no")
    print("        command/@-mention/reply has been sent in any group.")
    print("        FIX (quick):    in the group, send: @<bot-username> hi")
    print("        FIX (proper):   in @BotFather → /mybots → <your bot> →")
    print("                        Bot Settings → Group Privacy → Turn off,")
    print("                        then re-add the bot to the group, then")
    print("                        send any message.")
    print("    (b) Bot has not been added to any chat yet.")
    print("        FIX: add the bot to a group/channel OR start a DM with it,")
    print("             then send any message.")
    print("    (c) Updates were consumed by a previous getUpdates call.")
    print("        Telegram acks updates after a poll. If you already polled")
    print("        once, send another message to repopulate the queue.")
else:
    seen = set()
    for r in results:
        for key in ("message","edited_message","channel_post","my_chat_member","chat_join_request"):
            msg = r.get(key)
            if msg and "chat" in msg:
                chat = msg["chat"]
                cid = chat.get("id")
                ctype = chat.get("type")
                title = chat.get("title") or chat.get("username") or chat.get("first_name","(no name)")
                if cid not in seen:
                    seen.add(cid)
                    print("  chat_id=" + str(cid) + "  type=" + str(ctype) + "  name=" + str(title) + "  (source: " + key + ")")
    if seen:
        print()
        print("  Use any chat_id above as HERALD_TGRAM_CHAT_ID.")
        print("  For groups/channels, the id is NEGATIVE (e.g. -1001234567890).")
        print("  For DMs, the id is POSITIVE.")
'
