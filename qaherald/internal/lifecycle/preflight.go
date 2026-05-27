// Pre-flight validator (T6).
//
// `qaherald lifecycle` REFUSES to start when any precondition is
// missing. There is NO silent degradation — every gate that fails
// returns a *PreflightError carrying a DISTINCT exit code so the
// shell-script adapter (tests/test_wave6.5_lifecycle.sh) can branch
// on the failure class, and every failure message points the operator
// at docs/guides/OPERATOR_CREDENTIALS.md.
//
// §107 anti-bluff posture: each gate asserts on a CONCRETE value
// drawn from a real Preflight() round-trip (or a real os.Stat / env
// read). A no-op validator that returned nil would FAIL every gate-
// FAIL unit test in preflight_test.go because those tests feed a stub
// messenger whose canned PreflightReport (or error) drives each gate.
//
// Gate catalogue:
//
//	G1  pherald-bot present in chat (rep.PheraldBotPresent) + best-
//	    effort OTel-port liveness when HERALD_OTEL_PORT is set     → exit 2
//	G2  qa-bot getMe succeeds + non-empty username                 → exit 2
//	G3  qa-bot Privacy Mode disabled (CanReadAllGroupMessages)     → exit 3
//	G4  qa-bot in chat AND chat type is group|supergroup           → exit 4
//	G5  qa-bot username != pherald-bot username                    → exit 5
//	G6  HERALD_QA_BOT_TOKEN != HERALD_TGRAM_BOT_TOKEN (both set)    → exit 5
//	G7  docs-dir/Issues.md AND docs-dir/Fixed.md exist             → exit 6
//	G8  pherald-qa-out-dir exists (when configured)                → exit 6
//	G9  qa-bot user-id resolved (informational; no FAIL)           → n/a
//	G10 non-op bot distinct from main + NOT in HERALD_OPERATOR_IDS → exit 7
package lifecycle

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// PreflightError is the structured failure type every gate returns.
// ExitCode is consumed by the Cobra command (and, transitively, the
// shell-script adapter) to surface a distinct process exit per gate
// class. Gate is the G-tag; Reason is the human-readable cause.
type PreflightError struct {
	Gate     string
	ExitCode int
	Reason   string
}

func (e *PreflightError) Error() string {
	return fmt.Sprintf("[%s] %s (exit %d) — see docs/guides/OPERATOR_CREDENTIALS.md",
		e.Gate, e.Reason, e.ExitCode)
}

