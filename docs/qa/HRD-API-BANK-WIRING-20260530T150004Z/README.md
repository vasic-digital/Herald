# api-bank rewrite self-validation

Harness self-validation of `challenges/helixqa-banks/herald-api-v1.yaml` after the
shell-curl rewrite (2026-05-30). Postgres :24100 was UNREACHABLE in this
environment, so a true `pherald serve` live run was SKIPPED (§11.4.3). To prove
the rewritten bank's `shell:` actions GENUINELY EXECUTE and every assertion
BITES, the bank was run via the REAL `helixqa run --platform desktop`
orchestrator (the platform whose path runs shell: steps via os/exec) against a
local self-signed-HTTPS stub (`stub.py`) emulating pherald /v1/* responses.

- GOOD stub (correct pherald responses): **11/11 PASS, rc=0**.
- BAD stub (wrong status/body for every route): **10/11 FAIL, rc=1**.
  HRD-QA-API-006 expects 202+JSON; the bad stub returns 202 for all POSTs, so it
  coincidentally satisfies the positive case. Every NEGATIVE / divergent-expectation
  case correctly FAILS — proving the gates have teeth (a wrong status or missing
  body/header string forces a non-zero shell exit → case FAIL).

Files:
- `out_good/qa-report.json` — 11/11 passed against the good stub.
- `out_bad/qa-report.json`  — 10/11 failed against the bad stub (bite proof).
- `run_good.log` / `run_bad.log` — helixqa run console output.
- `stub.py` — the self-signed-HTTPS pherald /v1/* emulator used for the proof.

This is harness self-validation against a stub, NOT a claim the live pherald API
passed. The live run is gated on operator-supplied Postgres + `pherald serve` —
see `docs/guides/HELIXQA_INTEGRATION.md` (api-bank section).
