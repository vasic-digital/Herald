<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# QA evidence — HelixQA CLI-flavors bank (run 20260530T142628Z)

**Feature under test:** Herald's eight flavor binaries (`pherald`, `sherald`,
`cherald`, `bherald`, `rherald`, `iherald`, `scherald`, `qaherald`) driven
end-to-end by the HelixQA autonomous-QA framework via
`challenges/helixqa-banks/herald-cli-flavors.yaml`, launched by
`scripts/helixqa_run.sh` on `--platform desktop`.

**Result:** 10/10 challenges PASSED. See `qa-report.md`.

## Why this is real anti-bluff evidence (§107 / §11.4)

Each bank case uses a `shell:` action, so HelixQA's desktop challenge path
(`pkg/orchestrator/definition_challenge.go::executeDesktopShellSteps`)
**actually executes the compiled binary** via `sh -c`, captures the real exit
code + combined output, and scores PASS only when the step exits 0. The shell
command itself encodes the assertion — it pipes the binary's stdout through
`grep`, so a wrong `binary` field, a wrong `flavor` code, a branding-string
regression, or an unknown-subcommand that wrongly succeeds all force a
**non-zero exit → FAIL**. This is recorded verbatim in `qa-report.md` under
"### Recorded evidence" (the captured command + `exit=0` + the
`shell-exit-zero` assertion ledger).

This is NOT a metadata-only / crash-absence PASS:

- **Mutation-proven:** corrupting one case's assertion (asserting
  `binary:"NOTcherald"`) flips that challenge PASSED→FAILED (9/10, 90%) — the
  assertions genuinely bite.
- **Real wall-clock:** the per-challenge durations are real `os/exec` time
  (e.g. `pherald` ~780ms first-run), not the microsecond no-op of a skipped
  case.
- **Desktop crash-absence is explicitly rejected:** the HelixQA aggregator
  refuses to promote a desktop SKIP to PASSED on crash-absence
  (`promoteSkippedToPassed` desktop guard) — desktop PASSED is earned only by
  this real `shell:` execution.

## Ground truth cross-check

The asserted DisplayName / flavor / binary values were derived from BOTH the
real binary output AND `commons/branding.go` (`DefaultBranding`) — they agree,
and the agreement is the validation (it guards the historical §3.5 branding
regressions: "Compliance→Constitution", "RHR→Release",
"Schedule→Scheduled-audit").

## Files

| File | What it is |
|---|---|
| `qa-report.md` | The HelixQA report incl. the per-challenge "### Recorded evidence" (captured shell command + exit code + assertion ledger). |
| `helixqa_run.log` | The launcher's `helixqa run` transcript (10/10 passed, 0 crashes). |
| `api_serve_skip.txt` | §11.4.3 SKIP-with-reason: the `[api]` plane was skipped because Postgres :24100 was unreachable in this environment (the CLI bank is the desktop evidence; the api plane is driven by the launcher's live `pherald serve` boot when PG is up — see `docs/guides/HELIXQA_INTEGRATION.md`). |

## Reproduce

```bash
# Requires `claude` on PATH (the HelixQA LLM/Vision bridge) + ~/Projects/helixqa.
scripts/helixqa_run.sh
# evidence lands under qa-results/helixqa/<run-id>/ (gitignored); this dir is a
# committed copy of one such run per the §107.x docs/qa evidence mandate.
```
