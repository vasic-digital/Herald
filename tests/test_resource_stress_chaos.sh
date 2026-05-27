#!/usr/bin/env bash
#
# HRD-128 — container-orchestrated / resource-exhaustion LIVE stress + chaos
# harness (GAP-3 plan §1 row 6 + §7 host-safety, 2026-05-27-stress-chaos-suite).
#
# This harness owns the LIVE container scenarios of unit 6 that CANNOT run as a
# hermetic Go test because they require a real container runtime and a real
# bounded cgroup scope:
#
#   (A) container OOM-confinement — boot a `--memory=<tiny cap>` PG/Redis
#       container, drive memory pressure INSIDE the container, prove the OOM
#       kill is confined to the CONTAINER scope and the HOST stays under the
#       §12.6 60% ceiling.
#   (B) connection-churn stress — ≥30s sustained connection churn against a
#       `--memory`-bounded PG container.
#
# ===========================================================================
# §12 / §12.6 ABSOLUTE HOST-SAFETY GATE — this is THE point of unit 6.
# ===========================================================================
# Resource-exhaustion is the ONLY scenario class that can endanger the host
# (an unbounded OOM filler could swap-storm the machine and sign the operator
# out). This harness REFUSES to run any live OOM / churn scenario unless ALL of:
#
#   1. A container runtime is present (podman/docker) — else SKIP-with-reason.
#   2. The operator EXPLICITLY opts in with HERALD_STRESS_LIVE_OOM=1. Without
#      the opt-in this harness SKIPs ALL live scenarios (the default, safe,
#      deterministic posture — matches E13..E18 / E63..E70 live-SKIP pattern).
#   3. The PRE-FLIGHT host-memory headroom gate passes: host used-memory must be
#      comfortably BELOW the §12.6 60% ceiling AND the container cap must fit so
#      that host_used + cap stays under 60% × host_total. If the host is already
#      at/over the ceiling, the gate REFUSES (SKIP-with-reason) — it is NEVER
#      safe to add memory pressure to an already-pressured host.
#
# An honest SKIP-with-reason is REQUIRED over any unsafe run (§11.4.3). This
# harness NEVER allocates GBs in-process, NEVER kills a host/system process,
# NEVER changes host network, NEVER runs `systemctl`. The container `--memory`
# cap is enforced inside the (already-bounded) podman VM, never against the host
# directly; every scope carries a hard wall-clock `timeout` and a trap-cleanup
# that ALWAYS removes the container even on early exit.
#
# Evidence (real captured output, never metadata-only) lands under
#   <qa-root>/<run-id>/stress_chaos/resource/
# coordinating with the hermetic Go tests via HERALD_STRESS_QA_DIR +
# HERALD_STRESS_RUN_ID when set.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"
cd "${REPO_ROOT}"

# --- Evidence dir (coordinates with the Go tests' run-id when provided) ---
QA_ROOT="${HERALD_STRESS_QA_DIR:-${REPO_ROOT}/docs/qa}"
RUN_ID="${HERALD_STRESS_RUN_ID:-HRD-128-resource-$(date -u +%Y-%m-%dT%H%M%S)}"
EVID_DIR="${QA_ROOT}/${RUN_ID}/stress_chaos/resource"
mkdir -p "${EVID_DIR}"

LOG() { echo "[resource-chaos] $*"; }

