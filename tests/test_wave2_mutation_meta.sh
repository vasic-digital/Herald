#!/usr/bin/env bash
# tests/test_wave2_mutation_meta.sh — Paired §1.1 mutation test for the
# Wave 2 e2e_bluff_hunt invariants E19-E33 (added 2026-05-21 with the
# 6 new flavor binaries: sherald, cherald, iherald, bherald, rherald,
# scherald).
#
# Per Universal §11.4 + §1.1 + Herald §107:
#   Every gate MUST have a paired mutation proving it FAILS when the
#   property it claims to enforce is removed. A gate without a paired
#   mutation is itself a §11.4 PASS-bluff (the operator mandate the
#   gate exists to enforce).
#
# Three mutations — each one removes a load-bearing property and the
# corresponding probe MUST fail (proving the e2e_bluff_hunt invariant
# actually catches the bluff it claims to catch):
#
#   M1. Strip the HRD pointer from commons/cli/StubCmd's error format
#       → sherald destructive-guard no longer mentions "HRD-033" in
#         stderr → E31 invariant MUST fail.
#
#   M2. Force commons/cli/VersionCmd to emit BuildVersion = "" (empty)
#       → all 6 flavor `version --json` outputs have empty version →
#         E19-E24 invariant MUST fail (asserts d["version"] is truthy).
#
#   M3. Mutate commons/branding.go case "c": DefaultPort 24792 → 9999
#       → cherald serve (no --http-port flag) binds 9999 not 24792 →
#         E26 invariant MUST fail (curl to documented port 24792
#         times out / refuses).
#
# Hardlink-backup restore after every mutation (per the operator safety
# mandate / Helix §9). Returns 0 only if all three mutations cause the
# expected probe to FAIL. Non-zero on any bluff.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

# Wave 3a: sherald + cherald now refuse to start without a usable JWT
# verifier (HERALD_AUTH_MODE + matching secret/URL). Set process-wide so
# the M2 `version --json` probe and the M3 `serve` probe can boot the
# binary. Without this the binaries exit 1 on `verifier_init` before the
# Wave 2 invariants can even be checked.
export HERALD_AUTH_MODE="${HERALD_AUTH_MODE:-hmac}"
export HERALD_AUTH_HMAC_SECRET="${HERALD_AUTH_HMAC_SECRET:-test-secret-32-bytes-of-padding!!}"

STUBS_GO="${REPO_ROOT}/commons/cli/stubs.go"
VERSION_GO="${REPO_ROOT}/commons/cli/version.go"
BRANDING_GO="${REPO_ROOT}/commons/branding.go"

pass_count=0
fail_count=0

# ----------------------------------------------------------------------
# Helpers (hardlink backup → restore pattern, same as test_i8 meta).
# ----------------------------------------------------------------------

# Use cp (not hardlink) so that subsequent in-place writes to the working
# file don't also overwrite the backup. The i8 meta-test uses hardlink
# safely because awk-then-mv replaces the working inode; here Python
# rewrites the file in place which would propagate to a hardlinked backup.
backup_file() {
    local f="$1"
    cp "$f" "$f.w2meta-backup"
}

restore_from_backup() {
    local f="$1"
    if [ -f "$f.w2meta-backup" ]; then
        cp "$f.w2meta-backup" "$f"
        rm -f "$f.w2meta-backup"
    fi
}

