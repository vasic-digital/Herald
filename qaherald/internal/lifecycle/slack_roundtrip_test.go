//go:build integration

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// TestSlack_LiveRoundTrip is the §11.4.98-compliant, fully-automated Slack
// inbound round-trip harness — the Slack analog of
// mtproto_wave6_loop_test.go's TestMTProto_Wave6_AutonomousClosedLoop. It
// closes HRD-115 (Slack send-side is already live-proven by
// TestSlack_Live_Send / E127; only the inbound ROUND-TRIP remains).
//
// The closed loop, with ZERO human action during execution:
//
//  1. A QA *user* identity (HERALD_SLACK_QA_USER_TOKEN, an xoxp-… user OAuth
//     token) posts a deterministic, nonce-bearing probe message into the
//     channel via chat.postMessage — posting AS THE USER, not as the bot.
//     This is the §32.9 echo-wall solution: a Slack bot never receives its
//     OWN messages over Socket Mode, so the bot side could never observe a
//     bot-authored probe. A SEPARATE user identity is mandatory.
//
//  2. The pherald Slack bot (spawned by this test as a `pherald listen
//     --channels slack` subprocess, wired to HERALD_SLACK_BOT_TOKEN +
//     HERALD_SLACK_APP_TOKEN) receives the user message over Socket Mode,
//     dispatches it to Claude Code, parses <<<HERALD-REPLY>>>, and replies.
//
//  3. This test asserts the full round-trip via two legs:
//     LEG 1 — autonomy chain (Tier-2, Claude Code), proven via the pherald
//     --qa-out-dir JSONL journal: the inbound message line (kind is
//     channel-agnostic in journal.go; the payload carries "channel":"slack")
//     carrying our probe text proves pherald RECEIVED our user-posted
//     message; `cc.dispatch` proves it DISPATCHED; `cc.reply` proves Claude
//     RESPONDED. The CC reply-DELIVERY to Slack is only a soft note here — a
//     fresh, context-less bootstrap session returns empty reply text, so
//     nothing is posted back (HRD-159, a pre-existing CROSS-CHANNEL gap; the
//     Telegram round-trip masks the same gap with a soft BONUS).
//     LEG 2 — reply-DELIVERY (HARD-asserted), proven DETERMINISTICALLY via the
//     Tier-1 fast-path: the QA user posts a natural-language status query
//     ("what is the status of ATM-<pid>?") which the deterministic
//     CommandRecognizer answers WITHOUT a Claude round-trip
//     ("Looking up the status of ATM-<pid>…") via slack.SendReply; the QA user
//     reads that reply back from Slack (ts>probe, unique id echoed), proving
//     the reply-back-to-Slack leg really lands — independent of CC session
//     fidelity.
//
//  4. Teardown SIGTERMs the listen subprocess and asserts a clean exit.
//
//  5. A redacted transcript (tokens + wss:// scrubbed) is written under
//     docs/qa/HRD-115-LIVE-roundtrip-<TS>/ as the §107.x evidence artefact.
//
// Honest-SKIP per §11.4.3 when ANY required credential is absent — NEVER a
// fake pass, NEVER a manual-action fallback. The new credential this harness
// introduces is HERALD_SLACK_QA_USER_TOKEN (the user side); the bot side
// reuses the already-present HERALD_SLACK_BOT_TOKEN / _APP_TOKEN /
// _CHANNEL_ID.
//
// Build tag: integration (matches slack_live_integration_test.go).
//
// §11.4.98 rule (2) session-collision guard: spawns pherald with
// HERALD_CLAUDE_PROJECT_NAME=Herald-Slack-Test-<unix-ns> so its Claude
// session UUID is dedicated, never colliding with the dev conductor's session.
func TestSlack_LiveRoundTrip(t *testing.T) {
	botToken := os.Getenv("HERALD_SLACK_BOT_TOKEN")
	appToken := os.Getenv("HERALD_SLACK_APP_TOKEN")
	channelID := os.Getenv("HERALD_SLACK_CHANNEL_ID")
	qaUserToken := os.Getenv("HERALD_SLACK_QA_USER_TOKEN")
	claudeBin := os.Getenv("HERALD_CLAUDE_BIN")

	if botToken == "" || appToken == "" || channelID == "" || qaUserToken == "" {
		t.Skipf("skip: Slack round-trip credentials missing per §11.4.3 " +
			"(HERALD_SLACK_BOT_TOKEN + HERALD_SLACK_APP_TOKEN + HERALD_SLACK_CHANNEL_ID + HERALD_SLACK_QA_USER_TOKEN required; " +
			"the QA user token is the xoxp-… user OAuth token (chat:write + channels:history) that drives the user side)")
	}
	if claudeBin == "" {
		if _, err := exec.LookPath("claude"); err != nil {
			t.Skipf("skip: HERALD_CLAUDE_BIN unset and 'claude' not on PATH per §11.4.3")
		}
	} else if _, err := exec.LookPath(claudeBin); err != nil {
		t.Skipf("skip: HERALD_CLAUDE_BIN=%q not executable", claudeBin)
	}

	// Resolve timeout from env (default 180s — Claude Code processing time).
	timeoutSec := 180
	if v := os.Getenv("HERALD_SLACK_ROUNDTRIP_TIMEOUT_SEC"); v != "" {
		if n, perr := slackAtoiPositive(v); perr == nil {
			timeoutSec = n
		}
	}

	// Locate the repo root (the test runs in qaherald/internal/lifecycle/).
	repoRoot, err := slackFindRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}

	// Build pherald into a temp dir.
	tmpDir := t.TempDir()
	pheraldBin := filepath.Join(tmpDir, "pherald")
	buildCmd := exec.Command("go", "build", "-o", pheraldBin, "./pherald/cmd/pherald")
	buildCmd.Dir = repoRoot
	if out, berr := buildCmd.CombinedOutput(); berr != nil {
		t.Fatalf("build pherald: %v\n%s", berr, string(out))
	}
	t.Logf("pherald built at %s", pheraldBin)

	// QA user-side Slack client (drives the USER side: chat.postMessage posts
	// AS the user via the xoxp- token; conversations.history reads the bot's
	// reply). The empty baseURL defaults to https://slack.com/api — real bytes.
	qaUser := messenger.NewSlackClient(qaUserToken, channelID, "")
	defer qaUser.Close()

	// Confirm the QA user identity up-front (fail loud if the token is a bot
	// token by mistake — a bot xoxb- token would still auth.test OK but the
	// echo-wall solution requires a USER identity distinct from the bot).
	identCtx, identCancel := context.WithTimeout(context.Background(), 30*time.Second)
	qaUsername, _, ierr := qaUser.Me(identCtx)
	identCancel()
	if ierr != nil {
		t.Fatalf("QA user auth.test failed (is HERALD_SLACK_QA_USER_TOKEN a valid xoxp- user token?): %v", ierr)
	}
	t.Logf("QA user identity: %s", qaUsername)

	// Spawn pherald listen --channels slack with a dedicated Claude session
	// UUID per §11.4.98 rule (2). Journal to <tmp>/qa-journal/transcript.jsonl.
	qaDir := filepath.Join(tmpDir, "qa-journal")
	if err := os.MkdirAll(qaDir, 0o700); err != nil {
		t.Fatalf("mkdir qa-journal: %v", err)
	}
	pheraldCtx, pheraldCancel := context.WithCancel(context.Background())
	defer pheraldCancel()
	pheraldProjectName := fmt.Sprintf("Herald-Slack-Test-%d", time.Now().UnixNano())
	pheraldCmd := exec.CommandContext(pheraldCtx, pheraldBin, "listen", "--channels", "slack", "--qa-out-dir", qaDir)
	pheraldCmd.Env = append(os.Environ(),
		"HERALD_CLAUDE_PROJECT_NAME="+pheraldProjectName,
		// Pin the channel set on the spawned process so it does not pick up a
		// stray HERALD_CHANNELS from the inherited env.
		"HERALD_CHANNELS=slack",
	)
	pheraldCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// pherald listen resolves docs/Issues.md relative to its CWD; run it from
	// the repo root so it finds the docs tree (same as the MTProto harness).
	pheraldCmd.Dir = repoRoot
	logFile, err := os.Create(filepath.Join(qaDir, "pherald.log"))
	if err != nil {
		t.Fatalf("create pherald.log: %v", err)
	}
	defer logFile.Close()
	pheraldCmd.Stdout = logFile
	pheraldCmd.Stderr = logFile
	if err := pheraldCmd.Start(); err != nil {
		t.Fatalf("start pherald: %v", err)
	}
	t.Logf("pherald listen --channels slack PID=%d (project=%s, journal=%s)", pheraldCmd.Process.Pid, pheraldProjectName, qaDir)

	// Teardown: SIGTERM with grace, then SIGKILL; assert clean exit.
	var pheraldExitErr error
	teardownDone := false
	teardown := func() {
		if teardownDone {
			return
		}
		teardownDone = true
		_ = syscall.Kill(-pheraldCmd.Process.Pid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- pheraldCmd.Wait() }()
		select {
		case pheraldExitErr = <-done:
		case <-time.After(8 * time.Second):
			_ = syscall.Kill(-pheraldCmd.Process.Pid, syscall.SIGKILL)
			pheraldExitErr = <-done
		}
	}
	defer teardown()

	// Wait for the Socket Mode connection to come up. The slack adapter logs
	// to pherald.log; poll for the listen startup line AND give the WebSocket
	// a moment to finish apps.connections.open. We gate on the startup line
	// (deterministic) then add a short settle so the first inbound is not
	// dropped before the socket is live.
	if err := slackWaitForListenReady(t, filepath.Join(qaDir, "pherald.log"), 30*time.Second); err != nil {
		tail, _ := exec.Command("tail", "-40", filepath.Join(qaDir, "pherald.log")).CombinedOutput()
		t.Fatalf("pherald listen did not signal Slack readiness within 30s: %v\nlog tail:\n%s", err, string(tail))
	}
	// Socket Mode settle: the startup line prints before RunContext finishes
	// the WebSocket handshake. A short fixed settle avoids a lost-first-message
	// race without depending on a log line the SDK does not emit deterministically.
	time.Sleep(4 * time.Second)

	// Post the deterministic, nonce-bearing probe AS THE QA USER.
	// Nonce derived from the probe content + project name (NOT random/time-only)
	// so it is unique-per-run yet reconstructable for the transcript.
	nonce := fmt.Sprintf("%s-%d", pheraldProjectName, pheraldCmd.Process.Pid)
	// A real instruction that elicits a deterministic NON-EMPTY reply containing
	// the nonce — so (a) Claude produces reply text (an empty reply is a no-op,
	// proving nothing about the reply-back-to-Slack leg) and (b) the reply we
	// observe in Slack is unambiguously THIS round-trip's reply, not a stale
	// message from an earlier run.
	probe := "Please reply with a brief acknowledgement that includes this exact token verbatim: " + nonce
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 30*time.Second)
	probeTS, serr := qaUser.Send(sendCtx, probe)
	sendCancel()
	if serr != nil {
		t.Fatalf("QA user Send (chat.postMessage) failed: %v", serr)
	}
	t.Logf("QA user posted probe ts=%s text=%q", probeTS, probe)

	// §11.4.98 autonomy assertion via the pherald journal (evidence channel a).
	journalPath := filepath.Join(qaDir, "transcript.jsonl")
	t.Logf("waiting up to %ds for journal entries proving autonomy chain (in/dispatch/reply)...", timeoutSec)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	var sawIn, sawDispatch, sawReply bool
	for time.Now().Before(deadline) {
		if data, rerr := os.ReadFile(journalPath); rerr == nil {
			text := string(data)
			if !sawIn && strings.Contains(text, `"kind":"tgram.message"`) && strings.Contains(text, probe) {
				sawIn = true
				t.Logf("✓ journal: inbound message carrying our probe text (channel:slack)")
			}
			if !sawDispatch && strings.Contains(text, `"kind":"cc.dispatch"`) && strings.Contains(text, probe) {
				sawDispatch = true
				t.Logf("✓ journal: cc.dispatch out referencing our probe text")
			}
			if !sawReply && strings.Contains(text, `"kind":"cc.reply"`) {
				sawReply = true
				t.Logf("✓ journal: cc.reply in — Claude responded")
			}
		}
		if sawIn && sawDispatch && sawReply {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if !sawIn || !sawDispatch || !sawReply {
		tail, _ := exec.Command("tail", "-40", filepath.Join(qaDir, "pherald.log")).CombinedOutput()
		t.Fatalf("§11.4.98 autonomy chain INCOMPLETE: in=%v dispatch=%v reply=%v — pherald log tail:\n%s",
			sawIn, sawDispatch, sawReply, string(tail))
	}
	t.Logf("PASS: §11.4.98 autonomy chain proven via journal — Slack user → bot inbound (Socket Mode) → Claude dispatch → Claude reply")

	// SOFT note (Tier-2 / CC delivery): try to read the Claude reply back. In
	// this harness pherald bootstraps a FRESH, context-less Claude session which
	// returns EMPTY reply text (pherald.log "reply skipped — empty reply text"),
	// so the CC reply is typically NOT delivered. That specific gap is tracked by
	// HRD-159 (cross-channel — the Telegram round-trip has it too). We do NOT
	// hard-assert it here; reply-DELIVERY is instead PROVEN below via the
	// deterministic Tier-1 fast-path (which does not depend on CC session
	// fidelity). ts>probe freshness-gates against any stale earlier-run message.
	ccProbeCtx, ccProbeCancel := context.WithTimeout(context.Background(), 20*time.Second)
	if reply, werr := qaUser.WaitForReply(ccProbeCtx, probeTS, func(r messenger.Reply) bool {
		return r.SenderIsBot && slackTSAfter(string(r.MessageID), string(probeTS)) && r.Text != probe
	}, 18*time.Second); werr == nil {
		t.Logf("note: CC reply also delivered (bonus, ts=%s) text=%q nonce_echoed=%v",
			reply.MessageID, slackTrunc(reply.Text, 80), strings.Contains(reply.Text, nonce))
	} else {
		t.Logf("note: CC (Tier-2) reply not delivered — fresh bootstrap session returns empty text (HRD-159, cross-channel): %v", werr)
	}
	ccProbeCancel()

	// HARD assertion — reply-DELIVERY leg (§11.4.98), proven DETERMINISTICALLY
	// via the Tier-1 fast-path. A natural-language status query ("what is the
	// status of ATM-<pid>?") is recognized by the deterministic CommandRecognizer
	// (NO Claude round-trip — so it does not depend on CC session fidelity) and
	// pherald replies "Looking up the status of ATM-<pid>…" via slack.SendReply.
	// The QA user reading that reply back from Slack proves the FULL round-trip
	// reply-DELIVERY leg really lands. The id token (ATM-<pid>) is unique-per-run
	// and echoed in the deterministic reply, giving unambiguous provenance; the
	// ts>probe2 gate excludes any stale earlier message.
	idToken := fmt.Sprintf("ATM-%d", pheraldCmd.Process.Pid)
	cmdProbe := "What is the status of " + idToken + "?"
	send2Ctx, send2Cancel := context.WithTimeout(context.Background(), 30*time.Second)
	probe2TS, serr2 := qaUser.Send(send2Ctx, cmdProbe)
	send2Cancel()
	if serr2 != nil {
		t.Fatalf("QA user Send (Tier-1 status-query probe) failed: %v", serr2)
	}
	t.Logf("QA user posted Tier-1 status-query probe ts=%s text=%q", probe2TS, cmdProbe)

	var sawCmdReply bool
	var deliveredReply messenger.Reply
	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 45*time.Second)
	if reply, werr := qaUser.WaitForReply(cmdCtx, probe2TS, func(r messenger.Reply) bool {
		// Bot-authored, strictly newer than this probe, carrying the unique id
		// token the deterministic Tier-1 reply echoes ("Looking up the status of
		// ATM-<pid>…"). ts>probe2 excludes any stale earlier message.
		return r.SenderIsBot && slackTSAfter(string(r.MessageID), string(probe2TS)) && strings.Contains(r.Text, idToken)
	}, 40*time.Second); werr == nil {
		sawCmdReply = true
		deliveredReply = reply
		t.Logf("✓ reply-DELIVERY PROVEN: Tier-1 reply landed in Slack (ts=%s > probe ts=%s) text=%q",
			reply.MessageID, probe2TS, slackTrunc(reply.Text, 90))
	} else {
		t.Logf("Tier-1 reply NOT observed in Slack within 40s: %v", werr)
	}
	cmdCancel()

	if !sawCmdReply {
		tail, _ := exec.Command("tail", "-25", filepath.Join(qaDir, "pherald.log")).CombinedOutput()
		t.Fatalf("§11.4.98 reply-DELIVERY leg FAILED: the deterministic Tier-1 reply (carrying %q) was not observed in Slack newer than the probe (ts=%s) — the round-trip reply did not reach Slack. pherald log tail:\n%s",
			idToken, probe2TS, string(tail))
	}

	// Teardown now (before writing evidence) so we can record the exit status.
	teardown()
	if pheraldExitErr != nil {
		// SIGTERM-induced exit surfaces as *exec.ExitError; that is the
		// expected clean-shutdown signal, not a failure. Only an unexpected
		// non-signal error is a problem.
		var exitErr *exec.ExitError
		if !errors.As(pheraldExitErr, &exitErr) {
			t.Errorf("pherald listen did not exit cleanly on SIGTERM: %v", pheraldExitErr)
		} else {
			t.Logf("pherald listen exited on SIGTERM (expected): %v", pheraldExitErr)
		}
	} else {
		t.Logf("pherald listen exited cleanly (status 0)")
	}

	// §107.x evidence: write a redacted transcript under docs/qa/.
	evidenceDir := filepath.Join(repoRoot, "docs", "qa",
		fmt.Sprintf("HRD-115-LIVE-roundtrip-%s", time.Now().UTC().Format("2006-01-02T15-04-05Z")))
	if werr := slackWriteEvidence(evidenceDir, slackEvidence{
		Nonce:          nonce,
		Probe:          probe,
		ProbeTS:        string(probeTS),
		QAUsername:     qaUsername,
		CmdProbe:       cmdProbe,
		CmdProbeTS:     string(probe2TS),
		IDToken:        idToken,
		ReplyDelivered: sawCmdReply,
		DeliveredReply: deliveredReply,
		JournalPath:    journalPath,
		LogPath:        filepath.Join(qaDir, "pherald.log"),
	}); werr != nil {
		t.Errorf("write §107.x evidence: %v", werr)
	} else {
		t.Logf("§107.x evidence written under %s", evidenceDir)
	}
}

