#!/usr/bin/env bash
# tests/test_constitution_inheritance.sh — Inheritance gate.
#
# Herald does NOT carry its own copy of the Helix Constitution. Herald
# is consumed as a submodule of a parent project that provides the
# constitution at <ancestor>/constitution/. This gate discovers that
# directory via parent-walk (mirroring the canonical
# find_constitution.sh) and asserts the inheritance contract holds.
#
# Constitution: §11.4 (end-user quality guarantee); paired with
# tests/test_constitution_inheritance_meta.sh per §1.1.
#
# Returns 0 on PASS, non-zero on FAIL.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- Bootstrap discovery -------------------------------------------------
# Inline mirror of constitution/find_constitution.sh phase 1. Kept tiny
# (≤15 lines) so it's a negligible maintenance burden. Once located, we
# delegate to the canonical script for any subsequent operations.
find_constitution_root() {
    local cur="${1:-$PWD}"
    while [[ "${cur}" != "/" ]]; do
        if [[ -f "${cur}/constitution/Constitution.md" ]]; then
            echo "${cur}/constitution"; return 0
        fi
        cur="$(dirname "${cur}")"
    done
    return 1
}

CONST_DIR="$(find_constitution_root "${REPO_ROOT}" || true)"

fail_count=0
pass_count=0

check() {
    local label="$1"; local cmd="$2"
    if eval "${cmd}" > /dev/null 2>&1; then
        echo "PASS  ${label}"
        pass_count=$((pass_count + 1))
    else
        echo "FAIL  ${label}"
        fail_count=$((fail_count + 1))
    fi
}

# Invariant 1: constitution discovered via parent walk
if [ -n "${CONST_DIR}" ]; then
    echo "PASS  I1 constitution discovered at ${CONST_DIR}"
    pass_count=$((pass_count + 1))
else
    echo "FAIL  I1 constitution NOT found by walking parents of ${REPO_ROOT}"
    echo "       Herald assumes the parent project provides constitution/. For"
    echo "       standalone work, place a clone next to Herald:"
    echo "         git clone git@github.com:HelixDevelopment/HelixConstitution.git \\"
    echo "             $(dirname "${REPO_ROOT}")/constitution"
    fail_count=$((fail_count + 1))
    echo "----"
    echo "Result: ${pass_count} PASS / ${fail_count} FAIL"
    exit 1
fi

# Invariant 2: Constitution.md contains the §11.4 forensic anchor
# (the EXACT line meta_test_inheritance.sh mutates — see §1.1)
ANCHOR_CONST='### §11.4 End-user quality guarantee — forensic anchor (User mandate, 2026-04-28)'
check "I2 Constitution.md contains §11.4 forensic anchor" \
    "grep -qF \"${ANCHOR_CONST}\" '${CONST_DIR}/Constitution.md'"

# Invariant 3: CLAUDE.md contains the anti-bluff covenant anchor
check "I3 constitution/CLAUDE.md contains anti-bluff covenant anchor" \
    "grep -qF 'MANDATORY ANTI-BLUFF COVENANT' '${CONST_DIR}/CLAUDE.md'"

# Invariant 4: AGENTS.md contains the anti-bluff covenant anchor
check "I4 constitution/AGENTS.md contains anti-bluff covenant anchor" \
    "grep -qF 'Anti-bluff covenant' '${CONST_DIR}/AGENTS.md'"

# Invariant 5: Herald root docs declare parent-discovery inheritance
check "I5a CLAUDE.md declares Helix Constitution inheritance (parent-discovery)" \
    "grep -qF 'INHERITED FROM Helix Constitution' '${REPO_ROOT}/CLAUDE.md' && \
     grep -qF 'find_constitution.sh' '${REPO_ROOT}/CLAUDE.md'"
check "I5b AGENTS.md references the parent-provided constitution + find_constitution.sh" \
    "grep -qF 'find_constitution.sh' '${REPO_ROOT}/AGENTS.md'"
check "I5c HERALD_CONSTITUTION.md extends the Helix Universal Constitution" \
    "grep -qF 'extends' '${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md' && \
     grep -qF 'Helix Universal Constitution' '${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md'"
check "I5d README.md documents the constitution-inheritance contract" \
    "grep -qF 'Helix Constitution' '${REPO_ROOT}/README.md'"

# Invariant 6: NO embedded copy of the constitution inside Herald
# (Herald MUST use the parent-provided constitution; a local submodule
# violates the deployment model.)
#
# Refinement (HRD-080, V3 §44.9 / 2026-05-20):
# I6 originally forbade ANY .gitmodules file at the repo root. The
# refined form forbids only a `path = constitution` (or `constitution/...`)
# entry inside .gitmodules, so non-constitution submodules (the
# Helix-stack `digital.vasic.*` modules Herald consumes at Foundation
# M2/M3 per the CLAUDE.md vendored-SDK policy) are permitted.
#
# Two-clause check:
#   (a) no <repo>/constitution/ directory
#   (b) no `path = constitution` or `path = constitution/*` entry
#       inside .gitmodules (regex-anchored to the path= key so a
#       coincidental match elsewhere in the file does not fire).
check "I6 No constitution submodule embedded in Herald" \
    "! [ -d '${REPO_ROOT}/constitution' ] && \
     { ! [ -f '${REPO_ROOT}/.gitmodules' ] || \
       ! grep -qE '^[[:space:]]*path[[:space:]]*=[[:space:]]*constitution(/.*)?$' '${REPO_ROOT}/.gitmodules'; }"

# Invariant 7: spec-change rule propagated to CLAUDE.md, AGENTS.md, and
# HERALD_CONSTITUTION.md (§106). The literal anchor is the phrase
# 'comprehensive planning and implementation' from the spec's
# §"Specification documents" — see docs/guides/HERALD_CONSTITUTION.md §106.
SPEC_CHANGE_ANCHOR='comprehensive planning and implementation'
check "I7a CLAUDE.md contains spec-change rule anchor" \
    "grep -qF '${SPEC_CHANGE_ANCHOR}' '${REPO_ROOT}/CLAUDE.md'"
check "I7b AGENTS.md contains spec-change rule anchor" \
    "grep -qF '${SPEC_CHANGE_ANCHOR}' '${REPO_ROOT}/AGENTS.md'"
check "I7c HERALD_CONSTITUTION.md §106 contains spec-change rule anchor" \
    "grep -qF '${SPEC_CHANGE_ANCHOR}' '${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md' && \
     grep -qF '§106' '${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md'"

echo "----"
echo "Result: ${pass_count} PASS / ${fail_count} FAIL"
if [ "${fail_count}" -gt 0 ]; then
    exit 1
fi
exit 0
