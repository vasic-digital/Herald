#!/usr/bin/env bash
# tests/test_i8_usability_meta.sh — Paired §1.1 mutation test for the I8
# End-user-usability covenant invariant (HERALD_CONSTITUTION.md §107 /
# Helix Constitution §11.4 / verbatim operator mandate 2026-04-28
# reasserted 2026-05-19 + 2026-05-20).
#
# Anti-bluff confirmation that I8a/I8b/I8c (assert the verbatim
# operator-mandate quote is present in CLAUDE.md / AGENTS.md /
# HERALD_CONSTITUTION.md) actually catch what they claim to catch.
#
# Per Universal §11.4 + §1.1 + Herald §107:
#   Every gate MUST have a paired mutation proving it FAILS when the
#   property it claims to enforce is removed. A gate without a paired
#   mutation is itself a §11.4 PASS-bluff (the operator mandate the
#   gate exists to enforce).
#
# Three subtests (one per propagation file):
#
#   M1. Strip the verbatim covenant from CLAUDE.md → gate MUST FAIL on I8a.
#   M2. Strip the verbatim covenant from AGENTS.md → gate MUST FAIL on I8b.
#   M3. Strip the verbatim covenant from HERALD_CONSTITUTION.md → gate
#       MUST FAIL on I8c.
#
# Hardlink-backup restore after every mutation (per the operator safety
# mandate / Helix §9). Returns 0 only if all three mutations cause the
# gate to FAIL on the expected invariant. Non-zero on any bluff.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
GATE="${SCRIPT_DIR}/test_constitution_inheritance.sh"

CLAUDE_MD="${REPO_ROOT}/CLAUDE.md"
AGENTS_MD="${REPO_ROOT}/AGENTS.md"
HERALD_CONST="${REPO_ROOT}/docs/guides/HERALD_CONSTITUTION.md"

# The exact anchor literal asserted by I8a/b/c (must match
# USABILITY_ANCHOR in test_constitution_inheritance.sh).
ANCHOR='all tests do execute with success and all Challenges as well, but in reality the most of the features does not work'

pass_count=0
fail_count=0

# ----------------------------------------------------------------------
# Helpers
# ----------------------------------------------------------------------

# hardlink_backup <file>  → creates <file>.i8meta-backup as a hardlink
hardlink_backup() {
    local f="$1"
    ln -f "$f" "$f.i8meta-backup"
}

# restore_from_backup <file>
restore_from_backup() {
    local f="$1"
    if [ -f "$f.i8meta-backup" ]; then
        # Use cat | tee to refresh the inode contents (since hardlink shares it).
        # We took the backup BEFORE the mutation, so the .i8meta-backup inode
        # still has the original content via the hardlink semantics ONLY if
        # the mutation was a file-rewrite (sed -i with a temp file). To be
        # safe we always restore by copying the backup's content over the
        # working file.
        cat "$f.i8meta-backup" > "$f"
        rm -f "$f.i8meta-backup"
    fi
}

# strip_anchor_from <file>  → in-place removal of any line containing
# the ANCHOR fragment. Returns 0 on success, 1 if no line was removed
# (which would mean the gate is already a bluff — anchor not present).
strip_anchor_from() {
    local f="$1"
    if ! grep -qF "$ANCHOR" "$f"; then
        echo "  ERROR: anchor not present in $f BEFORE mutation — gate is a bluff"
        return 1
    fi
    # Use awk to write to a tmp file then move (avoids in-place sed quoting issues).
    local tmp
    tmp="$(mktemp)"
    grep -vF "$ANCHOR" "$f" > "$tmp"
    mv "$tmp" "$f"
    if grep -qF "$ANCHOR" "$f"; then
        echo "  ERROR: anchor STILL present in $f AFTER mutation — strip failed"
        return 1
    fi
    return 0
}

# Run the gate, capture exit code AND output. Returns the gate's exit code.
run_gate_silent() {
    bash "$GATE" > /dev/null 2>&1
    return $?
}

# Run the gate and report whether a specific invariant label appears as FAIL.
gate_failed_on() {
    local invariant="$1"  # e.g. "I8a"
    local out
    out="$(bash "$GATE" 2>&1)"
    echo "$out" | grep -qE "^FAIL +${invariant}"
}

# subtest <name> <file> <expected_failing_invariant>
subtest() {
    local name="$1"; local file="$2"; local inv="$3"
    echo "== $name: mutate $file, expect I8 FAIL on $inv =="

    hardlink_backup "$file"

    if ! strip_anchor_from "$file"; then
        restore_from_backup "$file"
        echo "FAIL  $name: pre-mutation anchor missing (gate is a bluff)"
        fail_count=$((fail_count + 1))
        return
    fi

    # Run gate — it MUST fail (rc != 0) AND specifically on the expected invariant.
    local gate_rc=0
    run_gate_silent
    gate_rc=$?

    local invariant_failed="no"
    if gate_failed_on "$inv"; then
        invariant_failed="yes"
    fi

    restore_from_backup "$file"

    if [ "$gate_rc" -ne 0 ] && [ "$invariant_failed" = "yes" ]; then
        echo "PASS  $name: gate correctly FAILed on $inv when anchor stripped"
        pass_count=$((pass_count + 1))
    else
        echo "FAIL  $name: gate did NOT fail correctly (rc=$gate_rc, $inv-failed=$invariant_failed)"
        fail_count=$((fail_count + 1))
    fi
}

# ----------------------------------------------------------------------
# Pre-flight: gate must currently be green (anchors present in all 3 files).
# ----------------------------------------------------------------------
echo "== Pre-flight: gate green with all 3 anchors present =="
if run_gate_silent; then
    echo "PASS  pre-flight: gate is green BEFORE mutations"
    pass_count=$((pass_count + 1))
else
    echo "FAIL  pre-flight: gate is RED before mutations — cannot run meta-test"
    bash "$GATE" 2>&1 | tail -20
    exit 1
fi

# ----------------------------------------------------------------------
# Three mutations — one per propagation file.
# ----------------------------------------------------------------------
subtest "M1" "$CLAUDE_MD" "I8a"
subtest "M2" "$AGENTS_MD" "I8b"
subtest "M3" "$HERALD_CONST" "I8c"

# ----------------------------------------------------------------------
# Post-flight: gate must be green again after restores.
# ----------------------------------------------------------------------
echo "== Post-flight: gate green after restores =="
if run_gate_silent; then
    echo "PASS  post-flight: gate is green AFTER restores"
    pass_count=$((pass_count + 1))
else
    echo "FAIL  post-flight: gate is RED after restores — backup restore broken"
    bash "$GATE" 2>&1 | tail -20
    fail_count=$((fail_count + 1))
fi

echo "----"
echo "Result: ${pass_count} PASS / ${fail_count} FAIL"
if [ "${fail_count}" -gt 0 ]; then
    exit 1
fi

echo "✓ I8 META-TEST PASS: I8a/b/c correctly catch removal of the §107 covenant anchor"
exit 0
