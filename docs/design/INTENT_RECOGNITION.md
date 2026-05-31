<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — Intent Recognition & Clarification (DESIGN CONTRACT)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-31 |
| Status | active — implementation contract for the intent/command-recognition feature |
| Authority | Mandatory rule restated in HelixConstitution Constitution.md/CLAUDE.md/AGENTS.md/QWEN.md (inherited per §11.4.35) |

Operator mandate (2026-05-31): **users must NOT need to know command syntax** (no
`COMMAND: …`). They send a clear natural-language message; the System determines the
intent. The System recognizes the commands it has; if none matches it infers the exact
intent; if it is *totally unable* it **replies, tags the user (`@user …`), and asks to
clarify precisely**. We MUST always do our best to determine exact intent so we never
annoy end users. This is a CORE part of the System.

## 1. Three-tier intent resolution (the mandatory discipline)

Every inbound subscriber message is resolved to exactly one action via three tiers, in
order — the first that succeeds wins:

```
TIER 1  command recognition   — a deterministic CommandRecognizer maps clear
                                natural-language commands to a structured action
                                WITHOUT an LLM round-trip (fast-path; no "COMMAND:"
                                prefix needed). Matches → that action.
TIER 2  intent inference      — when no command matches, the Claude Code dispatch
                                (the LLM) infers the intent from the message and
                                returns a <<<HERALD-REPLY>>> action. The envelope
                                INSTRUCTS the LLM to recognize Herald's command set,
                                map natural language to the right action, and NEVER
                                guess.
TIER 3  clarify (fallback)    — when neither a command nor a confident intent can be
                                determined, the resolution is action="clarify": the
                                System REPLIES to the original message, TAGS the sender
                                (@username, via the §11.4.104 IdentityResolver), and
                                asks a precise clarifying question. No guessing, no
                                silent drop.
```

Tier 3 is the anti-annoyance guarantee: the user is never ignored and never has to learn
syntax — at worst they get a friendly, specific "@user, did you mean X or Y?".

## 2. The command set Tier 1 recognizes (natural-language → action)

The recognizer maps unambiguous imperatives (with an `ATM-NNN`/item id where relevant) to
the EXISTING inbound actions. No special prefix; case-insensitive; tolerant of phrasing:

| Natural-language intent (examples) | action | fields |
|---|---|---|
| "close ATM-123", "mark ATM-5 fixed/done/resolved" | `item.update` | status=closed/fixed |
| "set ATM-9 to in progress", "ATM-9 is blocked" | `item.update` | status=<parsed> |
| "assign ATM-5 to @bob", "give ATM-5 to @bob" | `item.update` | assigned_to=@bob |
| "open a bug: <title>", "create a task: <title>", "new feature request: <title>" | `issue.open` | type+title (created_by=sender) |
| "investigate ATM-7", "look into ATM-7" | `investigation.start` | atm_id |
| "status of ATM-9?", "what's ATM-9?" | `reply` (query) | atm_id |
| anything conversational / a question | `reply` | — |
| ambiguous / unparseable intent | `clarify` | question |

The recognizer is deliberately CONSERVATIVE: it only fast-paths a match it is confident
about (a clear imperative verb + a resolvable target). Everything else falls to Tier 2
(the LLM), and only genuine ambiguity reaches Tier 3. False command-matches are worse than
a deferral to the LLM, so when in doubt the recognizer returns "no match".

## 3. The `clarify` action (new) — contract

`<<<HERALD-REPLY>>>` gains `action: "clarify"`:

```json
<<<HERALD-REPLY>>> {"action":"clarify","question":"did you want to close ATM-9, reassign it, or just get its status?"}
```

Handler (`pherald/internal/inbound`): on `action=clarify`, send a reply to the original
message (`Replier.SendReply`, quoting/threading the original) whose body is:
`@<sender-username> <question>` — the sender resolved to their per-channel `@username` via
the §11.4.104 `IdentityResolver.UsernameFor(senderHandle, channel)` (fall back to the raw
sender handle if no alias). The question MUST be specific (name the candidate intents),
never a generic "I didn't understand".

The LLM is instructed (envelope/system prompt) to RETURN `action=clarify` with a precise
question whenever it cannot determine the intent — rather than guess an action. (§11.4.6
no-guessing applies: a wrong action is worse than a clarifying question.)

## 4. Envelope instruction (Tier 2 wiring)

`FormatEnvelope` (the `<<<HERALD-DISPATCH-v1>>>` block) gains a short instruction block
telling the LLM: (a) the user speaks plain language — no command syntax; (b) recognize
Herald's command set (§2) and map to the right action; (c) if you cannot determine the
intent with confidence, return `action=clarify` with a precise clarifying question naming
the candidate intents — do NOT guess. This is additive to the existing reply-format note.

## 5. Wiring points

- `pherald/internal/inbound`: a `CommandRecognizer` (Tier 1) tried BEFORE the LLM dispatch;
  on a confident match it produces the action directly; otherwise the existing Claude Code
  dispatch runs (Tier 2). The parsed `Reply.Action` may now be `clarify` (Tier 3) →
  routed to a new `clarifyHandler` that tags the sender and asks.
- `commons_messaging/dispatch/claude_code`: the envelope instruction (§4).
- The clarify reply reuses the §11.4.104 participant tagging (`@username` resolution).

## 6. Anti-bluff testing mandate (MANDATORY — §107 / Helix §11.4)

Every tier ships unit + integration + **E2E** + **full-automation** tests with **real
captured evidence** (no metadata-only PASS, no false +/−):
- Tier 1: a truth-table of natural-language messages → expected action+fields (and the
  conservative negatives that MUST fall through to "no match").
- Tier 3 E2E: an ambiguous message drives a real dispatch whose recording-sink reply body
  is EXACTLY `@<sender> <specific question>` — proving the user is tagged + asked, not
  ignored. A NEGATIVE: a clear command does NOT trigger clarify.
- A paired §1.1 mutation gate: break the recognizer's confidence guard (so it false-matches)
  OR drop the clarify tag → a test MUST FAIL.
- Evidence committed under `docs/qa/<run-id>/`.

## 7. Constitution authority (MANDATORY, inherited)

Restated as a mandatory constraint in HelixConstitution Constitution.md + CLAUDE.md +
AGENTS.md + QWEN.md (root, inherited per §11.4.35), then in Herald + ATMOSphere docs: the
System MUST determine intent from natural language (no required command syntax), recognize
its command set, infer intent when no command matches, and — only when totally unable —
reply-tag-and-ask the user to clarify precisely. Never guess an action; never ignore a
message. The anti-bluff covenant (§107 / §11.4) is a hard precondition of every PASS.
