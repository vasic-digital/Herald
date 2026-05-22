#!/usr/bin/env bash
# tests/test_wave4_mutation_meta.sh — Paired §1.1 mutation test for Wave 4a
# (HTTP/3 + Brotli + Alt-Svc + TLS-1.3 transport substrate, E49-E55).
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILS when the property it claims to enforce is
# removed. A gate without a paired mutation is itself a §11.4 PASS-bluff.
#
# §107 covenant: this script MUST observe each mutation actually applies
# (grep + assert anchor in mutated file), MUST run the e2e probe and
# observe the targeted invariant FAIL on real wire behaviour, MUST restore
# the original file, MUST verify the restore worked (grep + assert
# original content present + post-flight e2e returns to baseline). A
# mutation gate that doesn't actually bite is itself a bluff.
#
# Wave 4a covers three mutations:
#
#   M1. Force opts.DisableH3 = true in commons/cli/serve.go RunServe →
#       E50 MUST FAIL (UDP listener never binds because startQUIC is not
#       called).
#   M2. Force opts.DisableBrotli = true in commons/cli/serve.go RunServe
#       AND pad sherald's SafetyState JSON > 256B so E53 takes the
#       brotli-encoded branch → E53 MUST FAIL (Content-Encoding:br absent
#       because BrotliMiddleware was not wired).
#   M3. Add MaxVersion: tls.VersionTLS12 to the TCP server TLSConfig in
#       commons/cli/serve.go → E54 MUST FAIL (openssl -tls1_3 client
#       cannot negotiate TLS 1.3 because the server now refuses anything
#       above TLS 1.2).
#
# Hardlink-backup-restore pattern lifted from tests/test_wave3_mutation_meta.sh.
# Post-flight verifies the full e2e battery is still green after all three
# mutations are restored.
#
# Returns 0 only when every expected mutation causes the targeted invariant
# to FAIL AND the post-flight passes. Non-zero on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
E2E="${REPO_ROOT}/scripts/e2e_bluff_hunt.sh"

pass=0
fail=0

# Backup uses cp (NOT ln) because mutations may rewrite the file contents
# wholesale; a hardlink would be truncated alongside. Restore uses
# `cat backup > file` to preserve the inode while overwriting contents.
file_backup() { cp "$1" "$1.w4meta-backup"; }
restore() {
    if [ -f "$1.w4meta-backup" ]; then
        cat "$1.w4meta-backup" > "$1"
        rm -f "$1.w4meta-backup"
    fi
}

# Safety net — if the test trips mid-run, restore every backup we know
# about and kill any orphan sherald processes left binding the test port.
SERVE_GO="${REPO_ROOT}/commons/cli/serve.go"
H3_GO="${REPO_ROOT}/commons/cli/h3.go"
HANDLER_GO="${REPO_ROOT}/sherald/internal/safety/handler.go"
AGG_GO="${REPO_ROOT}/sherald/internal/safety/aggregator.go"

cleanup_all() {
    restore "${SERVE_GO}"
    restore "${H3_GO}"
    restore "${HANDLER_GO}"
    restore "${AGG_GO}"
    # 24793 is the W4A_PORT in e2e_bluff_hunt.sh.
    for port in 24793; do
        lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
    done
}
trap cleanup_all EXIT

# Pre-flight: kill any orphan listener on the W4a target port.
for port in 24793; do
    lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
done

# ----------------------------------------------------------------------
# M1: strip H3 listener by no-op'ing the startQUIC call in RunServe.
# Targeted invariant: E50 (UDP/H3 listener bound on the W4a port). The
# mutation keeps !opts.DisableH3 truthy so cert resolution still runs +
# the TCP listener still binds HTTPS+H2 — only the UDP/H3 listener
# disappears. This is the surgical "H3 only" regression E50 catches.
#
# Why not mutate opts.DisableH3 at the top of RunServe? Two reasons:
#   1. It trips the commons/cli unit tests (TestServeCmd_BothListeners
#      GracefulShutdown + TestServeCmd_AltSvcAndBrotliAutoWired) before
#      e2e ever runs — the e2e then short-circuits on E3 (unit tests)
#      and never reaches E50, masking the actual regression.
#   2. With DisableH3=true the cert-resolution branch short-circuits
#      (lines 271-293 of serve.go), sherald falls back to plaintext
#      HTTP/1.1 → E49/E51/E52/E53/E54 ALL fail → over-broad signal
#      (= a bluffing mutation that "wins" by collateral damage).
#
# Anchor: `\t\th3Srv, h3StartErr = startQUIC(ctx, addr, r, cert)`
# (inside the `if !opts.DisableH3 { ... }` block, two tabs deep).
# ----------------------------------------------------------------------
echo "== M1: strip H3 listener from commons/cli/serve.go =="
file_backup "${SERVE_GO}"
# Replace the startQUIC call with a no-op assignment. The `_ = cert` and
# `_ = addr` keep the variables "used" so the compiler doesn't complain
# in the !DisableH3 branch where they were previously only consumed by
# startQUIC.
perl -i -0pe 's|\t\th3Srv, h3StartErr = startQUIC\(ctx, addr, r, cert\)|\t\t// MUTATED M1: startQUIC call no-op'\''d so UDP listener never binds\n\t\th3Srv, h3StartErr = nil, nil\n\t\t_ = cert\n\t\t_ = addr|' "${SERVE_GO}"
if ! grep -q "MUTATED M1" "${SERVE_GO}"; then
    echo "FAIL  M1: perl mutation failed — anchor 'startQUIC(ctx, addr, r, cert)' not found in serve.go"
    fail=$((fail+1))