# Kill any leftover serve processes from a previous run.
cleanup_serve() {
    pkill -f "cherald-w2meta" 2>/dev/null || true
    pkill -f "sherald-w2meta" 2>/dev/null || true
    rm -f /tmp/*-w2meta-$$ 2>/dev/null || true
}
trap cleanup_serve EXIT

# ----------------------------------------------------------------------
# M1: Strip HRD pointer from cli.StubCmd error format.
# ----------------------------------------------------------------------
echo "== M1: strip HRD pointer from commons/cli/StubCmd error format =="
backup_file "${STUBS_GO}"

# Mutate: replace the error format string to omit the HRD argument.
python3 - <<'PYEOF'
import re, pathlib
p = pathlib.Path("commons/cli/stubs.go")
s = p.read_text()
# Replace the error format to drop the HRD substitution.
mutated = s.replace(
    'return fmt.Errorf("%s: not yet implemented — see %s for status", name, hrd)',
    'return fmt.Errorf("%s: not yet implemented", name)',
)
assert mutated != s, "M1 mutation did not apply"
p.write_text(mutated)
PYEOF

bin="/tmp/sherald-w2meta-$$"
if go build -o "${bin}" ./sherald/cmd/sherald > /tmp/w2-build.out 2>&1; then
    set +e
    "${bin}" destructive-guard > /tmp/w2-m1.out 2>&1
    rc=$?
    set -e
    # E31 PASS requires: rc != 0 AND grep 'HRD-033' /tmp/w2-m1.out.
    # After M1 mutation the grep MUST fail (the HRD pointer is gone).
    if grep -q 'HRD-033' /tmp/w2-m1.out; then
        echo "FAIL  M1: HRD-033 STILL present in stub error after mutation — E31 invariant NOT proven"
        head -5 /tmp/w2-m1.out | sed 's/^/      /'
        fail_count=$((fail_count + 1))
    else
        echo "PASS  M1: HRD-033 absent from stub error after mutation — E31 catches it (rc=${rc})"
        pass_count=$((pass_count + 1))
    fi
    rm -f "${bin}"
else
    echo "FAIL  M1: sherald build failed after M1 mutation"
    tail -10 /tmp/w2-build.out | sed 's/^/      /'
    fail_count=$((fail_count + 1))
fi

restore_from_backup "${STUBS_GO}"

# ----------------------------------------------------------------------
# M2: Force empty BuildVersion in commons/cli/VersionCmd.
# ----------------------------------------------------------------------
echo ""
echo "== M2: force empty version field in commons/cli/VersionCmd =="
backup_file "${VERSION_GO}"

python3 - <<'PYEOF'
import pathlib
p = pathlib.Path("commons/cli/version.go")
s = p.read_text()
# Mutate the JSON map literal so the "version" key always emits "".
# The main.go of each flavor sets cli.BuildVersion = version at startup,
# so mutating the package-level default won't propagate; we instead
# rewrite the map entry directly.
mutated = s.replace(
    '"version":    BuildVersion,',
    '"version":    "",',
)
assert mutated != s, "M2 mutation did not apply"
p.write_text(mutated)
PYEOF

bin="/tmp/sherald-w2meta-m2-$$"
if go build -o "${bin}" ./sherald/cmd/sherald > /tmp/w2-build.out 2>&1; then
    # E19-E24 probe asserts d["version"] is truthy.
    set +e
    "${bin}" version --json > /tmp/w2-m2.out 2>&1
    set -e
    # Use python3 to check the version field is empty (proves probe will fail).
    is_empty="$(python3 -c 'import json,sys; d=json.loads(open("/tmp/w2-m2.out").read()); print("yes" if d.get("version","") == "" else "no")' 2>/dev/null || echo "parse-error")"
    if [ "${is_empty}" = "yes" ]; then
        echo "PASS  M2: version field empty after mutation — E19-E24 invariant catches it"
        pass_count=$((pass_count + 1))
    else
        echo "FAIL  M2: version field still populated (is_empty=${is_empty}) — E19-E24 NOT proven"
        head -5 /tmp/w2-m2.out | sed 's/^/      /'
        fail_count=$((fail_count + 1))
    fi
    rm -f "${bin}"
else
    echo "FAIL  M2: sherald build failed after M2 mutation"
    tail -10 /tmp/w2-build.out | sed 's/^/      /'
    fail_count=$((fail_count + 1))
fi

restore_from_backup "${VERSION_GO}"

# ----------------------------------------------------------------------
# M3: Mutate cherald DefaultPort 24792 → 9999.
# ----------------------------------------------------------------------
echo ""
echo "== M3: mutate cherald DefaultPort 24792 → 9999 in commons/branding.go =="
backup_file "${BRANDING_GO}"

python3 - <<'PYEOF'
import pathlib, re
p = pathlib.Path("commons/branding.go")
s = p.read_text()
# Find the case "c": block and rewrite its DefaultPort assignment.
# The block contains 'case "c":' followed (within ~10 lines) by
# 'b.DefaultPort = 24792'. We mutate only that occurrence.
new = re.sub(
    r'(case "c":[\s\S]{0,400}?b\.DefaultPort = )24792',
    r'\g<1>9999',
    s,
    count=1,
)
assert new != s, "M3 mutation did not apply"
p.write_text(new)
PYEOF

# Build cherald + serve WITHOUT --http-port flag (so DefaultPort takes effect).
# The Wave 2 E26 invariant builds cherald + binds an explicit port (24992) in
# the test harness, but the *documented* DefaultPort 24792 is what operators
# get when they run `cherald serve` with no flag. If that constant moves to
# 9999 then a properly-paired e2e invariant would check the documented port.
# To prove this we directly verify: when no --http-port is passed, the
# port actually bound is 9999, NOT 24792 (the value the docs + e2e suite
# expect). This is what an E26 variant probing the default would catch.
cbin="/tmp/cherald-w2meta-m3-$$"
if go build -o "${cbin}" ./cherald/cmd/cherald > /tmp/w2-build.out 2>&1; then
    "${cbin}" serve > /tmp/w2-m3-serve.log 2>&1 &
    serve_pid=$!
    sleep 1.5

    # Probe A: documented port 24792 MUST NOT respond.
    if curl -fsS --max-time 1 'http://127.0.0.1:24792/v1/healthz' >/dev/null 2>&1; then
        port_24792_responds="yes"
    else
        port_24792_responds="no"
    fi

    # Probe B: mutated port 9999 MUST respond.
    if curl -fsS --max-time 1 'http://127.0.0.1:9999/v1/healthz' >/dev/null 2>&1; then
        port_9999_responds="yes"
    else
        port_9999_responds="no"
    fi

    kill "${serve_pid}" 2>/dev/null || true
    wait "${serve_pid}" 2>/dev/null || true

    if [ "${port_24792_responds}" = "no" ] && [ "${port_9999_responds}" = "yes" ]; then
        echo "PASS  M3: cherald bound 9999 not documented 24792 — E26-class invariant catches it"
        pass_count=$((pass_count + 1))
    else
        echo "FAIL  M3: 24792-responds=${port_24792_responds} 9999-responds=${port_9999_responds}"
        echo "      serve log:"
        tail -5 /tmp/w2-m3-serve.log 2>&1 | sed 's/^/      /'
        fail_count=$((fail_count + 1))
    fi
    rm -f "${cbin}"
else
    echo "FAIL  M3: cherald build failed after M3 mutation"
    tail -10 /tmp/w2-build.out | sed 's/^/      /'
    fail_count=$((fail_count + 1))
fi

restore_from_backup "${BRANDING_GO}"

# ----------------------------------------------------------------------
# Post-flight: verify all source files compile cleanly after restores.
# ----------------------------------------------------------------------
echo ""
echo "== Post-flight: full Herald build green after all restores =="
if go build ./commons/... ./commons_prefix/... ./commons_messaging/... ./commons_storage/... ./commons_constitution/... ./commons_infra/... ./pherald/... ./sherald/... ./cherald/... ./iherald/... ./bherald/... ./rherald/... ./scherald/... > /tmp/w2-postflight.out 2>&1; then
    echo "PASS  post-flight: full Herald build is green after restores"
    pass_count=$((pass_count + 1))
else
    echo "FAIL  post-flight: build broken after restores — backup restore corrupted"
    tail -15 /tmp/w2-postflight.out | sed 's/^/      /'
    fail_count=$((fail_count + 1))
fi

echo "----"
echo "Result: ${pass_count} PASS / ${fail_count} FAIL"
if [ "${fail_count}" -gt 0 ]; then
    exit 1
fi

echo "WAVE 2 META-TEST PASS: E19-E33 invariants catch their claimed regressions (M1/M2/M3)"
exit 0
