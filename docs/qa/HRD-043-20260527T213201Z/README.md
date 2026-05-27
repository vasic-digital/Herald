# HRD-043 — v1.0.0 Batch C cluster C1 (§43 pherald command body)

§11.4.36 install-upstreams — configure mirror remotes from upstreams/*.sh. Transcript shows REAL git remotes configured from parsed declarations, verified via git remote -v.

- Run-id: `HRD-043-20260527T213201Z` (UTC).
- Evidence: `transcript.txt` — REAL captured stdout from the built `pherald` binary.
- HERMETIC: throwaway `t.TempDir`-style repos + `file://` fake remotes; NO real Herald origin/mirror touched, NO real `docs/Issues.md` mutated.
- Composition: each command produces the Subject the HRD-023 pherald constitution binding classifies; the `[emit]` line confirms the verdict drove a REAL constitution event through an in-memory commons_constitution pipeline.
