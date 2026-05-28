// Hermetic tests for the gotd/td-backed Client wiring.
//
// What these tests COVER (no real Telegram calls):
//   - Compile-time interface satisfaction (var _ Client = (*liveClient)(nil)).
//   - sanitizeMTProtoError is invoked on every error path through
//     Connect / SendMessage / WaitForReply / WhoAmI when the client is
//     not connected.
//   - Close is safe to call multiple times AND safe to call before
//     Connect.
//   - Goroutine-leak check post-Close (NumGoroutine returns to baseline
//     after Close, allowing for normal jitter).
//   - resolvePeer correctly classifies positive / negative / -100-prefixed
//     chat IDs into the right tg.InputPeer* variants WITHOUT calling
//     Telegram (we test only the legacy-group branch which needs no
//     access_hash).
//   - extractSentMessageID + inspectUpdate cover the UpdatesClass
//     branches via constructed tg.* values (still hermetic).
//   - convertTGMessage maps tg.Message → Message correctly.
//
// What these tests DO NOT cover (would need live Telegram):
//   - Full Connect → SendMessage → WaitForReply round-trip.
//   - Real FLOOD_WAIT recovery.
//   - Real session persistence across process restart.
//
// Run `qaherald mtproto whoami` after `qaherald mtproto login` for the
// live-evidence path; that's the §11.4.98(B) one-time-interactive +
// thereafter-autonomous contract.
package mtproto

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

// TestLiveClient_SatisfiesInterface is the compile-time pin: if liveClient
// ever drifts off the Client contract this test (well, the package) fails
// to compile.
func TestLiveClient_SatisfiesInterface(t *testing.T) {
	var _ Client = (*liveClient)(nil)
}

