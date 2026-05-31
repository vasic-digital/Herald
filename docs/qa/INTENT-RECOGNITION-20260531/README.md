<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# QA Evidence — Three-Tier Intent Recognition (INTENT-RECOGNITION-20260531)

Run-id evidence for the three-tier intent-resolution feature
(`docs/design/INTENT_RECOGNITION.md`). Per §107.x every shipped feature carries
a recorded, auditable runtime transcript; this directory is that transcript for
the intent/command-recognition + clarify-and-tag work.

## Feature

`pherald` inbound now resolves every subscriber message to exactly one action
via three tiers, first-success-wins:

| Tier | Mechanism | Code |
|---|---|---|
| TIER 1 | deterministic `CommandRecognizer` — plain NL → action, no LLM round-trip | `pherald/internal/inbound/command_recognizer.go` |
| TIER 2 | Claude Code envelope instruction — map NL to command set, return `action=clarify` rather than guess | `commons_messaging/dispatch/claude_code/claude_code.go` (`intentInferenceInstruction`) |
| TIER 3 | `clarify` action — reply tags the sender `@<username>` + asks a precise question | `pherald/internal/inbound/dispatcher.go` (`actClarify` / `clarifyTag`) |

Users never need command syntax. Tier 1 fast-paths confident matches; ambiguous
messages defer to the LLM (Tier 2); genuinely undeterminable intent is never
dropped — Tier 3 tags the user and asks (`@<sender> <specific question>`).

## Artifacts

| File | What it proves |
|---|---|
| `tier1_truth_table.txt` | `TestCommandRecognizer_TruthTable` — ~20 NL messages → expected (action, fields), INCLUDING conservative negatives that MUST return no-match (vague "look at the thing", targetless "close it", unknown status, no-title issue.open, bare ATM mention). |
| `tier2_envelope_instruction.txt` | `TestFormatEnvelope_IntentInferenceInstruction` — the rendered `<<<HERALD-DISPATCH-v1>>>` envelope contains the INTENT RECOGNITION block, the `action=clarify` directive, the `DO NOT guess` rule, and the command-set mapping, ordered before the structured marker. |
| `tier3_clarify_e2e.txt` | Tier-3 E2E through the real `Dispatcher.Handle` path with the production recording sink — an ambiguous message yields a clarify reply whose body is EXACTLY `@<sender> <specific question>`; plus unknown-sender + nil-resolver fallbacks and the empty-question loud-error. Includes `TestClearCommand_DoesNotTriggerClarify` (the NEGATIVE: a clear "close ATM-9" routes to `item.update` via Tier 1 and NEVER triggers clarify). |
| `clarify_body_evidence.txt` | The captured EXACT recording-sink clarify body showing the `@carol` tag + the non-generic candidate-intent question, threaded to the original message. |
| `mutation_gate.txt` | `tests/test_intent_mutation_meta.sh` — paired §1.1 mutation proof, 3 PASS / 0 FAIL (M1 Tier-1 confidence guard, M2 Tier-3 @sender tag, + §107.y quiescence). |

## Captured clarify reply body (anti-bluff core evidence)

Inbound (ambiguous): `hey can you do the ATM-9 thing` — sender `carol` on `tgram`.

Tier 1 declines (no confident imperative+target) → Tier 2 LLM returns
`action=clarify` → Tier 3 handler emits (captured verbatim from the recording
sink):

```
@carol did you want to close ATM-9, reassign it, or just get its status?
```

The user is **tagged** (`@carol`) and **asked a specific question** naming the
candidate intents — never ignored, never required to learn syntax.

## How to reproduce

```bash
go test -race -count=1 ./pherald/internal/inbound/... ./pherald/internal/workflow/...
go test -race -count=1 -run Format ./commons_messaging/dispatch/claude_code/...
bash tests/test_intent_mutation_meta.sh
```

All green; mutation gate reports `3 PASS / 0 FAIL`.