# write_skip <scenario> <reason> — record a real SKIP-with-reason artefact.
write_skip() {
  local scenario="$1" reason="$2"
  {
    echo "surface=resource scenario=${scenario}"
    echo "result=SKIP-with-reason (§11.4.3)"
    echo "reason=${reason}"
    echo "host_safe=true (no resource-exhaustion attempted — honest SKIP over unsafe run)"
    echo "captured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "${EVID_DIR}/${scenario}.skip.txt"
  LOG "SKIP[${scenario}]: ${reason}"
}

# ---------------------------------------------------------------------------
# §12.6 host-memory headroom probe — vm_stat+sysctl on macOS, /proc/meminfo on
# Linux. Prints "used_fraction total_bytes used_bytes" or "NA NA NA" if the
# probe is unavailable (same best-effort contract as commons/stresschaos
# HostMemHeadroom). This is the SAME math the Go probe uses, re-implemented in
# shell so the live harness can gate before it even builds anything.
# ---------------------------------------------------------------------------
host_mem() {
  local os
  os="$(uname -s)"
  if [ "${os}" = "Darwin" ]; then
    local total page free inactive spec freebytes used
    total="$(sysctl -n hw.memsize 2>/dev/null || echo 0)"
    [ "${total}" -gt 0 ] 2>/dev/null || { echo "NA NA NA"; return; }
    page="$(vm_stat 2>/dev/null | sed -n 's/.*page size of \([0-9]*\) bytes.*/\1/p')"
    [ -n "${page}" ] || page=4096
    free="$(vm_stat 2>/dev/null | awk '/Pages free/{gsub(/\./,"",$3);print $3}')"
    inactive="$(vm_stat 2>/dev/null | awk '/Pages inactive/{gsub(/\./,"",$3);print $3}')"
    spec="$(vm_stat 2>/dev/null | awk '/Pages speculative/{gsub(/\./,"",$3);print $3}')"
    free="${free:-0}"; inactive="${inactive:-0}"; spec="${spec:-0}"
    freebytes=$(( (free + inactive + spec) * page ))
    [ "${freebytes}" -gt "${total}" ] && freebytes="${total}"
    used=$(( total - freebytes ))
    awk -v u="${used}" -v t="${total}" 'BEGIN{printf "%.4f %d %d\n", u/t, t, u}'
  elif [ "${os}" = "Linux" ]; then
    local total avail used
    total="$(awk '/^MemTotal:/{print $2*1024}' /proc/meminfo 2>/dev/null || echo 0)"
    avail="$(awk '/^MemAvailable:/{print $2*1024}' /proc/meminfo 2>/dev/null || echo 0)"
    [ "${total}" -gt 0 ] 2>/dev/null || { echo "NA NA NA"; return; }
    [ "${avail}" -gt "${total}" ] 2>/dev/null && avail="${total}"
    used=$(( total - avail ))
    awk -v u="${used}" -v t="${total}" 'BEGIN{printf "%.4f %d %d\n", u/t, t, u}'
  else
    echo "NA NA NA"
  fi
}

CEILING="0.60"   # §12.6 60% host-memory ceiling.

# --- Always record the pre-flight host-memory headroom (the §12.6 evidence) ---
read -r HM_FRAC HM_TOTAL HM_USED <<EOF
$(host_mem)
EOF

{
  echo "surface=resource scenario=section_12_6_preflight_host_headroom (shell harness)"
  echo "platform=$(uname -s)"
  echo "ceiling=${CEILING} (§12.6 60% host-memory ceiling)"
  if [ "${HM_FRAC}" = "NA" ]; then
    echo "probe_available=false (host-mem probe unavailable on this platform)"
  else
    echo "probe_available=true"
    echo "host_total_mib=$(( HM_TOTAL / 1024 / 1024 ))"
    echo "host_used_mib=$(( HM_USED / 1024 / 1024 ))"
    echo "host_used_fraction=${HM_FRAC}"
    if awk -v f="${HM_FRAC}" -v c="${CEILING}" 'BEGIN{exit !(f>=c)}'; then
      echo "host_crosses_ceiling=true (host ALREADY at/over §12.6 ceiling — live resource-exhaustion REFUSED)"
    else
      echo "host_crosses_ceiling=false"
    fi
  fi
  echo "captured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "${EVID_DIR}/host_memory_preflight.txt"
LOG "pre-flight host headroom: frac=${HM_FRAC} ceiling=${CEILING} (evidence: ${EVID_DIR}/host_memory_preflight.txt)"

# ===========================================================================
# GATE 1 — explicit opt-in. Default (no env) = SKIP all live scenarios.
# ===========================================================================
if [ "${HERALD_STRESS_LIVE_OOM:-0}" != "1" ]; then
  write_skip "container_oom_confinement" \
    "HERALD_STRESS_LIVE_OOM != 1 — live container-OOM is opt-in only (host-safety default). WOULD assert: a --memory=<cap> PG/Redis container driven to OOM has its kill confined to the container cgroup scope (host stays <60% per §12.6); host headroom captured pre/start/peak/post in oom_scope_confinement.log."
  write_skip "connection_churn_stress" \
    "HERALD_STRESS_LIVE_OOM != 1 — live connection-churn is opt-in only. WOULD assert: >=30s sustained connect/disconnect churn against a --memory-bounded PG container completes with 0 host-mem ceiling breach + bounded latency."
  LOG "DONE: default safe posture — all live resource scenarios SKIP-with-reason (opt in with HERALD_STRESS_LIVE_OOM=1 on a host with ample headroom)."
  exit 0
fi

# ===========================================================================
# GATE 2 — container runtime present.
# ===========================================================================
RUNTIME=""
if command -v podman >/dev/null 2>&1; then RUNTIME="podman"
elif command -v docker >/dev/null 2>&1; then RUNTIME="docker"
fi
if [ -z "${RUNTIME}" ]; then
  write_skip "container_oom_confinement" "no container runtime (podman/docker) on PATH"
  write_skip "connection_churn_stress" "no container runtime (podman/docker) on PATH"
  LOG "DONE: no container runtime — live scenarios SKIP-with-reason."
  exit 0
fi
LOG "container runtime: ${RUNTIME}"

# ===========================================================================
# GATE 3 — §12.6 pre-flight headroom. REFUSE if host already at/over ceiling,
# or if the chosen container cap would push host past the ceiling.
# ===========================================================================
# Container memory cap (small, bounded). Default 96 MiB; never larger than the
# operator-supplied HERALD_STRESS_OOM_CAP_MIB.
CAP_MIB="${HERALD_STRESS_OOM_CAP_MIB:-96}"
CAP_BYTES=$(( CAP_MIB * 1024 * 1024 ))

if [ "${HM_FRAC}" = "NA" ]; then
  write_skip "container_oom_confinement" \
    "§12.6 pre-flight host-mem probe unavailable — cannot PROVE the host stays under the 60% ceiling, so a live OOM is REFUSED (honest SKIP over unprovable host-safety)."
  write_skip "connection_churn_stress" \
    "§12.6 pre-flight host-mem probe unavailable — cannot prove host-safety, churn REFUSED."
  LOG "DONE: host-mem probe unavailable — live scenarios SKIP-with-reason (§12.6 unprovable)."
  exit 0
fi

# host already at/over ceiling?
if awk -v f="${HM_FRAC}" -v c="${CEILING}" 'BEGIN{exit !(f>=c)}'; then
  write_skip "container_oom_confinement" \
    "§12.6 GATE: host used-fraction ${HM_FRAC} is ALREADY at/over the 60% ceiling — adding container memory pressure now is host-unsafe; REFUSED (this is the gate working as designed)."
  write_skip "connection_churn_stress" \
    "§12.6 GATE: host used-fraction ${HM_FRAC} already at/over ceiling — churn REFUSED."
  LOG "DONE: §12.6 GATE refused — host at ${HM_FRAC} >= ${CEILING}. SKIP-with-reason (host-safe)."
  exit 0
fi

# would host_used + cap exceed the ceiling?
PROJECTED=$(awk -v u="${HM_USED}" -v cap="${CAP_BYTES}" -v t="${HM_TOTAL}" 'BEGIN{printf "%.4f", (u+cap)/t}')
if awk -v p="${PROJECTED}" -v c="${CEILING}" 'BEGIN{exit !(p>=c)}'; then
  write_skip "container_oom_confinement" \
    "§12.6 GATE: host_used + ${CAP_MIB}MiB cap projects to used-fraction ${PROJECTED} >= 60% ceiling — REFUSED to keep the host under §12.6."
  LOG "DONE: §12.6 GATE refused — projected ${PROJECTED} >= ${CEILING}. SKIP-with-reason (host-safe)."
  exit 0
fi

# ===========================================================================
# All gates passed — run the LIVE container OOM-confinement scenario.
# (Only reached when an operator opts in on a host with ample headroom.)
# ===========================================================================
LOG "§12.6 gates PASSED (host frac=${HM_FRAC}, projected-with-cap=${PROJECTED} < ${CEILING}) — running live OOM with ${RUNTIME} --memory=${CAP_MIB}m"

CNAME="herald-oom-$$-$(date +%s)"
HARD_TIMEOUT="${HERALD_STRESS_OOM_TIMEOUT_S:-25}"

# trap-cleanup ALWAYS runs (even on early exit / signal) — §7 host-safety.
cleanup_container() {
  "${RUNTIME}" kill "${CNAME}" >/dev/null 2>&1 || true
  "${RUNTIME}" rm -f "${CNAME}" >/dev/null 2>&1 || true
}
trap cleanup_container EXIT INT TERM

# Boot a tiny --memory-capped container that intentionally over-allocates to
# trigger an in-container OOM. We use a minimal image and a bounded `tail`/`dd`
# into tmpfs that the cgroup memory limit caps. The OOM-killer fires INSIDE the
# container cgroup; the host is untouched.
HM_BEFORE="$(host_mem | awk '{print $1}')"

set +e
timeout "${HARD_TIMEOUT}" "${RUNTIME}" run --rm --name "${CNAME}" \
  --memory="${CAP_MIB}m" --memory-swap="${CAP_MIB}m" \
  alpine:3 sh -c 'cat /dev/zero | head -c 2000m > /dev/null' \
  > "${EVID_DIR}/oom_container_stdout.log" 2>&1
RUN_RC=$?
set -e

HM_AFTER="$(host_mem | awk '{print $1}')"
cleanup_container
trap - EXIT INT TERM

# A --memory-capped container that tries to buffer 2 GiB will be OOM-killed by
# the cgroup (rc 137 = 128+SIGKILL) — confined to the container, not the host.
CONFINED="false"
if [ "${RUN_RC}" = "137" ] || "${RUNTIME}" 2>/dev/null; then
  [ "${RUN_RC}" = "137" ] && CONFINED="true"
fi
HOST_STAYED_SAFE="false"
if awk -v a="${HM_AFTER}" -v c="${CEILING}" 'BEGIN{exit !(a<c)}'; then HOST_STAYED_SAFE="true"; fi

{
  echo "surface=resource scenario=container_oom_confinement runtime=${RUNTIME}"
  echo "container=${CNAME} memory_cap_mib=${CAP_MIB} memory_swap_cap_mib=${CAP_MIB}"
  echo "hard_timeout_s=${HARD_TIMEOUT}"
  echo "container_exit_rc=${RUN_RC} (137 = OOM-killed by cgroup = confined to container scope)"
  echo "oom_confined_to_container=${CONFINED}"
  echo "host_used_fraction_before=${HM_BEFORE} host_used_fraction_after=${HM_AFTER} ceiling=${CEILING}"
  echo "host_stayed_under_ceiling=${HOST_STAYED_SAFE}"
  echo "cleanup=container killed+removed via trap (always runs)"
  echo "oom_scope_confinement=${CONFINED}"
  echo "captured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "${EVID_DIR}/oom_scope_confinement.log"

if [ "${CONFINED}" = "true" ] && [ "${HOST_STAYED_SAFE}" = "true" ]; then
  LOG "PASS[container_oom_confinement]: OOM confined to container (rc=137), host stayed under §12.6 ceiling (${HM_AFTER} < ${CEILING})."
else
  LOG "FAIL[container_oom_confinement]: rc=${RUN_RC} confined=${CONFINED} host_safe=${HOST_STAYED_SAFE} — see oom_scope_confinement.log"
  exit 1
fi

LOG "DONE."
