#!/usr/bin/env bash
# tests/test_i6_refinement_meta.sh — Paired mutation test for HRD-080 (I6
# refinement). Anti-bluff confirmation that the refined I6 invariant
# (forbids `constitution/` entry in .gitmodules — but permits other
# submodule entries) actually catches what it claims to catch.
#
# Three subtests:
#
#   M1. Refined I6 PASSES with a non-constitution .gitmodules entry.
#       (Anti-bluff against the original blanket-forbid form.)
#   M2. Refined I6 FAILS if `path = constitution` is reintroduced.
#       (Anti-bluff against gate weakening.)
#   M3. Refined I6 FAILS if `path = constitution/anything` is added.
#       (Anti-bluff against path-prefix-only matching.)
#
# All three subtests use hardlink-backup to restore the working .gitmodules
# (if any) afterwards, per the operator safety mandate (2026-05-20).
#
# Returns 0 only if all three subtests behave as claimed. Non-zero on any
# bluff or unexpected behavior.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
GATE="${SCRIPT_DIR}/test_constitution_inheritance.sh"
GM="${REPO_ROOT}/.gitmodules"
BACKUP="${REPO_ROOT}/.gitmodules.hrd080-backup"

cleanup() {
    if [ -f "${BACKUP}" ]; then
        mv -f "${BACKUP}" "${GM}"
    elif [ -f "${GM}" ] && [ ! -s "${GM}" ]; then
        # We may have created an empty .gitmodules ourselves; remove it.
        rm -f "${GM}"
    fi
}
trap cleanup EXIT

# Backup any pre-existing .gitmodules (hardlink so the move is atomic-ish).
if [ -f "${GM}" ]; then
    ln -f "${GM}" "${BACKUP}"
fi

run_gate_and_capture_exit() {
    bash "${GATE}" > /dev/null 2>&1
    echo "$?"
}

pass=0
fail=0

# M1: non-constitution entry → gate must PASS.
cat > "${GM}" <<'EOF'
[submodule "submodules/database"]
    path = submodules/database
    url = git@github.com:vasic-digital/database.git
EOF
exit_code="$(run_gate_and_capture_exit)"
if [ "${exit_code}" = "0" ]; then
    echo "PASS  M1 refined I6 permits non-constitution .gitmodules entry"
    pass=$((pass + 1))
else
    echo "FAIL  M1 refined I6 falsely FAILED with non-constitution entry (exit=${exit_code})"
    fail=$((fail + 1))
fi

# M2: explicit constitution/ entry → gate must FAIL.
cat > "${GM}" <<'EOF'
[submodule "submodules/database"]
    path = submodules/database
    url = git@github.com:vasic-digital/database.git
[submodule "constitution"]
    path = constitution
    url = git@github.com:HelixDevelopment/HelixConstitution.git
EOF
exit_code="$(run_gate_and_capture_exit)"
if [ "${exit_code}" != "0" ]; then
    echo "PASS  M2 refined I6 catches constitution/ re-introduction"
    pass=$((pass + 1))
else
    echo "FAIL  M2 refined I6 FALSELY PASSED with constitution/ re-introduced (BLUFF GATE)"
    fail=$((fail + 1))
fi

# M3: constitution/sub-path entry → gate must FAIL.
cat > "${GM}" <<'EOF'
[submodule "constitution-foo"]
    path = constitution/foo
    url = git@github.com:HelixDevelopment/x.git
EOF
exit_code="$(run_gate_and_capture_exit)"
if [ "${exit_code}" != "0" ]; then
    echo "PASS  M3 refined I6 catches constitution/<sub-path> re-introduction"
    pass=$((pass + 1))
else
    echo "FAIL  M3 refined I6 FALSELY PASSED with constitution/<sub-path> (BLUFF GATE)"
    fail=$((fail + 1))
fi

# Done — cleanup happens via trap.
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -gt 0 ]; then
    exit 1
fi
exit 0
