package claude_code

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// forkExecLock serialises the fork/exec START of every `claude` subprocess
// this package spawns (the live-dispatch path in Dispatch and the cold-start
// path in bootstrapSession). It is held ONLY across cmd.Start() — the moment
// the process group is created (setProcessGroup sets SysProcAttr.Setpgid) —
// and released before cmd.Wait(), so subprocesses still run and are reaped
// fully concurrently.
//
// Why this is required (root-cause, 2026-06-02): setProcessGroup sets
// SysProcAttr.Setpgid=true (HRD-146 process-group kill-on-timeout). Under
// high-concurrency fork/exec on Darwin + the race detector, two goroutines
// forking simultaneously while the child must setpgid()+execve() can wedge
// the child between fork() and execve() — the parent then blocks forever in
// syscall.forkExec's readlen() on the child status pipe (the stress suite
// deterministically timed out after 10m with exactly one worker stuck there).
// Go's os/exec already takes its internal ForkLock in *shared* mode for the
// common path; this exclusive lock closes the residual Setpgid-fork window
// our process-group setup opens, mirroring the ForkLock intent. Start is
// microseconds, so serialising it does not meaningfully reduce throughput
// (the subprocess body — the slow part — stays concurrent).
var forkExecLock sync.Mutex

// runOutputSerialized starts cmd under forkExecLock (serialising only the
// fork/exec) and then waits for it OUTSIDE the lock, returning the captured
// stdout. It is the concurrency-safe replacement for cmd.Output() for the
// process-group-enabled `claude` invocations. Stderr is captured into an
// *exec.ExitError (matching cmd.Output()'s contract) so the existing
// ee.Stderr diagnostics in Dispatch / bootstrapSession keep working.
func runOutputSerialized(cmd *exec.Cmd) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	forkExecLock.Lock()
	startErr := cmd.Start()
	forkExecLock.Unlock()
	if startErr != nil {
		return nil, startErr
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		// Mirror cmd.Output(): surface stderr on the ExitError so callers
		// that inspect ee.Stderr keep their diagnostics.
		if ee, ok := waitErr.(*exec.ExitError); ok && ee.Stderr == nil {
			ee.Stderr = stderr.Bytes()
		}
		return stdout.Bytes(), waitErr
	}
	return stdout.Bytes(), nil
}

// Dispatch invokes `claude --resume <UUID> --print "<envelope>"` and parses
// the structured reply line prefixed with <<<HERALD-REPLY>>>.
//
// §107 (anti-bluff): a PASS requires (a) the claude binary exits 0, AND
// (b) stdout contains a well-formed JSON reply on a line carrying the
// <<<HERALD-REPLY>>> marker. A reply where Claude refused, errored, or
// never produced the marker is an explicit FAIL — we do not synthesise
// defaults.
//
// Spec §33.2 step 1 (session resolution): if ResolveSession returns
// uuid.Nil we MUST NOT pretend. The HRD-012 step-7 root-cause fix
// (2026-05-22) wires d.bootstrapSession(ctx, anchor) inside buildCmd
// to spawn `claude --session-id <new-uuid>` non-interactively, persist
// the anchor, and proceed with the regular `--resume <new-uuid>` path.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResponse, error) {
	// HRD-146: impose a bounded per-message deadline. The production caller
	// (pherald listen → Handle → Dispatch) passes the unbounded long-poll
	// runtime ctx, so without this a hung `claude` invocation would block
	// the inbound subscriber goroutine forever. context.WithTimeout takes
	// the MINIMUM of the caller's existing deadline and ours, so a tighter
	// caller deadline (e.g. the 180s integration-test budget capped, or an
	// upstream cancellation) is preserved — we never extend the caller.
	timeout := d.dispatchTimeoutOrDefault()
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, sessionUUID, anchor, err := d.buildCmd(dctx, req)
	if err != nil {
		return DispatchResponse{}, err
	}
	// Kill the whole process group (not just the direct child) on
	// ctx-cancel, so a `claude` that spawns helper subprocesses holding the
	// stdout pipe cannot keep cmd.Output() blocked past the deadline. No-op
	// fallback on platforms without process groups (see setProcessGroup).
	setProcessGroup(cmd)

	// Serialise ONLY the fork/exec start (process-group creation) across
	// concurrent Dispatch calls; the subprocess then runs + is waited on
	// concurrently. See runOutputSerialized / forkExecLock for the
	// root-cause (Setpgid + concurrent fork/exec deadlock).
	out, err := runOutputSerialized(cmd)
	if err != nil {
		// Distinguish a deadline/cancellation from an ordinary non-zero
		// exit. On timeout the child is SIGKILLed by exec.CommandContext, so
		// the surfaced error would otherwise be a bare "signal: killed" with
		// no hint that WE bounded it — name the timeout explicitly so the
		// operator can tell a hang from a genuine claude failure (§107).
		if de := dctx.Err(); de == context.DeadlineExceeded && ctx.Err() == nil {
			return DispatchResponse{}, fmt.Errorf(
				"claude_code: dispatch claude --resume %s: timed out after %s (HERALD_CLAUDE_DISPATCH_TIMEOUT); process killed: %w",
				sessionUUID, timeout, err)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return DispatchResponse{}, fmt.Errorf("claude_code: dispatch claude --resume %s: exit %d: %s",
				sessionUUID, ee.ExitCode(), strings.TrimSpace(string(ee.Stderr)))
		}
		return DispatchResponse{}, fmt.Errorf("claude_code: dispatch exec: %w", err)
	}

	resp, err := parseReply(out)
	if err != nil {
		return DispatchResponse{}, fmt.Errorf("claude_code: dispatch parse reply: %w", err)
	}
	// Stash session metadata for the persistence layer (HRD-012 step 7).
	resp.SessionUUID = sessionUUID
	resp.AnchorPath = anchor

	// Best-effort persistence: pool == nil means callers opted out via
	// New (Dispatch-only adapter); NewWithStorage opts in. Persistence
	// failure surfaces an error but does NOT undo the dispatch — Claude
	// has already responded, so re-issuing would double-spend.
	if d.pool != nil {
		if err := d.PersistSessionState(ctx, resp); err != nil {
			return resp, fmt.Errorf("claude_code: dispatch persist session: %w", err)
		}
	}
	return resp, nil
}

