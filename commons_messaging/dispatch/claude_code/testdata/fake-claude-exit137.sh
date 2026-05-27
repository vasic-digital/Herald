#!/bin/sh
# fake-claude-exit137.sh — hermetic `claude` stand-in for the HRD-127
# CHAOS (a) PROCESS-DEATH scenario: the subprocess writes a PARTIAL reply
# (a half-line, NOT a complete <<<HERALD-REPLY>>> JSON object) to stdout,
# emits a diagnostic to stderr, then exits 137 — the conventional
# SIGKILL-equivalent exit code (128 + SIGKILL(9)) an OOM-killed / kill -9'd
# process reports.
#
# Asserts the production contract: Dispatch's cmd.Output() sees a non-zero
# *exec.ExitError, returns a tagged "claude_code: dispatch claude --resume
# <uuid>: exit 137: ..." error (NOT a hang, NOT a silent partial-accept of
# the half-written stdout). §107: a partial write that exits non-zero MUST
# fail loud.
printf 'parti'                       # partial stdout — deliberately truncated mid-token
printf 'killed mid-write\n' 1>&2     # stderr diagnostic surfaced verbatim by Dispatch
exit 137