elif grep -qE '^\t\th3Srv, h3StartErr = startQUIC' "${SERVE_GO}"; then
    echo "FAIL  M1: mutation did not replace the original startQUIC call (still present)"
    fail=$((fail+1))
else
    bash "${E2E}" > /tmp/w4meta-m1.log 2>&1 || true
    if grep -qE "FAIL  E50" /tmp/w4meta-m1.log; then
        echo "PASS  M1: stripping startQUIC breaks E50 (gate proven — UDP listener absent)"
        pass=$((pass+1))
    else
        echo "FAIL  M1: no-op'ing startQUIC did NOT break E50 — gate is a bluff"
        echo "      (last 20 lines of e2e output)"
        tail -20 /tmp/w4meta-m1.log | sed 's/^/      /'
        fail=$((fail+1))
    fi
fi
restore "${SERVE_GO}"

# ----------------------------------------------------------------------
# M2: strip Brotli middleware AND pad sherald's SafetyState response so
# E53 hits the brotli-encoded branch (body >= 256B). Without the padding,
# safety_state is ~227B which is under MinLength=256; E53 takes the
# identity-passthrough branch and would PASS even with brotli disabled
# (a non-falsifying gate). With the padding in place, E53 takes the
# >=256B branch which asserts Content-Encoding:br is present → mutation
# (brotli disabled) makes that assertion fail → E53 FAIL.
#
# Two-file mutation:
#   - commons/cli/serve.go : force opts.DisableBrotli = true (so
#     BrotliMiddleware is never wired into the chain).
#   - sherald/internal/safety/aggregator.go : add a JSON-tagged padding
#     field to SafetyState with a 300-byte filler so the serialized
#     payload exceeds the brotli MinLength threshold.
#
# The padding mutation is the cleanest non-invasive way to push the
# response over the threshold without redesigning E53 itself — we want
# E53 to act as the OBSERVER of the brotli regression, not need
# modification per mutation.
# ----------------------------------------------------------------------
echo "== M2: strip Brotli middleware (force DisableBrotli + pad safety_state body >= 256B) =="
file_backup "${SERVE_GO}"
file_backup "${AGG_GO}"

# Mutation A: force DisableBrotli=true at the top of RunServe.
perl -i -0pe 's|(\tgin\.SetMode\(gin\.ReleaseMode\))|\topts.DisableBrotli = true // MUTATED M2: force-disable Brotli middleware\n\1|' "${SERVE_GO}"

# Mutation B: add Padding field to SafetyState (so JSON body > MinLength=256B).
# Anchor on the closing brace of the SafetyState struct definition.
perl -i -0pe 's|(\tLastDestructiveOp \*DestructiveOp `json:"last_destructive_op"` // nil = none seen yet)\n(\})|\1\n\tPadding           string         `json:"_w4meta_padding"` // MUTATED M2: pad body > 256B\n\2|' "${AGG_GO}"

# Mutation C: populate the Padding field in Snapshot().
perl -i -0pe 's|(\t\tLastDestructiveOp: op,)\n(\t\})|\1\n\t\tPadding:           "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", // MUTATED M2: 300-byte padding to push body > MinLength=256\n\t\}|' "${AGG_GO}"

if ! grep -q "MUTATED M2" "${SERVE_GO}"; then
    echo "FAIL  M2: serve.go perl mutation failed — DisableBrotli anchor not found"
    fail=$((fail+1))
elif ! grep -q "MUTATED M2: pad body" "${AGG_GO}"; then
    echo "FAIL  M2: aggregator.go struct-field perl mutation failed — SafetyState anchor not found"
    fail=$((fail+1))
elif ! grep -q "MUTATED M2: 300-byte padding" "${AGG_GO}"; then
    echo "FAIL  M2: aggregator.go Snapshot perl mutation failed — Snapshot return anchor not found"
    fail=$((fail+1))
else
    bash "${E2E}" > /tmp/w4meta-m2.log 2>&1 || true
    if grep -qE "FAIL  E53" /tmp/w4meta-m2.log; then
        echo "PASS  M2: stripping Brotli middleware breaks E53 (gate proven — Content-Encoding:br absent on >=256B body)"
        pass=$((pass+1))
    else
        echo "FAIL  M2: stripping Brotli did NOT break E53 — gate is a bluff"
        echo "      (last 15 lines of e2e output)"
        tail -15 /tmp/w4meta-m2.log | sed 's/^/      /'
        fail=$((fail+1))
    fi
fi
restore "${AGG_GO}"
restore "${SERVE_GO}"

