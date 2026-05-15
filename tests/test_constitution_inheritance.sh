#!/usr/bin/env bash
# tests/test_constitution_inheritance.sh — Inheritance gate.
#
# Asserts that the Helix Constitution submodule is correctly checked
# out and that Herald's root docs reference it. Read-only.
#
# Constitution: §11.4 (end-user quality guarantee), §1.1 (paired
# with tests/test_constitution_inheritance_meta.sh).
#
# Returns 0 on PASS, non-zero on FAIL. Each assertion prints a line
# prefixed with PASS / FAIL so the output is greppable from CI.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONST_DIR="${REPO_ROOT}/constitution"

fail_count=0
pass_count=0

check() {
    local label="$1"
    local cmd="$2"
    if eval "${cmd}" > /dev/null 2>&1; then
        echo "PASS  ${label}"
        pass_count=$((pass_count + 1))
    else
        echo "FAIL  ${label}"
        fail_count=$((fail_count + 1))
    fi
}

# Invariant 1: submodule directory exists
check "I1 constitution/ directory exists" \
    "[ -d '${CONST_DIR}' ]"

# Invariant 2: Constitution.md exists AND contains the §11.4 anchor
# (the EXACT line meta_test_inheritance.sh mutates — see §1.1)
ANCHOR_CONST='### §11.4 End-user quality guarantee — forensic anchor (User mandate, 2026-04-28)'
check "I2 Constitution.md contains §11.4 forensic anchor" \
    "grep -qF \"${ANCHOR_CONST}\" '${CONST_DIR}/Constitution.md'"

# Invariant 3: constitution/CLAUDE.md exists AND contains the anti-bluff anchor
ANCHOR_CLAUDE='MANDATORY ANTI-BLUFF COVENANT'
check "I3 constitution/CLAUDE.md contains anti-bluff covenant anchor" \
    "grep -qF '${ANCHOR_CLAUDE}' '${CONST_DIR}/CLAUDE.md'"

# Invariant 4: constitution/AGENTS.md exists AND contains the anti-bluff anchor
ANCHOR_AGENTS='Anti-bluff covenant'
check "I4 constitution/AGENTS.md contains anti-bluff covenant anchor" \
    "grep -qF '${ANCHOR_AGENTS}' '${CONST_DIR}/AGENTS.md'"

# Invariant 5: Herald root docs reference the submodule.
# Each root doc MUST contain a phrase that anchors the inheritance
# pointer (case-sensitive, exact).
check "I5a CLAUDE.md declares inheritance from constitution/CLAUDE.md" \
    "grep -qF 'INHERITED FROM constitution/CLAUDE.md' '${REPO_ROOT}/CLAUDE.md'"
check "I5b AGENTS.md references constitution/AGENTS.md" \
    "grep -qF 'constitution/AGENTS.md' '${REPO_ROOT}/AGENTS.md'"
check "I5c HERALD_CONSTITUTION.md extends Helix Universal Constitution" \
    "grep -qF 'extends' '${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md' && \
     grep -qF 'constitution/Constitution.md' '${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md'"

# Summary
echo "----"
echo "Result: ${pass_count} PASS / ${fail_count} FAIL"
if [ "${fail_count}" -gt 0 ]; then
    exit 1
fi
exit 0