// buildCmd assembles the `claude` invocation for a DispatchRequest WITHOUT
// running it. Extracted from Dispatch so unit tests can inspect the *exec.Cmd
// argv slice (proving the Wave 6 operator-locked `--model claude-opus-4-7`
// pin is load-bearing on the literal argv — not merely "configured somewhere
// hopefully").
//
// Returns the prepared *exec.Cmd, the resolved session UUID, the anchor
// path, and any session-resolution error. The caller (Dispatch) is
// responsible for cmd.Output() and reply parsing.
func (d *Dispatcher) buildCmd(ctx context.Context, req DispatchRequest) (*exec.Cmd, uuid.UUID, string, error) {
	sessionUUID, anchor, err := d.ResolveSession()
	if err != nil {
		return nil, uuid.Nil, anchor, fmt.Errorf("claude_code: dispatch resolve session: %w", err)
	}
	if sessionUUID == uuid.Nil {
		// HRD-012 step 7 root-cause fix (2026-05-22): on missing anchor,
		// spawn a fresh claude session non-interactively, persist the
		// anchor, and resume against the freshly-minted UUID. The
		// previous placeholder error path was a §107 PASS-bluff —
		// docs/Fixed.md claimed step 7 closed but the bootstrap never
		// landed, breaking every project without a hand-rolled anchor.
		bootUUID, err := d.bootstrapSession(ctx, anchor)
		if err != nil {
			return nil, uuid.Nil, anchor, fmt.Errorf("claude_code: dispatch: bootstrap session at %s: %w", anchor, err)
		}
		sessionUUID = bootUUID
	}

	envelope := d.FormatEnvelopeWithPreText(req, string(req.Channel))
	cmd := exec.CommandContext(ctx, d.binaryPath,
		"--resume", sessionUUID.String(),
		"--model", "claude-opus-4-7", // Wave 6 operator-locked: Opus always.
		"--print", envelope,
	)
	cmd.Dir = d.workingDir
	return cmd, sessionUUID, anchor, nil
}

// parseReply scans Claude Code's stdout for the <<<HERALD-REPLY>>> marker
// and decodes the first JSON object that follows it into a
// DispatchResponse. Returns an error if no marker is found or the JSON
// is malformed — explicit rejection preserves §107 anti-bluff.
//
// The marker may appear mid-line (e.g. prefixed by some Claude-side
// formatting) — we accept the JSON object that starts at the first '{'
// at-or-after the marker. This tolerates trailing prose without
// softening the strict "no marker = FAIL" rule.
func parseReply(stdout []byte) (DispatchResponse, error) {
	const marker = "<<<HERALD-REPLY>>>"
	s := string(stdout)
	idx := strings.Index(s, marker)
	if idx < 0 {
		return DispatchResponse{}, fmt.Errorf("no <<<HERALD-REPLY>>> marker found in claude stdout (§107 bluff guard); stdout=%q", truncate(s, 512))
	}
	after := s[idx+len(marker):]
	braceIdx := strings.Index(after, "{")
	if braceIdx < 0 {
		return DispatchResponse{}, fmt.Errorf("marker present but no JSON object follows it; after-marker=%q", truncate(after, 512))
	}
	dec := json.NewDecoder(strings.NewReader(after[braceIdx:]))
	var resp DispatchResponse
	if err := dec.Decode(&resp); err != nil {
		return DispatchResponse{}, fmt.Errorf("decode reply JSON: %w (raw: %q)", err, truncate(after[braceIdx:], 512))
	}
	return resp, nil
}

// truncate keeps error messages readable when claude emits multi-kilobyte
// stdout.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
