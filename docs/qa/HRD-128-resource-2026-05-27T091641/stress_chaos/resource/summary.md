# HRD-128 — Container-orchestrated / Resource-exhaustion stress + chaos (GAP-3 unit 6)

Run-id: `HRD-128-resource-2026-05-27T091641`
Plan: `docs/superpowers/plans/2026-05-27-stress-chaos-suite.md` §1 row 6 + §7 host-safety.
Authority: Universal §11.4.85 (inherited per §11.4.35) + Herald §108.a / §107.x; §12 / §12.6 host-session-safety.
Platform: darwin, host total 18432 MiB, Go 1.26.2, container runtime: podman (4 GiB applehv VM).

## Per-scenario result

| # | Scenario | Mode | Result | Evidence |
|---|----------|------|--------|----------|
| 1 | Disk-full → tagged error propagation | hermetic Go | **PASS** | `disk_full_tagged_error.txt` |
| 2 | §12.6 host-mem headroom proof | hermetic Go | **PASS** | `host_memory_headroom.txt` |
| 3 | Disk-full real bounded fill (≤64 MiB) | opt-in Go (HERALD_STRESS_LIVE_DISK=1) | SKIP-with-reason (deterministic; opt-in proven separately) | (live) |
| 4 | Container OOM-confinement | LIVE shell harness | **SKIP-with-reason** (§12.6 gate refused) | `container_oom_confinement.skip.txt` |
| 5 | Connection-churn stress (≥30s, --memory-bounded PG) | LIVE shell harness | **SKIP-with-reason** | `connection_churn_stress.skip.txt` |

## Hermetic PASS detail (real code paths)

**Disk-full (scenario 1).** A fake `digital.vasic.database.Database` injects `syscall.ENOSPC`
on the migration-UP exec, driving the REAL `commons_storage.RunMigrations` → `applyMigration`
write path (only the external DB boundary is faked per §11.4.27). Asserted:
`up_exec_attempts>=1` (real write path exercised), error returned (no silent swallow),
error tagged `commons_storage: apply` + `exec up`, `errors.Is(err, syscall.ENOSPC)` true
(wrap chain unbroken), 0 migrations reported applied (no partial-success bluff). Captured
full error:
`commons_storage: apply v1 (init_core): exec up: write tablespace pg_default: no space left on device`

**§12.6 headroom (scenario 2).** `HostMemHeadroom()` captured pre/post a 25× in-process
disk-full workload. The workload's own `used_fraction_delta` is ~0 (≈ -0.0003 — the host
needle did not move), proving the hermetic unit adds negligible host-mem pressure. This is
the §12.6 compliance-evidence artefact: the test can never consume enough host memory to
endanger the operator's session.

## §12 / §12.6 host-safety record (THE point of this unit)

- Pre-flight host used-fraction at run time: **0.7422** (≈74%) — **already above the §12.6 60% ceiling**
  (pre-existing, unrelated host load; NOT caused by this unit — the hermetic delta is ≈0).
- Because the host is over the ceiling, the LIVE container-OOM + connection-churn scenarios are
  **REFUSED** by the harness's §12.6 pre-flight gate even when the operator opts in
  (`HERALD_STRESS_LIVE_OOM=1`). An honest SKIP-with-reason over an unsafe run (§11.4.3). The harness
  WOULD, on a host with ample headroom, boot a `--memory=<cap>` container, drive in-container
  memory pressure, and prove the OOM-kill is confined to the container cgroup scope while the host
  stays <60% — never touching host memory directly, with trap-cleanup that always removes the container.
- NO host-OOM, NO real-host-disk fill (the opt-in real-fill path is HARD-capped at 64 MiB inside
  os.MkdirTemp with always-run cleanup), NO host/system process killed, NO `systemctl`, NO host
  network change. Verified: no `herald-oom-*` container leaked across all runs.

## Reproduce

```bash
# Hermetic Go (always safe, deterministic):
cd commons_storage && go test -race -count=3 \
  -run 'TestRunMigrations_Chaos_DiskFull_TaggedError|TestResource_HostMemHeadroom_Section126' ./...

# LIVE harness (SKIP-with-reason unless host has ample headroom AND operator opts in):
HERALD_STRESS_QA_DIR="$(pwd)/docs/qa" HERALD_STRESS_RUN_ID="HRD-128-resource-2026-05-27T091641" \
  bash tests/test_resource_stress_chaos.sh
```
