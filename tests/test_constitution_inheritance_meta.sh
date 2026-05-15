#!/usr/bin/env bash
# tests/test_constitution_inheritance_meta.sh — Paired mutation
# meta-test for the inheritance gate (Constitution §1.1).
#
# Delegates to constitution/meta_test_inheritance.sh which:
#   1. Snapshots constitution/Constitution.md
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
META="${REPO_ROOT}/constitution/meta_test_inheritance.sh"
GATE="${SCRIPT_DIR}/test_constitution_inheritance.sh"

if [ ! -x "${META}" ]; then
    echo "ERROR: constitution meta-test not found or not executable: ${META}" >&2
    exit 2
fi
if [ ! -x "${GATE}" ]; then
    echo "ERROR: inheritance gate not found or not executable: ${GATE}" >&2
    exit 2
fi

# The gate must FAIL with the anchor removed (that's the paired
# mutation). The constitution's meta runner asserts that for us.
bash "${META}" "bash '${GATE}' > /dev/null 2>&1"
