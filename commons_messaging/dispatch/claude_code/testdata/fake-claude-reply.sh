#!/bin/sh
# fake-claude-reply.sh — hermetic `claude` stand-in for the HRD-127
# (GAP-3 plan §1 row 5) claude_code dispatch STRESS suite.
#
# §11.4.27: the EXTERNAL `claude` binary is the dispatcher's boundary;
# faking it with a committed shell shim is the correct hermetic seam (the
# dispatcher's New(binaryPath,...) constructor IS the existing test seam —
# bootstrap_test.go's writeFakeClaudeBinary uses exactly this approach).
#
# Behaviour: ignore all argv, emit ONE well-formed <<<HERALD-REPLY>>> line
# carrying valid DispatchResponse JSON, exit 0. This is what a healthy
# `claude --resume <uuid> --print <envelope>` round-trip looks like on the
# wire: a single line the production parseReply() must accept.
#
# Used for: the N-parallel Dispatch stress run (exec round-trip latency +
# concurrency-safety / -race proof) AND as the bootstrap-success body when a
# test exercises the auto-bootstrap path (non-empty stdout satisfies the
# §107 bootstrap bluff guard in bootstrap.go).
printf '<<<HERALD-REPLY>>> {"outcome":"answered","summary":"stress ack","details":"hermetic fake-claude reply for HRD-127 stress run","affected_paths":[],"reproduction_steps":[],"estimated_effort":"S","follow_up_questions":[]}\n'
exit 0