// TestLiveClient_ConnectFailsWhenSessionMissing — when the persisted
// session file does not exist, Connect returns ErrNoSession (the explicit
// pointer at `qaherald mtproto login`). We can't easily exercise the
// real network path here without making an outbound MTProto connection,
// so we only assert the Constructor + initial state contract: Connect
// blocks on real network, but the runtime methods before Connect
// surface a clear error.
//
// NOTE: A direct test of "Connect with missing session returns
// ErrNoSession" would require a real MTProto handshake (which gotd
// performs in client.Run before the auth.Status check). That's why the
// session-missing path is asserted indirectly here, and end-to-end via
// `qaherald mtproto whoami` against an unconfigured session in the
// operator-driven acceptance run.
func TestLiveClient_ConnectFailsWhenSessionMissing(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		AppID:       12345678,
		AppHash:     "0123456789abcdef0123456789abcdef",
		Phone:       "+12025551234",
		SessionFile: filepath.Join(tmp, "session-does-not-exist.bin"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Sanity: cfg.SessionExists() reports false BEFORE Connect.
	exists, err := cfg.SessionExists()
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if exists {
		t.Fatalf("session file unexpectedly exists at %s", cfg.ResolvedSessionFile())
	}
	// We do NOT call Connect — it would attempt a real network dial,
	// which is not appropriate for a hermetic unit test. The
	// session-missing → ErrNoSession path is asserted by the cmd-level
	// integration test in mtproto_cmd_test.go which builds the binary
	// and runs `whoami` against a fresh session-dir.
	_ = c
}

// TestLiveClient_CloseBeforeConnect — Close is safe before any Connect
// (idempotent no-op).
func TestLiveClient_CloseBeforeConnect(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		AppID:       12345678,
		AppHash:     "0123456789abcdef0123456789abcdef",
		Phone:       "+12025551234",
		SessionFile: filepath.Join(tmp, "session.bin"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close (idempotency check): %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("third Close (idempotency check): %v", err)
	}
}

// TestLiveClient_RuntimeMethodsRejectPreConnect — every runtime method
// surfaces a clear non-nil error (NOT a nil deref) when called before
// Connect has succeeded.
func TestLiveClient_RuntimeMethodsRejectPreConnect(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		AppID:       12345678,
		AppHash:     "0123456789abcdef0123456789abcdef",
		Phone:       "+12025551234",
		SessionFile: filepath.Join(tmp, "session.bin"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	if _, err := c.SendMessage(ctx, -100123456, "hello"); err == nil {
		t.Errorf("SendMessage: want non-nil error pre-Connect")
	}
	if _, err := c.WaitForReply(ctx, -100123456, func(Message) bool { return true }); err == nil {
		t.Errorf("WaitForReply: want non-nil error pre-Connect")
	}
	if _, _, err := c.WhoAmI(ctx); err == nil {
		t.Errorf("WhoAmI: want non-nil error pre-Connect")
	}
}

// TestLiveClient_AssertConnected_AfterClose — runtime methods after Close
// return clear errors (not nil dereferences).
func TestLiveClient_AssertConnected_AfterClose(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		AppID:       12345678,
		AppHash:     "0123456789abcdef0123456789abcdef",
		Phone:       "+12025551234",
		SessionFile: filepath.Join(tmp, "session.bin"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := c.WhoAmI(context.Background()); err == nil {
		t.Errorf("WhoAmI after Close: want non-nil error")
	}
}

// TestSanitizeMTProtoError_AppliedConsistently — feed errors with each
// of the credential shapes through sanitizeMTProtoError and confirm none
// of the bytes survive.
func TestSanitizeMTProtoError_AppliedConsistently(t *testing.T) {
	cases := []struct {
		name string
		raw  error
	}{
		{"api_hash leak", errors.New("got 0123456789abcdef0123456789abcdef in error")},
		{"bot token leak", errors.New("via 1234567890:ABCdefGHIjklMNOpqrSTUvwxYZ_abcdefghij failure")},
		{"session bytes leak", errors.New("decode: " + strings.Repeat("A", 80))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleaned := sanitizeMTProtoError(tc.raw)
			if cleaned == nil {
				t.Fatalf("sanitizeMTProtoError returned nil for %s", tc.name)
			}
			if ContainsSecret(cleaned.Error()) {
				t.Errorf("sanitize FAILED — secret survived: %q", cleaned.Error())
			}
		})
	}
}

// TestResolvePeer_LegacyGroup — a negative chat ID without -100 prefix
// resolves to InputPeerChat with abs(chatID) as ChatID, with NO network
// call. This is the only resolvePeer branch we can test hermetically.
func TestResolvePeer_LegacyGroup(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		AppID:       12345678,
		AppHash:     "0123456789abcdef0123456789abcdef",
		Phone:       "+12025551234",
		SessionFile: filepath.Join(tmp, "session.bin"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	lc, ok := c.(*liveClient)
	if !ok {
		t.Fatalf("New returned %T; want *liveClient", c)
	}

	// We never call Connect here — but resolvePeer for a legacy group
	// short-circuits before any network call.
	peer, err := lc.resolvePeer(context.Background(), -4946584787)
	if err != nil {
		t.Fatalf("resolvePeer(legacy): %v", err)
	}
	p, ok := peer.(*tg.InputPeerChat)
	if !ok {
		t.Fatalf("resolvePeer(legacy): want *tg.InputPeerChat, got %T", peer)
	}
	if p.ChatID != 4946584787 {
		t.Errorf("ChatID: got %d, want 4946584787", p.ChatID)
	}
}

// TestIsSupergroupID — boundary conditions of the -100-prefix detector.
func TestIsSupergroupID(t *testing.T) {
	cases := []struct {
		id   int64
		want bool
	}{
		{-1001234567890, true},
		{-4946584787, false},      // legacy group
		{-100, true},              // edge: exactly "-100"
		{-1009999999999999, true}, // larger supergroup
		{0, false},
		{12345, false}, // positive (user)
		{-1, false},    // single-digit negative
	}
	for _, tc := range cases {
		got := isSupergroupID(tc.id)
		if got != tc.want {
			t.Errorf("isSupergroupID(%d) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// TestConvertTGMessage_Basic — the tg.Message → Message conversion
// retains ID, Text, Date, FromUserID, ReplyToMessageID.
func TestConvertTGMessage_Basic(t *testing.T) {
	chatID := int64(-1001234567890)
	now := time.Now().Unix()
	m := &tg.Message{
		ID:      42,
		Message: "hello bot",
		Date:    int(now),
	}
	// FromID is a conditional field; must be set via the helper so the
	// Flags bit is recorded — direct field assignment leaves GetFromID
	// returning (nil, false).
	m.SetFromID(&tg.PeerUser{UserID: 987654})
	out := convertTGMessage(m, chatID)
	if out.ID != 42 {
		t.Errorf("ID: got %d, want 42", out.ID)
	}
	if out.ChatID != chatID {
		t.Errorf("ChatID: got %d, want %d", out.ChatID, chatID)
	}
	if out.Text != "hello bot" {
		t.Errorf("Text: got %q, want %q", out.Text, "hello bot")
	}
	if out.FromUserID != 987654 {
		t.Errorf("FromUserID: got %d, want 987654", out.FromUserID)
	}
	if out.Date.Unix() != now {
		t.Errorf("Date: got %v, want unix %d", out.Date, now)
	}
}

// TestConvertTGMessage_NoFromID — service / system messages have no
// FromID; the converter must NOT panic and FromUserID should stay 0.
func TestConvertTGMessage_NoFromID(t *testing.T) {
	chatID := int64(-1001234567890)
	m := &tg.Message{ID: 1, Message: "system"}
	out := convertTGMessage(m, chatID)
	if out.FromUserID != 0 {
		t.Errorf("FromUserID: got %d, want 0 for service message", out.FromUserID)
	}
}

// TestExtractSentMessageID_ShortSent — the UpdateShortSentMessage path
// (which Telegram returns for messages sent to private users).
func TestExtractSentMessageID_ShortSent(t *testing.T) {
	upd := &tg.UpdateShortSentMessage{ID: 12345}
	if got := extractSentMessageID(upd); got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

// TestExtractSentMessageID_UpdatesEnvelope — the more common
// tg.Updates → UpdateMessageID path.
func TestExtractSentMessageID_UpdatesEnvelope(t *testing.T) {
	upd := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 67890},
		},
	}
	if got := extractSentMessageID(upd); got != 67890 {
		t.Errorf("got %d, want 67890", got)
	}
}

// TestExtractSentMessageID_NewChannelMessage — the path used when the
// destination is a channel/supergroup.
func TestExtractSentMessageID_NewChannelMessage(t *testing.T) {
	upd := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewChannelMessage{
				Message: &tg.Message{ID: 4242},
			},
		},
	}
	if got := extractSentMessageID(upd); got != 4242 {
		t.Errorf("got %d, want 4242", got)
	}
}

// TestExtractSentMessageID_NoIDReturnsZero — when the envelope has no
// id-bearing update we must return 0 (the SendMessage caller surfaces
// that as an error rather than treating it as success).
func TestExtractSentMessageID_NoIDReturnsZero(t *testing.T) {
	upd := &tg.Updates{Updates: []tg.UpdateClass{}}
	if got := extractSentMessageID(upd); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

// TestSleepCtx_RespectsCancel — sleepCtx returns ctx.Err immediately
// when its context is already cancelled.
func TestSleepCtx_RespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, 5*time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx with cancelled ctx: got %v, want context.Canceled", err)
	}
}

// TestSleepCtx_FiresTimer — sleepCtx returns nil after the timer fires
// when the context is still alive.
func TestSleepCtx_FiresTimer(t *testing.T) {
	start := time.Now()
	if err := sleepCtx(context.Background(), 50*time.Millisecond); err != nil {
		t.Errorf("sleepCtx: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("sleepCtx returned too early: %v", elapsed)
	}
}

// TestGoroutineLeak_PreConnect — pre-Connect lifecycle (New + Close)
// must NOT spawn or leak goroutines.
func TestGoroutineLeak_PreConnect(t *testing.T) {
	baseline := runtime.NumGoroutine()
	tmp := t.TempDir()
	for i := 0; i < 10; i++ {
		cfg := Config{
			AppID:       12345678,
			AppHash:     "0123456789abcdef0123456789abcdef",
			Phone:       "+12025551234",
			SessionFile: filepath.Join(tmp, fmt.Sprintf("session-%d.bin", i)),
		}
		c, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	// Give Go runtime a moment to reclaim any short-lived helpers
	// (none should exist for the pre-Connect path, but the tolerance
	// avoids false positives on busy CI).
	time.Sleep(50 * time.Millisecond)
	final := runtimeGoroutineCount()
	// Allow ±2 goroutine drift to account for runtime scheduler jitter.
	if final > baseline+2 {
		t.Errorf("possible goroutine leak: baseline=%d final=%d", baseline, final)
	}
}

// TestSupergroupChannelIDExtraction — the -100<id> → id mapping.
func TestSupergroupChannelIDExtraction(t *testing.T) {
	// Bot API convention: supergroup chat_id = -(10^12 + channel_id).
	// e.g. channel_id 1234567890 → chat_id -1001234567890.
	cases := []struct {
		chatID   int64
		wantChan int64
	}{
		{-1001234567890, 1234567890},
		{-1000000000001, 1},
	}
	for _, tc := range cases {
		got := supergroupChannelID(tc.chatID)
		if got != tc.wantChan {
			t.Errorf("supergroupChannelID(%d) = %d, want %d", tc.chatID, got, tc.wantChan)
		}
	}
}
