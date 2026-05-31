<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# QA evidence — Participant Attribution (inbound) + Notification Tagging (outbound), end-to-end through pherald

| Field | Value |
|---|---|
| Run-id | PARTICIPANT-ATTRIBUTION-20260531 |
| Feature | PARTICIPANT_ATTRIBUTION §2 (inbound attribution) + §3/§5 (outbound @-mention tagging) wired end-to-end through `pherald` |
| Contract | `docs/design/PARTICIPANT_ATTRIBUTION.md` |
| Covenant | §107.x docs/qa evidence mandate; §107 / Helix §11.4 anti-bluff |

## What this proves

This directory is the positive, captured runtime evidence that the participant
attribution + notification-tagging feature **actually works through the real
pherald inbound dispatcher and the real outbound `runner.ChannelDispatcher`** —
not a metadata-only / absence-of-error PASS.

### 1. Inbound attribution (`inbound_attribution_test.go`)

`pherald/internal/inbound/dispatcher.go` now threads a
`commons.IdentityResolver` into the `item.update` action. When an inbound
message opens/updates a workable item, the dispatcher injects `created_by` +
`assigned_to` into the field map handed to the real `ItemMutator` boundary.
The unit tests assert the **exact field map** the dispatcher built (the DB
boundary), per §2:

- a message from `@someuser` opening an item → `created_by == "@someuser"`
  (resolved from the sender's stamped Telegram `@username`);
- a System/Claude-opened item (LLM payload declares `created_by:"Claude"`) →
  `created_by == "Claude"` (never overwritten with the sender);
- default `assigned_to == OperatorHandle()` (from
  `HERALD_TGRAM_OPERATOR_USERNAME` via `OperatorHandleFromEnv` / `MemoryResolver`);
- an explicit `assign:@bob` directive in the body → `assigned_to == "@bob"`
  (both `assign:@bob` and `assign @bob` spellings recognised).

Captured output: [`inbound_attribution_test_output.txt`](inbound_attribution_test_output.txt) — all PASS.

### 2. Outbound tagging E2E (`outbound_tagging_e2e_test.go`)

`pherald/internal/workflow/workflow.go` gains a `NewTaggingNotifier` that, for
each dispatched change, computes `commons.MentionsFor(createdBy, assignedTo,
operator, channel, resolver)` and prepends the resolved per-channel
`@username`s to the outbound body via `tgram.PrependMentions`. The E2E test
drives **real item-change events through the real `runner.ChannelDispatcher` →
`recordingChannel`** (a real `commons.Channel` sink, reused verbatim from
`workflow_test.go`, registered under the `tgram` ChannelID) and asserts the
**captured body string** for each cell of the §3 matrix.

Captured dispatched bodies (the literal sink output, operator = `@milos85vasic`):

```
body[0] (ATM-A  opened+assigned to Operator)     : "🔄 ATM-A status: Queued → In progress"
body[1] (ATM-B  opened by Operator, assigned @bob): "cc: @bob\n🔄 ATM-B status: Queued → In progress"
body[2] (ATM-C  opened by @carol, assigned Operator): "cc: @carol\n🆕 ATM-C created"
body[3] (ATM-D  assigned @dave, NO tgram alias)   : "🔄 ATM-D status: Queued → In progress"
```

Matrix verification:

| Case | created_by | assigned_to | Expected tag | Captured body | Result |
|---|---|---|---|---|---|
| (a) | Operator | Operator | **no @-mention** (negative case) | no `cc:` line | PASS — Operator never tagged |
| (b) | Operator | @bob (tgram alias) | `@bob` | `cc: @bob` | PASS |
| (c) | @carol (subscriber) | Operator | `@carol` | `cc: @carol` | PASS |
| (d) | Operator | @dave (NO tgram alias) | **no mention for dave** | no `cc:` line | PASS — skip-if-not-on-channel |

The **negative case (a)** is the load-bearing proof the operator is NEVER
self-pinged: created_by==Operator and assigned_to==Operator both skipped → the
body carries no `cc:` line at all.

Captured output: [`outbound_tagging_e2e_test_output.txt`](outbound_tagging_e2e_test_output.txt) — all PASS.

### 3. Anti-bluff mutation proof (`mutation_proof_FAIL.txt`)

To prove the matrix assertions are real (not vacuous), the operator-skip in
`commons.MentionsFor` was transiently removed (`if handle == operatorHandle
{ return }` deleted). With that one cell flipped, the operator gets wrongly
`@`-tagged and the E2E test **FAILS** — every `wantAbsent` operator assertion
goes RED (see the captured FAIL output). The mutation was reverted immediately
(§107.y working-tree quiescence — single foreground run, no residue committed);
the restored source PASSes again. A wrong matrix therefore cannot pass silently.

Captured output: [`mutation_proof_FAIL.txt`](mutation_proof_FAIL.txt) — FAIL (as expected, with the mutation; reverted after capture).

## How to reproduce

```bash
go test -race -count=1 -v ./pherald/internal/inbound/...  -run TestInboundAttribution
go test -race -count=1 -v ./pherald/internal/workflow/... -run 'TestNotifier_OutboundTagging_E2E|TestNotifier_NoTagging'
```