// slackEvidence is the redacted transcript payload written under docs/qa/.
type slackEvidence struct {
	Nonce          string
	Probe          string // the Tier-2 CC autonomy-chain probe
	ProbeTS        string
	QAUsername     string
	CmdProbe       string // the Tier-1 deterministic reply-delivery probe
	CmdProbeTS     string
	IDToken        string // unique id echoed in the Tier-1 reply
	ReplyDelivered bool   // Tier-1 reply observed back in Slack (HARD-proven)
	DeliveredReply messenger.Reply
	JournalPath    string
	LogPath        string
}

// slackWriteEvidence writes a human-readable, REDACTED transcript +
// a copy of the (already token-free) JSONL journal and pherald.log under
// evidenceDir. Tokens (xoxb-/xapp-/xoxp-) and wss:// URLs are scrubbed from
// every byte copied out, satisfying the §107.x in-repo-evidence mandate
// without leaking credentials.
func slackWriteEvidence(evidenceDir string, ev slackEvidence) error {
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return fmt.Errorf("mkdir evidence dir: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# HRD-115 — Slack inbound round-trip (§11.4.98 self-driving)\n\n")
	fmt.Fprintf(&b, "QA user identity : %s\n\n", ev.QAUsername)
	fmt.Fprintf(&b, "## Leg 1 — autonomy chain (Tier-2, Claude Code)\n")
	fmt.Fprintf(&b, "CC probe text    : %s\n", ev.Probe)
	fmt.Fprintf(&b, "CC probe ts      : %s\n", ev.ProbeTS)
	fmt.Fprintf(&b, "Proven (journal) : inbound(channel:slack) -> cc.dispatch -> cc.reply (see transcript.jsonl)\n")
	fmt.Fprintf(&b, "Note             : CC reply-delivery in a fresh bootstrap session returns empty text (HRD-159, cross-channel)\n\n")
	fmt.Fprintf(&b, "## Leg 2 — reply-DELIVERY (Tier-1 deterministic fast-path)\n")
	fmt.Fprintf(&b, "Status-query     : %s\n", ev.CmdProbe)
	fmt.Fprintf(&b, "Query ts         : %s\n", ev.CmdProbeTS)
	fmt.Fprintf(&b, "Unique id token  : %s\n", ev.IDToken)
	fmt.Fprintf(&b, "Reply DELIVERED  : %v\n", ev.ReplyDelivered)
	if ev.ReplyDelivered {
		fmt.Fprintf(&b, "Delivered reply ts   : %s\n", ev.DeliveredReply.MessageID)
		fmt.Fprintf(&b, "Delivered reply text : %s\n", ev.DeliveredReply.Text)
	}
	fmt.Fprintf(&b, "\nDirection legend: USER -> (Slack) -> pherald bot -> {Claude Code | Tier-1 recognizer} -> (Slack reply) -> USER\n")
	if err := os.WriteFile(filepath.Join(evidenceDir, "TRANSCRIPT.md"), []byte(slackRedact(b.String())), 0o644); err != nil {
		return err
	}
	// Copy the JSONL journal + pherald.log, redacted (defence-in-depth; the
	// journal does not record tokens, but redact anyway).
	if data, err := os.ReadFile(ev.JournalPath); err == nil {
		_ = os.WriteFile(filepath.Join(evidenceDir, "transcript.jsonl"), []byte(slackRedact(string(data))), 0o644)
	}
	if data, err := os.ReadFile(ev.LogPath); err == nil {
		_ = os.WriteFile(filepath.Join(evidenceDir, "pherald.log"), []byte(slackRedact(string(data))), 0o644)
	}
	return nil
}

