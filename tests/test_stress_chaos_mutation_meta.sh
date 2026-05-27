#!/usr/bin/env bash
# tests/test_stress_chaos_mutation_meta.sh — Paired §1.1 mutation test for the
# GAP-3 §11.4.85 stress + chaos test suite (HRD-130, plan §6 / §5;
# 2026-05-27-stress-chaos-suite). Closes the last GAP-3 task: the stress/chaos
# detectors are themselves protected by a paired mutation gate proving each one
# is genuinely LOAD-BEARING — i.e. it FAILs when the production resilience /
# recovery property it claims to enforce is removed.
#
# Per Universal §11.4 + §1.1 + Herald §107: every gate MUST have a paired
# mutation proving it FAILs when the property it claims to enforce is removed.
# A stress/chaos test that PASSes even after the recovery guard it exercises is
# deleted is itself a §11.4 PASS-bluff at the resilience layer.
#
# §107.y working-tree quiescence (lesson from commit 72e81ab — the 2026-05-21
# JWT-bypass residue incident): this gate writes .git/MUTATION_IN_PROGRESS on
# entry, removes it via trap-on-exit, and refuses to start if the lockfile is
# already present (another gate in flight) or if the working tree is dirty.
# Trap-on-exit restores every mutated file even on early-exit; the final
# quiescence check greps the mutation targets for residual `MUTATED SC-M*`
# markers. Per the 2026-05-27 concurrent-gate-residue incident memo + §107.y,
# this gate MUST run ONE-AT-A-TIME, FOREGROUND, never backgrounded/concurrent —
# it transiently mutates production source, so any concurrency leaks residue.
#
# GAP-3 covers FIVE mutations, one per load-bearing production resilience
# property exercised by a hermetic stress/chaos detector (each detector needs
# NO container runtime + NO live PG/Redis + NO real `claude` — all hermetic):
#
#   M1 (timeout-resilience). claude_code dispatch.go buildCmd uses
#      `exec.CommandContext(ctx, ...)` so a context deadline cancels the child.
#      Mutate it to `exec.Command(...)` (drop the ctx) so cancellation can no
#      longer fire. Detector: TestDispatch_Chaos_TimeoutContextCancel — the
#      30s-sleep shim is no longer killed at the 800ms deadline, so Dispatch
#      blocks and the test's 8s watchdog `t.Fatal`s → FAIL. (dispatch.go's own
#      comment at the test, "the M3 mutation that drops ctx is what this
#      catches", names this exact property.)
#
#   M2 (fail-loud reply parse). claude_code parseReply returns an explicit
#      "decode reply JSON" error on a truncated/malformed <<<HERALD-REPLY>>>
#      object. Mutate the decode-error branch to swallow it (`return resp, nil`)
#      so a truncated reply is silently partial-accepted. Detector:
#      TestDispatch_Chaos_TruncatedReply — the truncated shim's reply now parses
#      to a nil error, so `if dispErr == nil { t.Fatalf }` fires → FAIL.
#
#   M3 (panic-free degrade). inbound dispatcher.go extractReplyToMessageID
#      returns (0, error) on an unsupported message_id type — the malformed-
#      payload degrade contract the long-poll loop relies on. Mutate the
#      `default:` branch to `panic(...)`. Detector:
#      TestInbound_Chaos_ExtractReplyToMessageID_Degrades — the
#      string/bool/slice/map cases now PANIC, and the test's recover-guard
#      `t.Fatalf`s on any panic → FAIL.
#
#   M4 (input-corruption 4xx). pherald http events.go mapErrorToStatus maps an
#      `event_parser:` stage error to 400 Bad Request. Mutate that case to
#      return http.StatusInternalServerError. Detector:
#      TestEventsHTTP_Chaos_InputCorruption — the malformed-body cases
#      (truncated_json / garbage / array / missing-id) now return 5xx, so the
#      `code/100 == 5` bad5xx counter fires and the test FAILs.
#
#   M5 (disk-full propagation). commons_storage migrator.go applyMigration
#      surfaces a tagged `exec up: %w` error wrapping the write fault. Mutate it
#      to swallow the write error (`_ = err; return nil`) so a disk-full UP exec
#      reports success. Detector: TestRunMigrations_Chaos_DiskFull_TaggedError —
#      RunMigrations now returns a nil error under ENOSPC, so the
#      `if err == nil { t.Fatalf }` silent-swallow guard fires → FAIL.
#
# Returns 0 only when every mutation causes its detector to FAIL AND every
# detector returns to PASS after restore AND no MUTATED SC-M* markers leaked.
# Non-zero on any bluff.

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

