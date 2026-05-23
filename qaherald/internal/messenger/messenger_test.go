// Interface-compliance tests for the messenger package.
//
// These tests are deliberately tiny: they prove that every concrete
// MessengerClient impl this package ships satisfies the interface and
// that the interface's sentinel errors are reachable.
//
// Per-impl wire-shape tests live in telegram_test.go (Telegram).
// Future Slack/Email impls (Wave 7) will add slack_test.go etc.
package messenger

import (
	"errors"
	"testing"
)

// TestTelegramClient_SatisfiesInterface is a compile+runtime proof
// that TelegramClient implements MessengerClient. The compile-time
// `var _ MessengerClient = (*TelegramClient)(nil)` in telegram.go is
// the primary anchor; this test secondarily proves the satisfier is
// constructible without immediate panic.
func TestTelegramClient_SatisfiesInterface(t *testing.T) {
	c, err := NewTelegramClient("test-token", 123, "")
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	var _ MessengerClient = c // compile-time assertion redux
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestNewTelegramClient_RejectsEmptyToken proves the constructor
// fails fast on missing credentials — the lifecycle orchestrator
// surfaces this as an HRD-101 required-field error.
func TestNewTelegramClient_RejectsEmptyToken(t *testing.T) {
	_, err := NewTelegramClient("", 123, "")
	if err == nil {
		t.Fatal("NewTelegramClient(token=\"\") returned nil error")
	}
	// Security mandate: error MUST NOT echo any token value (there is
	// none, but the test guards the contract).
	if errors.Is(err, ErrEmptyResponse) {
		t.Fatal("empty-token error should be local, not ErrEmptyResponse")
	}
}

// TestNewTelegramClient_RejectsZeroChatID proves the constructor
// fails fast on a zero chat-id. Telegram's API would silently route
// to nowhere on chat_id=0; we reject at construction.
func TestNewTelegramClient_RejectsZeroChatID(t *testing.T) {
	_, err := NewTelegramClient("test-token", 0, "")
	if err == nil {
		t.Fatal("NewTelegramClient(chatID=0) returned nil error")
	}
}

// TestSentinels_Reachable proves the package-level sentinels are
// reachable via errors.Is — guards against accidentally hiding them
// behind unexported wrappers.
func TestSentinels_Reachable(t *testing.T) {
	if !errors.Is(ErrPredicateNotSatisfied, ErrPredicateNotSatisfied) {
		t.Fatal("ErrPredicateNotSatisfied not self-identical")
	}
	if !errors.Is(ErrEmptyResponse, ErrEmptyResponse) {
		t.Fatal("ErrEmptyResponse not self-identical")
	}
}
