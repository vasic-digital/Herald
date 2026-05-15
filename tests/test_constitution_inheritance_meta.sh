#!/usr/bin/env bash
# tests/test_constitution_inheritance_meta.sh — Paired mutation
# meta-test for the inheritance gate (Constitution §1.1).
#
# Discovers the parent-provided constitution (via the same inline
# parent-walk used by the gate), then delegates to that constitution's
# meta_test_inheritance.sh which:
#   1. Snapshots <discovered-constitution>/Constitution.md
#   2. Mutates: deletes the §11.4 forensic anchor line
#   3. Runs Herald's inheritance gate (passed as argument)
#   4. Asserts the gate now FAILs (non-zero exit)
#   5. Restores the snapshot
#
# If the gate returns 0 with the anchor removed, it is a BLUFF GATE
# and this meta-test FAILs accordingly (anti-bluff per §1.1).
#
# Usage:
#   bash tests/test_constitution_inheritance_meta.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
GATE="${SCRIPT_DIR}/test_constitution_inheritance.sh"

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
if [ -z "${CONST_DIR}" ]; then
    echo "ERROR: constitution not found by walking parents of ${REPO_ROOT}" >&2
    echo "       Place a clone alongside Herald:" >&2
    echo "         git clone git@github.com:HelixDevelopment/HelixConstitution.git $(dirname "${REPO_ROOT}")/constitution" >&2
    exit 2
fi

META="${CONST_DIR}/meta_test_inheritance.sh"
if [ ! -x "${META}" ]; then
    echo "ERROR: discovered constitution missing meta_test_inheritance.sh: ${META}" >&2
    exit 2
fi
if [ ! -x "${GATE}" ]; then
    echo "ERROR: Herald inheritance gate not found or not executable: ${GATE}" >&2
    exit 2
fi

bash "${META}" "bash '${GATE}' > /dev/null 2>&1"