pass=0
fail=0

DISPATCH_GO="${REPO_ROOT}/commons_messaging/dispatch/claude_code/dispatch.go"
INBOUND_GO="${REPO_ROOT}/pherald/internal/inbound/dispatcher.go"
EVENTS_GO="${REPO_ROOT}/pherald/internal/http/events.go"
MIGRATOR_GO="${REPO_ROOT}/commons_storage/migrator.go"

# §107.y pre-flight: working-tree quiescence guard.
if git diff --quiet && git diff --cached --quiet; then : ; else
    echo "FAIL: working tree dirty before mutation gate; abort per §107.y"
    git status --short
    exit 1
fi

if [ -f .git/MUTATION_IN_PROGRESS ]; then
    echo "FAIL: another mutation gate already in flight (.git/MUTATION_IN_PROGRESS present)"
    exit 1
fi

# Pre-flight: refuse to start if production code already carries GAP-3 mutation
# residue (governance test files are exempt by virtue of file path).
for f in "${DISPATCH_GO}" "${INBOUND_GO}" "${EVENTS_GO}" "${MIGRATOR_GO}"; do
    if grep -qE 'MUTATED SC-M' "${f}" 2>/dev/null; then
        echo "FAIL: production file ${f} carries pre-existing MUTATED SC-M marker"
        exit 1
    fi
done

touch .git/MUTATION_IN_PROGRESS

cleanup_all() {
    echo "[stress-chaos-mutation] cleanup: restoring mutated files + clearing lockfile"
    git checkout -- "${DISPATCH_GO}" "${INBOUND_GO}" "${EVENTS_GO}" "${MIGRATOR_GO}" 2>/dev/null || true
    rm -f .git/MUTATION_IN_PROGRESS
}
trap cleanup_all EXIT INT TERM

# Verify a file matches HEAD byte-for-byte after restore (§107.y).
assert_restored() {
    local path rel
    path="$1"
    rel="${path#${REPO_ROOT}/}"
    if diff -q <(git show "HEAD:${rel}") "${path}" >/dev/null 2>&1; then
        return 0
    fi
    echo "FAIL: ${rel} does NOT match HEAD byte-for-byte after restore (§107.y residue!)"
    diff <(git show "HEAD:${rel}") "${path}" | head -20 | sed 's/^/      /'
    return 1
}

# Quiescence check: scan the four mutation target files for residual
# MUTATED SC-M markers. The check scopes to known mutation target files —
# broader greps would match legitimate prose in this script itself.
check_quiescence() {
    local label="$1"
    local leaks=0
    for f in "${DISPATCH_GO}" "${INBOUND_GO}" "${EVENTS_GO}" "${MIGRATOR_GO}"; do
        if grep -qE 'MUTATED SC-M[0-9]+' "${f}" 2>/dev/null; then
            echo "ABORT  ${label}: MUTATED SC-M marker LEAKED in $(basename "${f}") — restore failed!"
            leaks=$((leaks+1))
        fi
    done
    return ${leaks}
}

