<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Forensic finding — Telegram bot-to-bot message invisibility (qaherald-auto automation constraint)

| Field | Value |
|---|---|
| Date | 2026-05-27 |
| Discovered during | qaherald-auto live 15-scenario run (HRD-101), run-id `2026-05-27T03-30-22-w6.5auto` |
| Severity | Architecture-blocking for the 2nd-bot automation approach (NOT a Herald code defect) |
| Status | documented; pivot to MTProto user-client (Wave 7 MessengerClient impl) |

## Summary

The qaherald-auto automated lifecycle test was designed to drive pherald's inbound
runtime by having a **second Telegram bot** (`@pherald_qa_bot`) post the 15 scenario
messages into the ATMOSphere Development group, which pherald-bot (`@atmosphere_worker_bot`)
would then receive via `getUpdates` and process. The run produced **0 PASS / 14 FAIL / 1 SKIP**
— every scenario failed with `await-reply: context deadline exceeded`.

Root-cause investigation proved this is **not a Herald defect**. It is a hard Telegram
platform rule:

> **Bots cannot see messages from other bots in groups — regardless of privacy mode,
> @mentions, or any API flag.**

## Evidence (physical, §107 / §11.4.87)

1. **pherald-side transcript empty.** `docs/qa/HRD-101-lifecycle-<run>-pherald/transcript.jsonl`
   had **0 lines** after 14 scenario sends — pherald's `getUpdates` long-poll received nothing
   to dispatch. pherald listen log showed only boot lines, no `inbound dispatched`.

2. **Plain message probe.** qa-bot sent a plain message (`sent_message_id=14`) to the group;
   pherald-bot's `getUpdates?limit=10` returned `0` updates 3 s later (pherald listen stopped
   first to rule out queue-draining).

3. **@mention probe.** qa-bot sent `@atmosphere_worker_bot mention-probe …` (`sent_message_id=15`);
   pherald-bot's `getUpdates` STILL returned `0`. @mentions normally bypass privacy mode for
   human senders — but bot-sender messages are never relayed to other bots.

4. **Bot-level flag was correct.** Both bots reported `can_read_all_group_messages: true`
   via `getMe`. The bot-level privacy flag being disabled is necessary-but-insufficient; the
   bot-to-bot invisibility is a separate, absolute platform restriction.

5. **Contrast — HRD-100 worked.** The Wave 6 closed-loop (HRD-100, 2026-05-22) succeeded
   because the sender was a **human user** (`milos85vasic`, user-id `2057253161`), and
   user→bot messages ARE delivered. Real transport is proven: inbound `message_id=25` →
   Claude Opus reply → outbound `message_id=26` with `reply_to_message_id=25`.

## What this means for the automation strategy

The `MessengerClient` interface (`qaherald/internal/messenger/`) + the lifecycle scenario
engine + orchestrator + report generator (`qaherald/internal/lifecycle/`) are all SOUND —
12 hermetic unit tests PASS, the preflight gates work (G1 was hardened to a real
`getChatMember` membership proof in commit `b45e45d`). The ONLY broken assumption was the
transport binding: a 2nd bot cannot impersonate a subscriber to another bot.

Two viable paths to full automation of the inbound (subscriber-types) leg:

### Path A — MTProto user client (RECOMMENDED; real Telegram channel, full automation)
- A real Telegram **user account** (not a bot) posts the scenario messages via the MTProto
  protocol (Go library `github.com/gotd/td`).
- pherald-bot receives them (user→bot is delivered).
- Requires operator provisioning: `app_id` + `app_hash` from <https://my.telegram.org>, plus
  a one-time interactive phone-number login that produces a reusable `session` file.
- Folds into **Wave 7** (genericize messenger framework) as a new `MessengerClient` impl —
  the lifecycle scenario engine, orchestrator, report, and 12 hermetic tests are reused
  unchanged; only the transport impl swaps from bot-token to MTProto-user-session.

### Path B — Telegram-API-double (full pipeline automation, no operator setup)
- qaherald stands up a local HTTP server mimicking the Telegram Bot API; pherald-bot is
  pointed at it via a base-URL override. The double serves synthetic `getUpdates` (the 15
  scenario messages) and records `sendMessage`/`sendPhoto`/etc. calls.
- Exercises the FULL pherald inbound pipeline (real classifier, real Claude Opus dispatch,
  real `docs/Issues.md` mutation, real reply-JSON formatting) — everything EXCEPT the
  external Telegram transport.
- The real transport is separately proven by HRD-100 (human round-trip). Per §11.4.27, mocking
  an external third-party API boundary for integration breadth is acceptable when the real
  transport has its own captured-evidence proof.
- No operator credentials required → immediately autonomously actionable.

## Decision

Operator to choose Path A (real-channel, needs my.telegram.org creds) and/or Path B
(autonomous, transport-mocked). Both reuse the qaherald-auto scenario engine. Recommended:
Path B now (autonomous breadth proof) + Path A as the Wave 7 real-transport upgrade once
operator supplies MTProto credentials.

## Non-defect attestation

No `docs/Issues.md` / `docs/Fixed.md` mutation occurred during the failed run (verified via
`git diff` — pherald received nothing, so no lifecycle action fired). The qaherald-auto code,
the pherald inbound runtime, the classifier, the issue-opener, and the migration logic are all
correct and unit-test-proven. The failure was a transport-layer architecture mismatch in the
test harness, now documented and pivoted.
