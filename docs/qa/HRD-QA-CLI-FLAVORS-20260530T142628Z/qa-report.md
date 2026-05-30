# HelixQA Test Report

**Generated:** 2026-05-30T19:26:44+05:00

## Overview

| Metric | Value |
|--------|-------|
| Total Challenges | 10 |
| Passed | 10 |
| Failed | 0 |
| Pass Rate | 100% |
| Total Crashes | 0 |
| Total ANRs | 0 |
| Total Duration | 6.62577975s |
| Platforms Tested | 1 |

## Platform: DESKTOP

- **Duration:** 6.62577975s
- **Crashes:** 0
- **ANRs:** 0
- **Challenges:** 10

| Challenge | Status | Duration |
|-----------|--------|----------|
| pherald version emits canonical JSON build info | PASSED | 1.21995675s |
| sherald version emits canonical JSON build info | PASSED | 519.447541ms |
| cherald version emits canonical JSON build info | PASSED | 541.555125ms |
| bherald version emits canonical JSON build info | PASSED | 531.927875ms |
| rherald version emits canonical JSON build info | PASSED | 531.855333ms |
| iherald version emits canonical JSON build info | PASSED | 515.430958ms |
| pherald help lists its subcommands | PASSED | 326.982375ms |
| scherald version emits canonical JSON build info | PASSED | 523.389916ms |
| qaherald version emits canonical JSON build info | PASSED | 544.657709ms |
| Unknown flavor subcommand fails loudly with non-zero exit | PASSED | 352.218875ms |

### Recorded evidence

Real captured runtime output + assertion ledger per challenge (§107.x anti-bluff: the report itself is the auditable proof the command ran and emitted the asserted output).

#### pherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/pherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"pherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"p"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]' && printf '%s' "$out" | grep -Eq '"version"[[:space:]]*:[[:space:]]*"[0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/pherald version | head -1 | grep -Eq '^Project Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### sherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/sherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"sherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"s"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/sherald version | head -1 | grep -Eq '^System Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### cherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/cherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"cherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"c"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/cherald version | head -1 | grep -Eq '^Constitution Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### bherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/bherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"bherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"b"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/bherald version | head -1 | grep -Eq '^Build Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### rherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/rherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"rherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"r"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/rherald version | head -1 | grep -Eq '^Release Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### iherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/iherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"iherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"i"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/iherald version | head -1 | grep -Eq '^Incident Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### pherald help lists its subcommands — PASSED

```
shell-step[0]: h=$(${HERALD_BIN_DIR}/pherald --help) && printf '%s' "$h" | grep -Eq '(^|[[:space:]])version([[:space:]]|$)' && printf '%s' "$h" | grep -Eq '(^|[[:space:]])serve([[:space:]]|$)' && printf '%s' "$h" | grep -Eq '(^|[[:space:]])listen([[:space:]]|$)' && printf '%s' "$h" | grep -Eq '(^|[[:space:]])watch([[:space:]]|$)'
shell-step[0]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |

#### scherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/scherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"scherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"sc"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/scherald version | head -1 | grep -Eq '^Scheduled-audit Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### qaherald version emits canonical JSON build info — PASSED

```
shell-step[0]: out=$(${HERALD_BIN_DIR}/qaherald version --json) && printf '%s' "$out" | grep -Eq '"binary"[[:space:]]*:[[:space:]]*"qaherald"' && printf '%s' "$out" | grep -Eq '"flavor"[[:space:]]*:[[:space:]]*"qa"' && printf '%s' "$out" | grep -Eq '"go_version"[[:space:]]*:[[:space:]]*"go[0-9]' && printf '%s' "$out" | grep -Eq '"os"[[:space:]]*:[[:space:]]*"[a-z]' && printf '%s' "$out" | grep -Eq '"arch"[[:space:]]*:[[:space:]]*"[a-z0-9]'
shell-step[0]: exit=0 output=""
shell-step[1]: ${HERALD_BIN_DIR}/qaherald version | head -1 | grep -Eq '^QA Herald '
shell-step[1]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |
| shell-exit-zero | step[1].exit_code | 0 | 0 | PASS |

#### Unknown flavor subcommand fails loudly with non-zero exit — PASSED

```
shell-step[0]: if ${HERALD_BIN_DIR}/pherald no-such-command >/dev/null 2>&1; then echo 'BUG: unknown subcommand exited 0'; exit 1; else exit 0; fi
shell-step[0]: exit=0 output=""
```

| Assertion | Target | Expected | Actual | Result |
|-----------|--------|----------|--------|--------|
| shell-exit-zero | step[0].exit_code | 0 | 0 | PASS |

### Step Validation

| Step | Status | Duration | Error |
|------|--------|----------|-------|
| HRD-QA-CLI-001 | PASSED | 4.625µs | - |
| HRD-QA-CLI-002 | PASSED | 5.417µs | - |
| HRD-QA-CLI-003 | PASSED | 4.834µs | - |
| HRD-QA-CLI-004 | PASSED | 10.834µs | - |
| HRD-QA-CLI-005 | PASSED | 3.959µs | - |
| HRD-QA-CLI-006 | PASSED | 20.083µs | - |
| HRD-QA-CLI-009 | PASSED | 1.792µs | - |
| HRD-QA-CLI-007 | PASSED | 5.25µs | - |
| HRD-QA-CLI-008 | PASSED | 4.542µs | - |
| HRD-QA-CLI-010 | PASSED | 11.583µs | - |

---

*Generated by HelixQA*
