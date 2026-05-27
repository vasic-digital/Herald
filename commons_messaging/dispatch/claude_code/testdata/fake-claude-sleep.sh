#!/bin/sh
# fake-claude-sleep.sh — hermetic `claude` stand-in for the HRD-127
# CHAOS (b) TIMEOUT scenario: the subprocess sleeps well past any bounded
# dispatch/bootstrap deadline, NEVER emitting a reply. The Go test wraps
# the call in a short context (context.WithTimeout) so the production
# exec.CommandContext cancellation fires and kills this child.
#
# §12 host-safety: the sleep is bounded (30s hard cap) so even if the Go
# test's context cancellation somehow failed to reap this child, the shim
# self-terminates quickly — it can never linger as a host process. In the
# expected path exec.CommandContext SIGKILLs it the instant ctx expires
# (well under 1s), long before this sleep elapses.
#
# We `exec sleep` (replacing this shell process with `sleep` IN PLACE)
# rather than `sleep 30` as a child. This faithfully models the REAL
# `claude` binary: a single long-lived process that, when SIGKILLed by
# exec.CommandContext on ctx-cancel, dies and closes its OWN stdout — so
# cmd.Output() unblocks immediately. A child `sleep` (separate PID) would
# inherit the stdout pipe write-end and keep cmd.Output() blocked reading
# stdout until the orphan sleep exits (the well-known Go pipe-inheritance
# gotcha) — which is NOT how a single-process `claude` behaves. Using
# `exec` keeps the test's assertion about the production cancellation
# contract honest instead of measuring a shim artefact. See HRD-127 FINDING.
#
# Asserts: the context deadline fires; Dispatch returns within a bounded
# time (NOT after 30s); the returned error reflects the cancellation
# ("signal: killed" from the ctx-driven SIGKILL).
exec sleep 30