// slackRedact scrubs Slack tokens (xoxb-/xapp-/xoxp-) and wss:// URLs from s,
// replacing each match with a sentinel. Whitespace-delimited token scrubbing
// is sufficient because Slack tokens carry no internal spaces.
func slackRedact(s string) string {
	out := make([]string, 0, 64)
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		// Preserve original spacing by replacing matched substrings in-place
		// rather than re-joining fields (which would collapse whitespace).
		redacted := line
		for _, f := range fields {
			trimmed := strings.Trim(f, `"',.;:()[]{}`)
			if slackIsSecret(trimmed) {
				redacted = strings.ReplaceAll(redacted, trimmed, "[REDACTED]")
			}
		}
		out = append(out, redacted)
	}
	return strings.Join(out, "\n")
}

// slackIsSecret reports whether tok looks like a Slack token or a WebSocket URL.
func slackIsSecret(tok string) bool {
	switch {
	case strings.HasPrefix(tok, "xoxb-"),
		strings.HasPrefix(tok, "xapp-"),
		strings.HasPrefix(tok, "xoxp-"),
		strings.HasPrefix(tok, "wss://"):
		return true
	}
	return false
}

// slackWaitForListenReady polls the pherald log file until the `pherald
// listen: starting inbound runtime` startup line appears (the deterministic
// boot marker) or the timeout expires.
func slackWaitForListenReady(t *testing.T, logPath string, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(logPath); err == nil {
			if strings.Contains(string(data), "starting inbound runtime") {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("startup line 'starting inbound runtime' not observed")
}

// slackFindRepoRoot walks up from the test's CWD until it finds go.work.
func slackFindRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("go.work not found walking up from CWD")
}

// slackAtoiPositive parses a positive integer or returns an error.
func slackAtoiPositive(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a positive integer: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("not a positive integer: %q", s)
	}
	return n, nil
}

// slackTrunc truncates s to at most n runes for log readability.
func slackTrunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// slackTSAfter reports whether Slack message ts a is strictly after ts b.
// Slack timestamps are "<epoch>.<seq>" decimal strings; a float compare orders
// them correctly. A non-parseable ts is treated as NOT-after (conservative), so
// a malformed value can never let a stale message satisfy a freshness check.
func slackTSAfter(a, b string) bool {
	fa, ea := strconv.ParseFloat(a, 64)
	fb, eb := strconv.ParseFloat(b, 64)
	if ea != nil || eb != nil {
		return false
	}
	return fa > fb
}
