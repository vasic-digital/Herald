// Builder tests for the channel-keyed MessengerClient constructor
// (Wave 7 T7 — HRD-116).
//
// Coverage map:
//
//	TestBuilderTgram          — Build("tgram", ...) → TelegramClient OK
//	TestBuilderSlack          — Build("slack", ...) → SlackClient OK
//	TestBuilderUnknownErrors  — unknown channel fails loud (§107 — no
//	                            silent nil)
package messenger_test

import (
	"strings"
	"testing"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

func TestBuilderTgram(t *testing.T) {
	c, err := messenger.Build(messenger.BuildConfig{
		Channel: "tgram",
		Token:   "123:abc",
		ChatID:  456,
	})
	if err != nil {
		t.Fatalf("Build(tgram): unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("Build(tgram): client is nil")
	}
	_ = c.Close()
}

func TestBuilderTgramDefault(t *testing.T) {
	// Empty Channel == "tgram" default.
	c, err := messenger.Build(messenger.BuildConfig{
		Token:  "123:abc",
		ChatID: 456,
	})
	if err != nil {
		t.Fatalf("Build(default→tgram): %v", err)
	}
	if c == nil {
		t.Fatal("Build(default→tgram): client is nil")
	}
	_ = c.Close()
}

func TestBuilderTgramPropagatesConstructorError(t *testing.T) {
	// NewTelegramClient errors on empty token. The builder MUST
	// surface that error (NOT swallow it) — a silently-nil client
	// would be a §107 bluff (preflight against nil client = silent
	// success).
	_, err := messenger.Build(messenger.BuildConfig{
		Channel: "tgram",
		Token:   "", // empty token → constructor error
		ChatID:  456,
	})
	if err == nil {
		t.Fatal("Build(tgram, empty-token): expected error from constructor, got nil")
	}
	if !strings.Contains(err.Error(), "Build(tgram)") {
		t.Fatalf("Build error %q does not wrap with builder context", err.Error())
	}
}

func TestBuilderSlack(t *testing.T) {
	c, err := messenger.Build(messenger.BuildConfig{
		Channel:   "slack",
		Token:     "xoxb-x",
		ChannelID: "C1",
		BaseURL:   "http://localhost",
	})
	if err != nil {
		t.Fatalf("Build(slack): unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("Build(slack): client is nil")
	}
	_ = c.Close()
}

func TestBuilderUnknownErrors(t *testing.T) {
	c, err := messenger.Build(messenger.BuildConfig{Channel: "nope"})
	if err == nil {
		t.Fatal("Build(unknown): expected error, got nil (§107: silent nil is a bluff)")
	}
	if c != nil {
		t.Fatalf("Build(unknown): expected nil client on error, got %v", c)
	}
	// The error MUST name the offending channel + cite supported set
	// so config-time mistakes are diagnosable.
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Build error %q does not name the unknown channel", err.Error())
	}
	if !strings.Contains(err.Error(), "supported") {
		t.Fatalf("Build error %q does not list supported channels", err.Error())
	}
}
