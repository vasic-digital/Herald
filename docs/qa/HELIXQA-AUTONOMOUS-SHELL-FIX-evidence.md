<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# QA evidence — HelixQA autonomous executor `shell:` fix

**Fix:** helixqa `6434a00` — `pkg/autonomous/structured_executor.go` `performAction`
gained an `ActionTypeShell` case (it previously had none, so every `shell:`
bank step returned "Unknown action type: shell" and the structured executor
filed a FALSE-NEGATIVE "Test Case Failed" finding).

**Before (autonomous run, pre-fix):** all 10 `HRD-QA-CLI-*` cases produced
`Test Case Failed … Actual: Unknown action type: shell` tickets — even though
the same bank PASSES under `helixqa run`. A report claiming failure for a
command it never ran is a §107 anti-bluff defect.

**After (autonomous run, post-fix):** see `structured-results.txt` (captured
from `helixqa autonomous --project Herald --platforms desktop`):

```
[structured] Completed: 10 passed, 0 failed, 0 skipped (bank placeholders),
             10 total, 18 steps executed in 21.495s
```

All 10 CLI cases `✓ PASSED`, **zero** "Unknown action type", **zero**
false-negative tickets. The autonomous structured executor now runs each
`shell:` step via `sh -c`, captures the real exit code, and scores PASS only on
exit 0 — the SAME semantics as the bank-driven `executeDesktopShellSteps`. The
two HelixQA execution paths now agree.

**Unit regression:** `pkg/autonomous/shell_action_test.go::TestPerformAction_Shell`
(exit0→pass, exit7→fail, grep-pass, grep-fail).

**Note:** the `herald-api-v1.yaml` bank (11 cases) still uses prose `Execute
curl …` actions in this run; it is being rewired to typed `http:` actions
separately (api-bank wiring follow-up). The CLI bank (10 cases) is the evidence
for this `shell:` fix.

## Sources verified

Verified 2026-05-30 against the in-tree helixqa sources:
- `~/Projects/helixqa/pkg/autonomous/structured_executor.go` — the added
  `ActionTypeShell` case.
- `~/Projects/helixqa/pkg/testbank/schema.go` — `ActionTypeShell = "shell"`.
