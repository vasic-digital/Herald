# HRD-156 T-A — ATMOSphere↔Herald outbound LIVE evidence

| Field | Value |
|---|---|
| Test | TestMTProto_ATMOSphere_SSoTChangeNotifiesGroup |
| Run ID | HRD-156-LIVE-20260530T132303Z |
| Date | 2026-05-30 |
| Result | **PASS LIVE** (8.18s) |

## What this proves (§107.x / §11.4.98 — real runtime, no bluff)

The full ATMOSphere↔Herald OUTBOUND flow end-to-end against REAL Telegram:
a workable-item created in the SQLite SSoT → `pherald watch` (real binary,
real fsnotify+WAL-poll) detected it → `commons_workable.Diff` → `pherald/internal/workflow`
rendered "🆕 <atm_id> created" → `runner.ChannelDispatcher` sent it via the bot →
the message was OBSERVED on the wire by a real MTProto user account (@milos85vasic,
user_id=2057253161) in the configured group chat.

- Unique nonce: `ATM-QALIVE-40681337` (the atm_id, present verbatim in the bot text).
- Observed: bot `message_id=211575` from `user_id=8823384001`, text `"🆕 ATM-QALIVE-40681337 created"`, chat `-4946584787`.
- NOT a "message was sent" log assertion — the bot's actual delivered message was read back via MTProto getHistory.

## Full automation (§11.4.98)
Self-driving end-to-end: builds the real pherald binary, spawns `pherald watch`,
mutates a temp SSoT, observes via MTProto. Single one-time prerequisite (outside
test execution): `qaherald mtproto login` (already bootstrapped; session valid).
Honest-SKIP per §11.4.3 when creds/session absent.

## Artefacts
- `atmosphere_outbound_live.log` — full -v transcript of the PASS.

## Remaining HRD-156 layers (next)
- T-B inbound (operator message → real SSoT CRUD), T-C exact-diff byte-assert,
  T-D/T-E §11.4.85 stress/chaos, T-F paired §1.1 mutation gate.
