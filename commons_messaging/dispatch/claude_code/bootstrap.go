package claude_code

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
)

// bootstrapPrompt is the first-message body sent to claude during a
// session-creation bootstrap. It is intentionally tight — every byte
// becomes a turn in the persisted .jsonl history that subsequent
// --resume invocations replay. We instruct the model to acknowledge
// with the same <<<HERALD-REPLY>>> JSON envelope the live dispatch
// path relies on, so the bootstrap exercise the same parser the
// runtime uses (§107: no separate "bootstrap-mode parser" — same code
// path, same wire format).
const bootstrapPrompt = `You are the Claude Code session for the Herald project. This is the bootstrap message that establishes this session. Future inbound messages from Herald subscribers (Telegram, Slack, …) will arrive via this same session via 'claude --resume <UUID> --print "<envelope>"'.

Acknowledge by responding with the structured reply marker exactly as below (single line, no surrounding prose):

<<<HERALD-REPLY>>> {"outcome":"answered","summary":"Herald inbound runtime session ready.","details":"Bootstrap acknowledged. Awaiting first subscriber message.","affected_paths":[],"reproduction_steps":[],"estimated_effort":"S","follow_up_questions":[]}`

// bootstrapSession spawns a fresh Claude Code session via
// `claude --session-id <new-uuid> --model claude-opus-4-7 --print
// <bootstrap-prompt>` and persists the anchor file. Returns the new
// session UUID on success.
//
// §107 (HRD-012 step 7, root-cause fix 2026-05-22): this replaces the
// prior placeholder "no anchored session" error path in buildCmd. The
// placeholder was a §107 PASS-bluff — docs/Fixed.md claimed step 7
// closed (persistence wired) but the auto-spawn never landed, so every
// project without a hand-rolled anchor file failed at runtime.
//
// Invocation shape verified against `claude --version` 2.1.148:
//
//   - `--session-id <uuid>` creates the on-disk session keyed by the
//     given UUID; claude writes the transcript to
//     ~/.claude/projects/<encoded-cwd>/<uuid>.jsonl during the run.
//   - `--print` forces non-interactive mode (exits after one reply).
//   - `--model claude-opus-4-7` Wave-6 operator-locked pin (matches
//     dispatch.go buildCmd literal).
//
// Idempotency: callers MUST resolve the anchor first; if a non-Nil
// UUID is already on disk, bootstrap is skipped. Concurrent bootstrap
// invocations on the same anchor are NOT serialised here (see §107.y
// working-tree quiescence) — last-writer-wins per the spec; the
// first-created orphan session becomes inert.
//
// Failure mode: on any non-zero exit from `claude`, the stderr is
// included verbatim in the returned error so the operator can
// distinguish auth failures from network failures from quota
// rejections without spelunking through logs.
func (d *Dispatcher) bootstrapSession(ctx context.Context, anchor string) (uuid.UUID, error) {
	newUUID, err := uuid.NewRandom()
	if err != nil {
		return uuid.Nil, fmt.Errorf("claude_code: bootstrap: generate UUID: %w", err)
	}

	timeout := d.bootstrapTimeout
	if timeout <= 0 {
		timeout = DefaultBootstrapTimeout
	}
	bootCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(bootCtx, d.binaryPath,
		"--session-id", newUUID.String(),
		"--model", "claude-opus-4-7",
		"--print", bootstrapPrompt,
	)
	cmd.Dir = d.workingDir

	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			return uuid.Nil, fmt.Errorf(
				"claude_code: bootstrap: claude --session-id %s exited %d (timeout=%s); stderr: %s",
				newUUID, ee.ExitCode(), timeout, stderr,
			)
		}
		// Context-deadline / exec lookup errors land here.
		if bootCtx.Err() == context.DeadlineExceeded {
			return uuid.Nil, fmt.Errorf(
				"claude_code: bootstrap: timed out after %s spawning claude (--session-id %s); underlying: %w",
				timeout, newUUID, err,
			)
		}
		return uuid.Nil, fmt.Errorf("claude_code: bootstrap: exec claude --session-id %s: %w", newUUID, err)
	}

	// §107 anti-bluff: do NOT trust exit=0 alone — the claude binary
	// has been observed to print empty stdout on auth-without-session
	// edge cases. Require a non-empty stdout body so we can prove the
	// model actually responded.
	if len(strings.TrimSpace(string(out))) == 0 {
		return uuid.Nil, fmt.Errorf(
			"claude_code: bootstrap: claude --session-id %s exited 0 but stdout was empty (§107 bluff guard)",
			newUUID,
		)
	}

	if err := d.PersistSession(newUUID, anchor); err != nil {
		return uuid.Nil, fmt.Errorf("claude_code: bootstrap: persist anchor %s: %w", anchor, err)
	}
	return newUUID, nil
}

// bootstrapTimeoutOrDefault returns the active bootstrap timeout, used
// by tests that want to assert the default value without poking at
// private fields.
func (d *Dispatcher) bootstrapTimeoutOrDefault() time.Duration {
	if d.bootstrapTimeout <= 0 {
		return DefaultBootstrapTimeout
	}
	return d.bootstrapTimeout
}