// PreflightExitCode extracts the gate exit code from err when err is a
// *PreflightError (possibly wrapped). Returns (code, true) on a match,
// (0, false) otherwise. The Cobra command uses this to map a preflight
// failure to a process exit code.
func PreflightExitCode(err error) (int, bool) {
	for e := err; e != nil; {
		if pe, ok := e.(*PreflightError); ok {
			return pe.ExitCode, true
		}
		// errors.Unwrap-style walk without importing errors twice.
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	return 0, false
}

// runPreflight runs all 10 gates against the configured messenger(s)
// and config. It returns the FIRST gate failure as a *PreflightError,
// or nil when every gate is green.
//
// writeEvent is the orchestrator's transcript closure; each gate
// outcome (PASS report + any FAIL) is journaled so the qaherald-side
// transcript.jsonl carries the preflight evidence chain. Passing
// writeEvent (rather than the 4-arg shape) is a strict superset of the
// plan's signature — it does not weaken any gate, it only adds the
// §107 evidence anchor the orchestrator already wires.
func runPreflight(ctx context.Context, msgr, msgrNonOp messenger.MessengerClient, cfg Config, writeEvent func(direction, kind string, payload any)) error {
	emit := func(kind string, payload any) {
		if writeEvent != nil {
			writeEvent("in", kind, payload)
		}
	}

	// ---- G2 — qa-bot getMe + non-empty username --------------------
	rep, err := msgr.Preflight(ctx, cfg.ChatID)
	if err != nil {
		pe := &PreflightError{Gate: "G2", ExitCode: 2, Reason: fmt.Sprintf("qa-bot getMe failed: %v", err)}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}
	emit("preflight.operator", rep)
	if strings.TrimSpace(rep.Username) == "" {
		pe := &PreflightError{Gate: "G2", ExitCode: 2, Reason: "qa-bot getMe returned an empty username (HERALD_QA_BOT_TOKEN invalid?)"}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}

	// ---- G9 — qa-bot user-id resolved (informational, no FAIL) -----
	emit("preflight.identity", map[string]any{
		"gate":     "G9",
		"username": rep.Username,
		"user_id":  rep.UserID,
	})

	// ---- G5 — qa-bot username != pherald-bot username --------------
	if cfg.PheraldBotUsername != "" && strings.EqualFold(rep.Username, cfg.PheraldBotUsername) {
		pe := &PreflightError{Gate: "G5", ExitCode: 5, Reason: fmt.Sprintf("qa-bot username (%s) equals pherald-bot username — wrong token", rep.Username)}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}

	// ---- G6 — qa-bot token distinct from pherald-bot token ---------
	if opToken := os.Getenv("HERALD_TGRAM_BOT_TOKEN"); opToken != "" && cfg.QABotToken != "" && opToken == cfg.QABotToken {
		pe := &PreflightError{Gate: "G6", ExitCode: 5, Reason: "HERALD_QA_BOT_TOKEN equals HERALD_TGRAM_BOT_TOKEN — use distinct bots"}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}

	// ---- G3 — qa-bot Privacy Mode disabled -------------------------
	if !rep.CanReadAllGroupMessages {
		pe := &PreflightError{Gate: "G3", ExitCode: 3, Reason: "qa-bot Privacy Mode is enabled — talk to @BotFather → /setprivacy → Disable"}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}

	// ---- G4 — qa-bot in chat AND chat type group|supergroup --------
	if !rep.InChat {
		pe := &PreflightError{Gate: "G4", ExitCode: 4, Reason: fmt.Sprintf("qa-bot is not a member of chat-id %d", cfg.ChatID)}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}
	if rep.ChatType != "group" && rep.ChatType != "supergroup" {
		pe := &PreflightError{Gate: "G4", ExitCode: 4, Reason: fmt.Sprintf("chat-id %d is type %q; expected group or supergroup", cfg.ChatID, rep.ChatType)}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}

	// ---- G7 — docs-dir/Issues.md AND docs-dir/Fixed.md exist -------
	docsDir := cfg.DocsDir
	if docsDir == "" {
		docsDir = "docs"
	}
	for _, name := range []string{"Issues.md", "Fixed.md"} {
		if _, statErr := os.Stat(filepath.Join(docsDir, name)); statErr != nil {
			pe := &PreflightError{Gate: "G7", ExitCode: 6, Reason: fmt.Sprintf("%s not found under --docs-dir %q", name, docsDir)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
	}

	// ---- G8 — pherald-qa-out-dir exists (when configured) ----------
	if cfg.PheraldQAOutDir != "" {
		if _, statErr := os.Stat(cfg.PheraldQAOutDir); statErr != nil {
			pe := &PreflightError{Gate: "G8", ExitCode: 6, Reason: fmt.Sprintf("--pherald-qa-out-dir %q does not exist (pherald listen not configured to write there?)", cfg.PheraldQAOutDir)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
	}

	// ---- G1 — pherald-bot present in chat --------------------------
	if !rep.PheraldBotPresent {
		pe := &PreflightError{Gate: "G1", ExitCode: 2, Reason: fmt.Sprintf("pherald-bot username %q not found in chat — is pherald listen running and joined?", cfg.PheraldBotUsername)}
		emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
		return pe
	}

	// ---- G1.5 — best-effort OTel-port liveness (only when set) -----
	if portStr := os.Getenv("HERALD_OTEL_PORT"); portStr != "" {
		port, convErr := strconv.Atoi(portStr)
		if convErr != nil {
			pe := &PreflightError{Gate: "G1", ExitCode: 2, Reason: fmt.Sprintf("HERALD_OTEL_PORT %q is not a valid port number", portStr)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
		conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)), 2*time.Second)
		if dialErr != nil {
			pe := &PreflightError{Gate: "G1", ExitCode: 2, Reason: fmt.Sprintf("pherald OTel port %d unreachable: %v", port, dialErr)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
		_ = conn.Close()
	}

	// ---- G10 — non-op bot distinct + NOT in HERALD_OPERATOR_IDS ----
	if msgrNonOp != nil {
		repNon, nonErr := msgrNonOp.Preflight(ctx, cfg.ChatID)
		if nonErr != nil {
			pe := &PreflightError{Gate: "G10", ExitCode: 7, Reason: fmt.Sprintf("non-operator qa-bot getMe failed: %v", nonErr)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
		emit("preflight.non_operator", repNon)
		if repNon.UserID == rep.UserID {
			pe := &PreflightError{Gate: "G10", ExitCode: 7, Reason: fmt.Sprintf("non-operator qa-bot user-id %d equals main qa-bot user-id — supply a DISTINCT 2nd bot", repNon.UserID)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
		if inOperatorAllowlist(os.Getenv("HERALD_OPERATOR_IDS"), repNon.UserID) {
			pe := &PreflightError{Gate: "G10", ExitCode: 7, Reason: fmt.Sprintf("non-operator qa-bot user-id %d is in HERALD_OPERATOR_IDS — defeats the purpose of S9", repNon.UserID)}
			emit("preflight.fail", map[string]string{"gate": pe.Gate, "reason": pe.Reason})
			return pe
		}
	}

	emit("preflight.pass", map[string]any{
		"qa_bot":      rep.Username,
		"pherald_bot": cfg.PheraldBotUsername,
		"chat_type":   rep.ChatType,
		"gates":       "G1..G10 green",
	})
	return nil
}

// inOperatorAllowlist reports whether userID appears as a comma-
// separated entry in the HERALD_OPERATOR_IDS-style allowlist string.
// Whitespace around each entry is trimmed; empty entries are ignored.
func inOperatorAllowlist(allowlist string, userID int64) bool {
	want := strconv.FormatInt(userID, 10)
	for _, raw := range strings.Split(allowlist, ",") {
		if strings.TrimSpace(raw) == want {
			return true
		}
	}
	return false
}
