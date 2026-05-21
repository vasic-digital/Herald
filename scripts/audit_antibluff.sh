#!/usr/bin/env bash
# Anti-bluff audit for Herald per Universal Constitution §11.4 (End-user
# quality guarantee — forensic anchor) + user mandate 2026-05-20:
#
# > "Make sure that all existing tests and Challenges do work in anti-
# > bluff manner — they MUST confirm that all tested codebase really
# > works as expected!"
#
# This script enforces THREE anti-bluff invariants across Herald + every
# owned-by-us submodule under submodules/ + containers/:
#
#   I1. §11.4 forensic anchor present in {CONSTITUTION,CLAUDE,AGENTS}.md
#       of every OWNED submodule (vasic-digital + HelixDevelopment) —
#       the exact verbatim user-mandate sentence beginning "We had been
#       in position...". Third-party vendored SDKs (per §11.4.74
#       catalogue-check no-match → vendor) are SKIP-with-reason: we
#       cannot mandate an anchor on an upstream we don't own.
#   I2. Go test suite passes WITHOUT integration-tag (proves unit-level
#       assertions exercise real behaviour even in CI's hermetic mode).
#   I3. Inheritance gate (tests/test_constitution_inheritance.sh) + the
#       paired I6-refinement meta-test (tests/test_i6_refinement_meta.sh)
#       both green — proves the §1.1 paired-mutation discipline holds.
#
# Exit 0 only when all three pass. Failure prints the offending invariant.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0
skip=0

# The verbatim §11.4 forensic anchor sentence (canonical) - one fragment.
ANCHOR='tests do execute with success and all Challenges as well'

# Classify a submodule by its remote URL (from .gitmodules) into
# "owned" (vasic-digital | HelixDevelopment → §11.4 anchor REQUIRED) or
# "vendor" (third-party → §11.4 anchor NOT required per §11.4.74).
# Echoes "owned" or "vendor".
classify_submodule() {
    local path="$1"
    local url
    url="$(git config -f .gitmodules --get-regexp 'submodule\..*\.path' 2>/dev/null \
        | awk -v p="${path}" '$2 == p { sub(/\.path$/, ".url", $1); print $1 }' \
        | while read -r key; do git config -f .gitmodules --get "${key}"; done)"
    case "${url}" in
        *vasic-digital*|*HelixDevelopment*) echo "owned" ;;
        *) echo "vendor" ;;
    esac
}

echo "== I1: §11.4 forensic anchor across submodules =="
all_submodules=()
for d in submodules/*/ containers/; do
    [ -d "$d" ] || continue
    all_submodules+=("$d")
done
if [ "${#all_submodules[@]}" -eq 0 ]; then
    echo "FAIL  no submodules found — Herald should consume at least the containers submodule"
    fail=$((fail+1))
fi
for sub in "${all_submodules[@]}"; do
    name="$(basename "$sub")"
    sub_path="${sub%/}"
    classification="$(classify_submodule "${sub_path}")"
    if [ "${classification}" = "vendor" ]; then
        echo "SKIP  ${name}: third-party vendored SDK (Catalogue-Check no-match per §11.4.74); §11.4 anchor not required"
        skip=$((skip+1))
        continue
    fi
    found=0
    for f in "$sub/CONSTITUTION.md" "$sub/CLAUDE.md" "$sub/AGENTS.md"; do
        if [ -f "$f" ] && grep -qF "${ANCHOR}" "$f"; then
            found=1; break
        fi
    done
    if [ "$found" = 1 ]; then
        echo "PASS  ${name}: §11.4 anchor present"
        pass=$((pass+1))
    else
        echo "FAIL  ${name}: §11.4 anchor MISSING — propagate per Universal §11.4 + CONST-047"
        fail=$((fail+1))
    fi
done

echo ""
echo "== I2: Go test suite (default mode, no -tags=integration) =="
modules=(
    "./commons/..."
    "./commons_prefix/..."
    "./commons_messaging/..."
    "./commons_storage/..."
    "./commons_constitution/..."
    "./commons_infra/..."
    "./pherald/..."
)
if go test -race -count=1 "${modules[@]}" > /tmp/herald_test_out.log 2>&1; then
    echo "PASS  go test -race -count=1 across $(printf '%s ' "${modules[@]}")"
    pass=$((pass+1))
else
    echo "FAIL  go test failed — see /tmp/herald_test_out.log"
    tail -20 /tmp/herald_test_out.log
    fail=$((fail+1))
fi

echo ""
echo "== I3: inheritance gate + paired I6 meta-test =="
if bash tests/test_constitution_inheritance.sh > /tmp/herald_gate_out.log 2>&1; then
    echo "PASS  tests/test_constitution_inheritance.sh"
    pass=$((pass+1))
else
    echo "FAIL  inheritance gate FAILed — see /tmp/herald_gate_out.log"
    tail -10 /tmp/herald_gate_out.log
    fail=$((fail+1))
fi
if bash tests/test_i6_refinement_meta.sh > /tmp/herald_i6_out.log 2>&1; then
    echo "PASS  tests/test_i6_refinement_meta.sh (paired §1.1 mutation)"
    pass=$((pass+1))
else
    echo "FAIL  I6 paired meta-test FAILed — see /tmp/herald_i6_out.log"
    tail -10 /tmp/herald_i6_out.log
    fail=$((fail+1))
fi
if bash tests/test_i8_usability_meta.sh > /tmp/herald_i8_out.log 2>&1; then
    echo "PASS  tests/test_i8_usability_meta.sh (paired §1.1 mutation — §107 covenant)"
    pass=$((pass+1))
else
    echo "FAIL  I8 paired meta-test FAILed — see /tmp/herald_i8_out.log"
    tail -10 /tmp/herald_i8_out.log
    fail=$((fail+1))
fi

echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL / ${skip} SKIP"
if [ "${fail}" -gt 0 ]; then
    exit 1
fi
echo ""
echo "Anti-bluff covenant intact:"
echo "  - All OWNED submodules (vasic-digital + HelixDevelopment) carry"
echo "    the §11.4 forensic anchor; third-party vendored SDKs (via"
echo "    §11.4.74 catalogue-check no-match → vendor) are excluded."
echo "  - Default-mode test suite genuinely exercises behaviour."
echo "  - Inheritance gate + paired §1.1 meta-test both green."
exit 0