# run_paired applies a perl mutation, asserts the marker landed, runs the
# detector and EXPECTS it to FAIL (mutation observable), restores, verifies the
# restore is byte-for-byte clean, and re-runs the detector EXPECTING PASS
# (production code regression caught).
#
#   $1 — friendly name (M1/M2/M3/M4/M5 + property)
#   $2 — path to file to mutate
#   $3 — perl one-liner that applies the mutation
#   $4 — detector command (go test invocation that pins the property)
#   $5 — anchor regex that MUST be findable in the file post-mutation
run_paired() {
    local name="$1" file="$2" mutate_cmd="$3" detector="$4" anchor="$5"
    echo ""
    echo "== ${name} =="
    echo "  → applying mutation"
    eval "${mutate_cmd}"
    if ! grep -qE "${anchor}" "${file}"; then
        echo "FAIL  ${name}: mutation did not apply — anchor regex ${anchor} not found in $(basename "${file}")"
        git checkout -- "${file}" 2>/dev/null || true
        fail=$((fail+1))
        return 1
    fi
    echo "  → mutation marker present; running detector (expect FAIL)"
    if eval "${detector}" > "/tmp/scmeta-${name// /-}.log" 2>&1; then
        echo "FAIL  ${name}: detector PASSed on mutated build — detector was never load-bearing"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/scmeta-${name// /-}.log" | sed 's/^/      /'
        git checkout -- "${file}" 2>/dev/null || true
        fail=$((fail+1))
        return 1
    else
        echo "  ✓ detector FAILed as expected on mutated build"
    fi
    echo "  → restoring tree"
    git checkout -- "${file}" 2>/dev/null
    if ! assert_restored "${file}"; then
        fail=$((fail+1))
        return 1
    fi
    echo "  → post-restore detector (expect PASS)"
    if ! eval "${detector}" > "/tmp/scmeta-${name// /-}-restored.log" 2>&1; then
        echo "FAIL  ${name}: detector FAILed on RESTORED tree — restore broken"
        echo "      (last 15 lines of detector output)"
        tail -15 "/tmp/scmeta-${name// /-}-restored.log" | sed 's/^/      /'
        fail=$((fail+1))
        return 1
    fi
    echo "PASS  ${name} (mutation observable + restore clean + post-restore PASS)"
    pass=$((pass+1))
}

# ----------------------------------------------------------------------
# M1: claude_code dispatch drops the context from exec.CommandContext.
# Production: `cmd := exec.CommandContext(ctx, d.binaryPath,` — the ctx wiring
# is what lets a deadline cancel the child mid-exec. Mutate to `exec.Command(`
# (no ctx). `ctx` is still used elsewhere in buildCmd (bootstrapSession(ctx,..))
# so the function still compiles with no unused-var error.
# Detector: TestDispatch_Chaos_TimeoutContextCancel sets an 800ms deadline
# against the 30s-sleep shim; with no ctx the child is never killed, Dispatch
# blocks, and the test's 8s watchdog t.Fatal fires → FAIL.
# ----------------------------------------------------------------------
run_paired \
    "M1-claude-drop-ctx" \
    "${DISPATCH_GO}" \
    "perl -i -pe 's|\tcmd := exec\.CommandContext\(ctx, d\.binaryPath,|\tcmd := exec.Command(d.binaryPath, // MUTATED SC-M1: dropped ctx, no timeout cancellation|' '${DISPATCH_GO}'" \
    "go test -run 'TestDispatch_Chaos_TimeoutContextCancel' -count=1 ./commons_messaging/dispatch/claude_code/..." \
    'MUTATED SC-M1'

# ----------------------------------------------------------------------
# M2: claude_code parseReply swallows the JSON decode error.
# Production: on a malformed/truncated object the decode branch returns an
# explicit "decode reply JSON" error. Mutate the decode-error return to
# `return resp, nil` (silent partial-accept).
# Detector: TestDispatch_Chaos_TruncatedReply drives the truncated shim
# (`<<<HERALD-REPLY>>> {"outcome":"answered","summary":"truncat` — marker + '{'
# present, object cut off → the DECODE path). Swallowing the error makes
# Dispatch succeed with an empty resp, so `if dispErr == nil { t.Fatalf }`
# fires → FAIL.
# ----------------------------------------------------------------------
run_paired \
    "M2-claude-swallow-decode-err" \
    "${DISPATCH_GO}" \
    "perl -i -0pe 's|\tif err := dec\.Decode\(&resp\); err != nil \{\n\t\treturn DispatchResponse\{\}, fmt\.Errorf\(\"decode reply JSON: %w \(raw: %q\)\", err, truncate\(after\[braceIdx:\], 512\)\)\n\t\}|\tif err := dec.Decode(\&resp); err != nil {\n\t\t_ = err\n\t\treturn resp, nil // MUTATED SC-M2: swallow decode error, silent partial-accept\n\t}|' '${DISPATCH_GO}'" \
    "go test -run 'TestDispatch_Chaos_TruncatedReply' -count=1 ./commons_messaging/dispatch/claude_code/..." \
    'MUTATED SC-M2'

# ----------------------------------------------------------------------
# M3: inbound extractReplyToMessageID panics on an unsupported type instead of
# degrading to (0, error).
# Production default branch: `return 0, fmt.Errorf("inbound: Raw.message_id
# unsupported type %T", v)`. Mutate to `panic(...)`.
# Detector: TestInbound_Chaos_ExtractReplyToMessageID_Degrades feeds
# string/bool/slice/map message_id values that hit the default branch; the
# panic trips the test's per-case recover-guard `t.Fatalf` → FAIL. The `v`
# variable stays used (it is the panic argument) so the switch still compiles.
# ----------------------------------------------------------------------
run_paired \
    "M3-inbound-panic-on-bad-type" \
    "${INBOUND_GO}" \
    "perl -i -pe 's|\t\treturn 0, fmt\.Errorf\(\"inbound: Raw\.message_id unsupported type %T\", v\)|\t\tpanic(fmt.Sprintf(\"inbound: Raw.message_id unsupported type %T\", v)) // MUTATED SC-M3: panic instead of degrade|' '${INBOUND_GO}'" \
    "go test -run 'TestInbound_Chaos_ExtractReplyToMessageID_Degrades' -count=1 ./pherald/internal/inbound/..." \
    'MUTATED SC-M3'

# ----------------------------------------------------------------------
# M4: events mapErrorToStatus maps an event_parser stage error to 5xx instead
# of 400.
# Production: `case strings.Contains(msg, "event_parser:"): return
# http.StatusBadRequest`. Mutate the returned status to
# http.StatusInternalServerError (already imported/used in this file).
# Detector: TestEventsHTTP_Chaos_InputCorruption fires malformed bodies
# (truncated_json / garbage / array / missing-id) that fail at the event_parser
# stage; with the mapping mutated they return 5xx, tripping the `code/100 == 5`
# bad5xx counter → the test FAILs.
# ----------------------------------------------------------------------
run_paired \
    "M4-events-parse-err-5xx" \
    "${EVENTS_GO}" \
    "perl -i -0pe 's|\tcase strings\.Contains\(msg, \"event_parser:\"\):\n\t\treturn http\.StatusBadRequest|\tcase strings.Contains(msg, \"event_parser:\"):\n\t\treturn http.StatusInternalServerError // MUTATED SC-M4: parse error mismapped to 5xx|' '${EVENTS_GO}'" \
    "go test -run 'TestEventsHTTP_Chaos_InputCorruption' -count=1 ./pherald/internal/http/..." \
    'MUTATED SC-M4'

# ----------------------------------------------------------------------
# M5: commons_storage applyMigration swallows the migration-UP write error.
# Production: `if _, err := tx.Exec(ctx, m.Up); err != nil { return
# fmt.Errorf("exec up: %w", err) }`. Mutate the return to swallow the error
# (`_ = err; return nil`) — a disk-full (ENOSPC) UP exec reports success.
# Detector: TestRunMigrations_Chaos_DiskFull_TaggedError injects ENOSPC on the
# UP exec; with the error swallowed RunMigrations returns a nil error, so the
# `if err == nil { t.Fatalf }` silent-swallow guard fires → FAIL.
# ----------------------------------------------------------------------
run_paired \
    "M5-storage-swallow-diskfull" \
    "${MIGRATOR_GO}" \
    "perl -i -pe 's|\t\treturn fmt\.Errorf\(\"exec up: %w\", err\)|\t\t_ = err; return nil // MUTATED SC-M5: swallow disk-full write error|' '${MIGRATOR_GO}'" \
    "go test -run 'TestRunMigrations_Chaos_DiskFull_TaggedError' -count=1 ./commons_storage/..." \
    'MUTATED SC-M5'

# ----------------------------------------------------------------------
# §107.y working-tree quiescence — assert no MUTATED SC-M* markers leaked into
# the four target files post-restore.
# ----------------------------------------------------------------------
echo ""
echo "== Quiescence: working tree free of MUTATED SC-M* markers =="
if check_quiescence "post-restore quiescence"; then
    echo "PASS  Quiescence: no MUTATED SC-M* markers leaked into tracked source"
    pass=$((pass+1))
else
    echo "FAIL  Quiescence: at least one MUTATED SC-M* marker survived restore — DO NOT COMMIT"
    fail=$((fail+1))
fi

# ----------------------------------------------------------------------
# Final tally.
# ----------------------------------------------------------------------
echo ""
echo "----"
echo "Result: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -eq 0 ]; then
    echo "PASS: stress-chaos mutation gate (5 paired)"
else
    echo "STRESS-CHAOS META-TEST FAIL"
fi
[ "${fail}" -eq 0 ]
