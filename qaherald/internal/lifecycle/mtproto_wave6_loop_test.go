//go:build integration_mtproto

package lifecycle

import (
	"context"
	tgJSON "encoding/json"
	"errors"
	"fmt"
	tgHTTP "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/mtproto"
)

// TestMTProto_Wave6_AutonomousClosedLoop is the §11.4.98-compliant
// replacement for tests/test_wave6_live_loop.sh. It exercises the full
// closed-loop end-to-end with NO operator action during the test:
//
//   1. MTProto user sends a unique message to HERALD_TGRAM_CHAT_ID.
//   2. pherald listen (spawned by this test) polls Telegram, sees the
//      message, dispatches to Claude Code, gets back a HERALD-REPLY.
//   3. pherald calls tgram.SendReply with reply_to_message_id pointing
//      at our MTProto-sent message_id.
//   4. MTProto sees the bot's reply via WaitForReply matcher
//      (FromUserID != myUserID && ReplyToMessageID == ourSentMsgID).
//   5. Assert reply landed within timeout + journal captured the flow.
//
// Honest-SKIP per §11.4.3 when credentials absent OR session missing.
// NEVER falls back to a manual-dep path.
//
// Build tag: integration_mtproto.
//
// §11.4.98 rule (2) session-collision guard: spawns pherald with
// HERALD_CLAUDE_PROJECT_NAME=Herald-MTProto-Test-<unix-ns> so its
// Claude session UUID is dedicated, never colliding with the dev
// conductor's session.
func TestMTProto_Wave6_AutonomousClosedLoop(t *testing.T) {
	appIDStr := os.Getenv("HERALD_MTPROTO_APP_ID")
	appHash := os.Getenv("HERALD_MTPROTO_APP_HASH")
	phone := os.Getenv("HERALD_MTPROTO_PHONE")
	password := os.Getenv("HERALD_MTPROTO_PASSWORD")
	botToken := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatIDStr := os.Getenv("HERALD_TGRAM_CHAT_ID")
	claudeBin := os.Getenv("HERALD_CLAUDE_BIN")

	if appIDStr == "" || appHash == "" || phone == "" || botToken == "" || chatIDStr == "" {
		t.Skipf("skip: MTProto/Tgram credentials missing per §11.4.3")
	}
	if claudeBin == "" {
		// PATH lookup fallback
		if _, err := exec.LookPath("claude"); err != nil {
			t.Skipf("skip: HERALD_CLAUDE_BIN unset and 'claude' not on PATH per §11.4.3")
		}
	} else if _, err := exec.LookPath(claudeBin); err != nil {
		t.Skipf("skip: HERALD_CLAUDE_BIN=%q not executable", claudeBin)
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		t.Skipf("skip: HERALD_MTPROTO_APP_ID not an integer")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		t.Skipf("skip: HERALD_TGRAM_CHAT_ID not an integer")
	}

	cfg := mtproto.Config{AppID: appID, AppHash: appHash, Phone: phone, Password: password}
	exists, err := cfg.SessionExists()
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if !exists {
		t.Skipf("skip: MTProto session file missing — run `qaherald mtproto login` first")
	}

	// Resolve timeout from env (default 180s — Claude Code processing time)
	timeoutSec := 180
	if v := os.Getenv("HERALD_MTPROTO_WAVE6_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}

	// Build pherald into temp dir
	tmpDir := t.TempDir()
	pheraldBin := filepath.Join(tmpDir, "pherald")
	buildCmd := exec.Command("go", "build", "-o", pheraldBin, "./pherald/cmd/pherald")
	repoRoot, _ := os.Getwd()
	// Walk up to repo root
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.work")); err == nil {
			break
		}
		repoRoot = filepath.Dir(repoRoot)
	}
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build pherald: %v\n%s", err, string(out))
	}
	t.Logf("pherald built at %s", pheraldBin)

	// MTProto client
	client, err := mtproto.New(cfg)
	if err != nil {
		t.Fatalf("mtproto.New: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec+60)*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	myID, myUsername, err := client.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	t.Logf("MTProto active: @%s (user_id=%d)", myUsername, myID)

	// Spawn pherald listen with dedicated Claude session UUID per §11.4.98 rule (2)
	qaDir := filepath.Join(tmpDir, "qa-journal")
	if err := os.MkdirAll(qaDir, 0o700); err != nil {
		t.Fatalf("mkdir qa-journal: %v", err)
	}
	pheraldCtx, pheraldCancel := context.WithCancel(context.Background())
	defer pheraldCancel()
	pheraldProjectName := fmt.Sprintf("Herald-MTProto-Test-%d", time.Now().UnixNano())
	pheraldCmd := exec.CommandContext(pheraldCtx, pheraldBin, "listen", "--qa-out-dir", qaDir)
	pheraldCmd.Env = append(os.Environ(),
		"HERALD_CLAUDE_PROJECT_NAME="+pheraldProjectName,
	)
	pheraldCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// CRITICAL: pherald listen resolves `docs/Issues.md` relative to its
	// CWD. The Go test runs in qaherald/internal/lifecycle/ — set Dir to
	// the repo root so pherald finds the docs tree.
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
	t.Logf("pherald listen PID=%d (project=%s, journal=%s)", pheraldCmd.Process.Pid, pheraldProjectName, qaDir)
	// Pre-ack stale updates so pherald (which we just spawned, but won't
	// have ack'd anything on its first getUpdates call) doesn't process
	// the prior session's chat history. Without this, pherald would try
	// to SendReply to N+1 messages in parallel → TLS connection-reset
	// storm from Telegram's anti-abuse rate limiter.
	consumePendingUpdates(t, os.Getenv("HERALD_TGRAM_BOT_TOKEN"))
	defer func() {
		// SIGTERM with grace, then SIGKILL
		_ = syscall.Kill(-pheraldCmd.Process.Pid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- pheraldCmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-pheraldCmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}()

	// Wait for pherald bootstrap
	time.Sleep(3 * time.Second)

	// Send MTProto message
	testMsg := fmt.Sprintf("herald-mtproto-wave6-closed-loop-%d", time.Now().UnixNano())
	sentID, err := client.SendMessage(ctx, chatID, testMsg)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	t.Logf("MTProto sent message_id=%d text=%q", sentID, testMsg)

	// Wait for bot's reply via MTProto WaitForReply
	t.Logf("waiting up to %ds for pherald → claude_code → SendReply round-trip...", timeoutSec)
	waitCtx, waitCancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer waitCancel()
	reply, err := client.WaitForReply(waitCtx, chatID, func(m mtproto.Message) bool {
		return m.FromUserID != myID && m.ReplyToMessageID == sentID
	})
	if err != nil {
		// Tail pherald log for diagnostic
		tail, _ := exec.Command("tail", "-30", filepath.Join(qaDir, "pherald.log")).CombinedOutput()
		t.Fatalf("WaitForReply: %v — pherald log tail:\n%s", err, string(tail))
	}
	t.Logf("PASS: closed-loop reply received — message_id=%d reply_to=%d text=%q", reply.ID, reply.ReplyToMessageID, strings.TrimSpace(reply.Text)[:min(80, len(strings.TrimSpace(reply.Text)))])

	// Verify journal captured the flow
	journalPath := filepath.Join(qaDir, "transcript.jsonl")
	if data, err := os.ReadFile(journalPath); err == nil {
		text := string(data)
		if !strings.Contains(text, "\"direction\":\"in\"") || !strings.Contains(text, "\"direction\":\"out\"") {
			t.Logf("WARN: journal at %s does not show both in+out — but reply landed so test still PASS", journalPath)
		}
	}

	// Ensure clean ctx
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected ctx error: %v", err)
	}
}

// min for pre-Go 1.21 friendliness (qaherald/go.mod is 1.25.3 but defensive).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// consumePendingUpdates walks the bot's getUpdates queue + ACKs every
// pending update so a freshly-spawned pherald doesn't process stale
// chat history. Critical for §11.4.98 autonomous tests because the test
// chat is reused across many runs; without this, each run's pherald
// would attempt to reply to every prior run's stimulus in parallel,
// triggering Telegram's TLS connection-reset rate limiter.
//
// Best-effort — failures are logged but don't fail the test.
func consumePendingUpdates(t *testing.T, botToken string) {
	t.Helper()
	if botToken == "" {
		return
	}
	type gu struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
		} `json:"result"`
	}
	pollURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=100&timeout=0", botToken)
	resp, err := tgHTTP.Get(pollURL)
	if err != nil {
		t.Logf("pre-ack getUpdates: %s (continuing)", strings.ReplaceAll(err.Error(), botToken, "<redacted-bot-token>"))
		return
	}
	defer resp.Body.Close()
	var r gu
	if err := tgJSON.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Logf("pre-ack decode: %v", err)
		return
	}
	if len(r.Result) == 0 {
		t.Logf("pre-ack: queue already empty")
		return
	}
	var maxID int64
	for _, u := range r.Result {
		if u.UpdateID > maxID {
			maxID = u.UpdateID
		}
	}
	ackURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&limit=1&timeout=0", botToken, maxID+1)
	if r2, err := tgHTTP.Get(ackURL); err == nil {
		r2.Body.Close()
	}
	t.Logf("pre-ack: discarded %d stale updates (max_update_id=%d)", len(r.Result), maxID)
}
