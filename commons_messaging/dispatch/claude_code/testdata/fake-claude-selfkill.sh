#!/bin/sh
# fake-claude-selfkill.sh — hermetic `claude` stand-in for the HRD-127
# CHAOS (a) PROCESS-DEATH variant where the subprocess SIGKILLs ITSELF
# mid-write (kill -KILL $$), rather than exiting with a chosen code. This
# models a `claude` process that is OOM-killed / externally SIGKILLed: the
# OS terminates it by signal, cmd.Output() surfaces a signal-termination
# *exec.ExitError (exit code -1 / "signal: killed").
#
# IMPORTANT (§12 host-safety): the ONLY process this script ever kills is
# ITSELF ($$ — this shim's own PID, a child the Go test spawned). It NEVER
# targets a real `claude` or any host process. No `pkill`, no PID
# discovery, no kill of anyone else.
#
# Asserts: Dispatch returns a tagged error and does NOT hang.
printf 'half'                        # partial stdout before self-termination
kill -KILL $$                        # self-SIGKILL — bounded to this shim's own PID
# unreachable
