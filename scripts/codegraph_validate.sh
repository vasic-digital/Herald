#!/usr/bin/env bash
# Anti-bluff validation for CodeGraph integration per Universal §11.4.78 +
# §11.4 end-user-quality covenant.
#
# A CodeGraph install that "looks installed" but returns empty results for
# real Herald symbols is exactly the bluff §11.4 forbids. This script
# QUERIES the graph for symbols that DEFINITELY exist (we wrote them in
# the M1/M2/M3 commits) and asserts non-empty hits.
#
# Symbols probed (each must yield ≥ 1 hit):
#   - Evaluator                (commons_constitution/evaluator.go)
#   - ConstitutionStore        (commons_constitution/state.go)
#   - Record                   (state/postgres.go method)
#   - WithTenantContext        (commons_storage/postgres.go)
#   - QuickstartBoot           (commons_infra/boot.go)
#   - Server                   (pherald/internal/http/server.go)
#
# Plus negative-control: a definitely-absent symbol MUST return ≤ 1 hit
# (we expect 0 but allow noise from accidental substring matches).
#
# Exit 0 on PASS, non-zero on FAIL with a per-symbol breakdown.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

if [ ! -d "${REPO_ROOT}/.codegraph" ]; then
    echo "ERROR: CodeGraph not initialised. Run scripts/codegraph_setup.sh first." >&2
    exit 1
fi

pass=0
fail=0

# query <symbol> <min-hits>
query() {
    local symbol="$1"
    local min="$2"
    # `query` outputs human-readable; we count lines that look like result entries.
    # A result entry has "  <path>:<line>" indentation. Count those.
    local hits
    # Strip ANSI escape sequences then count lines matching "path/.../file.go:NN".
    hits=$(npx -y @colbymchenry/codegraph query "${symbol}" 2>/dev/null \
        | sed $'s/\x1b\\[[0-9;]*[a-zA-Z]//g' \
        | grep -cE '[a-z_]+/.*\.go:[0-9]+' || true)
    if [ "${hits}" -ge "${min}" ]; then
        echo "PASS  ${symbol}: ${hits} hits (min ${min})"
        pass=$((pass + 1))
    else
        echo "FAIL  ${symbol}: ${hits} hits (min ${min}) — codegraph likely not indexed or broken"
        fail=$((fail + 1))
    fi
}

# Positive controls — these MUST exist in the indexed graph.
query "Evaluator" 1
query "ConstitutionStore" 1
query "Record" 1
query "WithTenantContext" 1
query "QuickstartBoot" 1
query "Server" 1

# §11.4.79 own-org submodule inclusion probes — DEFERRED until codegraph
# submodule-traversal support lands (HRD-091 open). Current codegraph
# (v0.x npm @colbymchenry/codegraph) silently skips directories with a
# nested `.git` file/dir — which is exactly how git submodules appear on
# disk. The constitutional rule §11.4.79 still applies; the tooling gap
# is tracked separately. SKIP-with-reason per §11.4.3 — never PASS-by-
# default just to silence the bluff guard.
echo "SKIP  TaskQueue probe (HRD-091 — codegraph submodule-traversal gap; §11.4.3 hardware_not_present)"
echo "SKIP  BackgroundTask probe (HRD-091 — codegraph submodule-traversal gap; §11.4.3 hardware_not_present)"

# Negative control: a symbol we never defined. ≤ 0 expected; allow noise.
neg_hits=$(npx -y @colbymchenry/codegraph query "XyzzyNeverDefinedSymbolHRDQQ" 2>/dev/null \
    | sed $'s/\x1b\\[[0-9;]*[a-zA-Z]//g' \
    | grep -cE '[a-z_]+/.*\.go:[0-9]+' || true)
if [ "${neg_hits}" -le 1 ]; then
    echo "PASS  Negative control: ${neg_hits} hits (≤ 1 for definitely-absent symbol)"
    pass=$((pass + 1))
else
    echo "FAIL  Negative control: ${neg_hits} hits for definitely-absent symbol — graph returning false positives"
    fail=$((fail + 1))
fi

echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -gt 0 ]; then
    exit 1
fi
exit 0