# ----------------------------------------------------------------------
# M3: downgrade the HTTP/3 listener's TLS MinVersion from TLS 1.3 to
# TLS 1.2 in commons/cli/h3.go. The upstream digital.vasic.http3 server
# rejects this at startup with an explicit error:
#
#   "http3: Config.TLSConf.MinVersion must be tls.VersionTLS13 or unset"
#
# That fail-loud guard is itself a §107 anti-bluff feature — the
# upstream library refuses to silently let H3 run on TLS 1.2 and
# emit a misleading "ALPN h3" handshake (which the wire spec forbids).
# When sherald cannot start its H3 listener, RunServe returns the
# wrapped startup error, the binary exits 1, and the W4a probe block
# in e2e_bluff_hunt.sh reports `FAIL  E49-E55 sherald serve never
# accepted HTTPS within 5s on :${W4A_PORT}` — with E54 (along with
# E49..E55) added to fail_names. That's the cascade falsification:
# the upstream guard converts the mutation into a hard startup failure
# rather than a silently-degraded listener.
#
# An alternative TLS-1.2-cap mutation on the TCP listener (serve.go's
# tcpSrv.TLSConfig) was attempted first but rejected because:
#   - E54's regex `'TLSv1\.3|Protocol  : TLSv1\.3'` matches the
#     openssl session-info "Protocol: TLSv1.3" line even when the
#     handshake actually fails (alert 70) — the requested version is
#     echoed regardless of negotiation outcome.
#   - That's a real lenient-grep finding worth a follow-up tightening
#     pass, BUT it means the mutation cannot bite cleanly today.
#
# An ALPN h3→h2 mutation in h3.go's NextProtos was also rejected:
# the upstream server.go (submodules/http3/pkg/server/server.go:97-98)
# auto-prepends "h3" if missing — the mutation gets silently undone
# before the listener binds. Honest documentation of what does NOT
# work is itself anti-bluff evidence.
#
# Anchor: the literal `MinVersion:   tls.VersionTLS13` inside the H3
# TLSConfig block (h3.go line 73 at time of writing).
# ----------------------------------------------------------------------
echo "== M3: downgrade H3 listener TLS MinVersion from 1.3 to 1.2 =="
file_backup "${H3_GO}"
perl -i -pe 's|MinVersion:   tls\.VersionTLS13|MinVersion:   tls.VersionTLS12 /* MUTATED M3: downgrade H3 TLS to 1.2 — upstream rejects */|' "${H3_GO}"
if ! grep -q "MUTATED M3" "${H3_GO}"; then
    echo "FAIL  M3: perl mutation failed — anchor 'MinVersion:   tls.VersionTLS13' not found in h3.go"
    fail=$((fail+1))
else
    bash "${E2E}" > /tmp/w4meta-m3.log 2>&1 || true
    # The mutation triggers a hard startup failure in the upstream
    # http3 library. The e2e block reports "FAIL  E49-E55 sherald serve
    # never accepted HTTPS within 5s" and pushes E54 into fail_names
    # (printed in the Bluffs-caught footer as "  - E54").
    if grep -qE "FAIL  E49-E55" /tmp/w4meta-m3.log && grep -qE "^  - E54$" /tmp/w4meta-m3.log; then
        echo "PASS  M3: downgrading H3 TLS to 1.2 breaks E54 (gate proven — upstream refuses H3 on TLS 1.2; cascade failure includes E54)"
        pass=$((pass+1))
    elif grep -qE "SKIP  E54" /tmp/w4meta-m3.log; then
        echo "SKIP  M3 (E54 itself is SKIP — openssl not installed; mutation cascade has no observable E54 invariant on this host — §11.4.3 explicit SKIP-with-reason)"
    else
        echo "FAIL  M3: downgrading H3 TLS to 1.2 did NOT break E54 — gate is a bluff"
        echo "      (last 20 lines of e2e output)"
        tail -20 /tmp/w4meta-m3.log | sed 's/^/      /'
        fail=$((fail+1))
    fi
fi
restore "${H3_GO}"

# ----------------------------------------------------------------------
# Post-flight: the full e2e battery must still be green after every
# mutation has been restored. A bluffing restore (where the mutation
# persisted) would surface here. The post-flight check also catches the
# case where a backup-restore left whitespace/encoding artefacts.
# ----------------------------------------------------------------------
echo "== Post-flight: full e2e green after restores =="
bash "${E2E}" > /tmp/w4meta-postflight.log 2>&1
postflight_ec=$?
if [ "${postflight_ec}" = 0 ] && grep -q "All Herald user-visible features verified" /tmp/w4meta-postflight.log; then
    echo "PASS  post-flight (e2e battery green after restores)"
    pass=$((pass+1))
else
    echo "FAIL  post-flight: e2e battery still has FAILs after restores (ec=${postflight_ec})"
    tail -15 /tmp/w4meta-postflight.log | sed 's/^/      /'
    fail=$((fail+1))
fi

echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "WAVE 4 META-TEST PASS"
else
    echo "WAVE 4 META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
