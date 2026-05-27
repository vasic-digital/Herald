# HRD-023 — pherald PROJECT constitution bindings — §107.x round-trip transcript

| Field | Value |
|---|---|
| HRD | HRD-023 (v1.0.0 Batch B, unit 5) |
| Flavor | `pherald` (PROJECT) |
| Run-id | HRD-023-20260527T210354Z |
| Captured | 2026-05-27 (UTC) |
| Foundation | Batch-A `commons_constitution` Evaluator + Registry + ModeLadder + ConstitutionStore + AuditStore + typed emitter |
| Anchor | spec V3 §42.3 pherald rows + §42.5 step 6; Helix §11.4.85 (stress+chaos); Herald §107 / §107.x |

This transcript records a REAL end-to-end project-binding round-trip — a recorded
project-op outcome DETECTED by a registered pherald binding → EMITTED on the bus
as a typed constitution event → PERSISTED as a `constitution_state` row AND a
`constitution_audit` row → queryable via the same `ConstitutionStore.List` a
future `/v1/project` pull surface reads. Every value below is captured runtime
output from the package test suite running the REAL foundation (real MemoryBus,
real Memory store/ladder/audit, real emit) — NO mocks-of-mocks (§11.4.27).

## Round-trip 1 — §2 commit-push discipline → `.repo.safety.breach`

Subject (recorded by the upstream §43 HRD-029 commit-push command body; the
binding only CLASSIFIES it — NO real git/commit/push occurs):

```
Kind: commit-push
ID:   abc1234|entrypoint=false|lock_held=false
```

`Pipeline.EvaluateSubject(ctx, "§2", tenant, subject)` with §2 at ModeEnforce
(its §42.3 default) produced, in one transition-gated pass:

```
out.Decision   = fail        (commit made OUTSIDE the single locked entrypoint)
out.Transition = {Changed:true, FirstSeen:true}
out.Mode       = enforce
out.Emitted    = true        → 1× digital.vasic.herald.constitution.repo.safety.breach on the bus
out.Audited    = true        → 1× constitution_audit row (mode_at_emission=enforce)
```

Persisted side-effects (queried back from the live backends):

- `ConstitutionStore.List(tenant, {RuleID:"§2"})` → exactly 1 state row,
  `decision=fail`, `subject="abc1234|entrypoint=false|lock_held=false"`.
- `AuditStore.ListAudit(tenant, {RuleID:"§2"})` → exactly 1 audit row,
  `mode_at_emission=enforce`.
- `bus.Metrics().PublishedByType["...repo.safety.breach"]` = 1 (exact class —
  NOT a misrouted `.policy.violation`).

Test: `TestPipeline_RepoSafetyBreachRoundTrip` → **PASS**.

## Round-trip 2 — §11.4.55 reopens-history → `.policy.violation`

Subject (recorded by the upstream §43 HRD-049 reopen command body):

```
Kind: reopen
ID:   HRD-099|recorded=false        (Issues←Fixed reversal with NO docs/Reopens/HRD-099.md record)
```

`Pipeline.EvaluateSubject(ctx, "§11.4.55", tenant, subject)` (flipped to
ModeEnforce so the emit fires; §11.4.55 warn-by-default audits only):

```
out.Decision = fail
out.Emitted  = true   → 1× digital.vasic.herald.constitution.policy.violation on the bus
audit row    EmittedEventID != uuid.Nil   (IDEmitter path captured the exact fanned-out event id)
```

Test: `TestPipeline_ReopensPolicyViolation` → **PASS**. Proves the policy rows
(§11.4.36 install-upstreams + §11.4.55 reopens) route through the policy class,
NOT the repo-safety class, and that the audit row captures the real event id.

## Bespoke detector truth-tables (all PURE — classify recorded outcomes, never perform the op)

| Detector | Rule | Subject example | Verdict |
|---|---|---|---|
| commit-push-discipline | §2 | `abc1234\|entrypoint=true\|lock_held=true` | PASS |
| commit-push-discipline | §2 | `abc1234\|entrypoint=false\|lock_held=false` | FAIL (entrypoint bypassed) |
| commit-push-discipline | §2 | `abc1234\|entrypoint=true\|lock_held=false` | FAIL (no lock held) |
| commit-push-discipline | §2 | `abc1234` (no fields) | FAIL (refuse to silent-PASS) |
| submodule-propagation-order | §3 | `propagate\|order=inner-first\|inner_pushed=true` | PASS |
| submodule-propagation-order | §3 | `propagate\|order=parent-first\|inner_pushed=true` | FAIL (parent before inner) |
| submodule-propagation-order | §3 | `propagate\|order=inner-first\|inner_pushed=false` | FAIL (parent pins dangling SHA) |
| pre-push-fetch-guard | §11.4.71 | `main\|fetched=true\|integrated=true` | PASS |
| pre-push-fetch-guard | §11.4.71 | `main\|fetched=false\|integrated=false` | FAIL (no pre-push fetch) |
| pre-push-fetch-guard | §11.4.71 | `main\|fetched=true\|integrated=false` | FAIL (incoming not integrated) |
| fetch-before-edit | §11.4.37 | `main\|rebased=true` | PASS |
| fetch-before-edit | §11.4.37 | `main\|rebased=false` | FAIL (edit on stale tree) |

Tests: `TestDetector_CommitPushDiscipline`, `TestDetector_SubmodulePropagationOrder`,
`TestDetector_PrePushFetchGuard`, `TestDetector_FetchBeforeEdit` → all **PASS**.

## Determinism + concurrency evidence

```
go test -race -count=3 ./pherald/internal/bindings/...
ok  github.com/vasic-digital/herald/pherald/internal/bindings  1.629s
```

Race detector clean; `-count=3` deterministic green (§11.4.50). Stress + chaos
artefacts under `./stress_chaos/bindings/` (§11.4.85): 1200 concurrent §2
repo-breach evaluations, 0 errors, 1200 emits + 1200 audit rows (no lost events
under load), p99=0.905ms; emit-fault chaos → 100% of bus-drop faults surface via
`EvaluateSubject` error, 0 swallowed.

## §12 host-safety posture

Every detector is PURE: it classifies a fabricated recorded project-op outcome
string. NONE commits, pushes, force-pushes, runs git, configures remotes, or
touches the filesystem. The live project-op interception lives in the upstream
§43 command bodies (HRD-029/030/043/044/049/053) and is scope-locked to those
follow-ups.
