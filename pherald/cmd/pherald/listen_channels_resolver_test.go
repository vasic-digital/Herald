// Wave 7 HRD-114b — hermetic unit tests for the `--channels` override (GAP G4)
// and the §11.4.104 participant-resolver wiring on the `pherald listen` path
// (GAP G1/G2).
//
// §107 anchor — every assertion is a positive runtime observation against the
// REAL production seams (resolveChannelListWithOverride, loadListenConfigFromEnv,
// buildResolver), never a metadata-only / "no error" PASS:
//
//	(a) --channels slack,tgram OVERRIDES HERALD_CHANNELS=tgram (the resolved
//	    subscriber set is EXACTLY {slack,tgram}, proving the env value lost).
//	(b) an empty --channels falls back to HERALD_CHANNELS (the env value wins).
//	(c) the production config built by loadListenConfigFromEnv carries a
//	    NON-NIL Resolver, and that resolver's operator handle for slack equals
//	    HERALD_SLACK_OPERATOR_USERNAME (proving the resolver is wired AND
//	    consumes the per-channel operator env var).
//
// All hermetic: no network, no `claude` spawn (claude_code.New does only local
// validation), no Telegram/Slack round-trip (loadListenConfigFromEnv builds the
// adapters from env-supplied fake tokens and never calls Subscribe).
package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/vasic-digital/herald/commons"
)

// TestChannelsFlagOverridesEnv proves GAP G4: a non-empty --channels value
// overrides HERALD_CHANNELS, and an empty value falls back to the env var.
func TestChannelsFlagOverridesEnv(t *testing.T) {
	t.Setenv("HERALD_CHANNELS", "tgram")

	// (a) override wins.
	got := resolveChannelListWithOverride("slack,tgram")
	want := []string{"slack", "tgram"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("override: resolveChannelListWithOverride(%q) with env=tgram = %v, want %v", "slack,tgram", got, want)
	}

	// (a') override of a single channel still wins (env had tgram only).
	if got := resolveChannelListWithOverride("slack"); !reflect.DeepEqual(got, []string{"slack"}) {
		t.Fatalf("override single: got %v, want [slack]", got)
	}

	// (b) empty / whitespace override falls back to the env var.
	if got := resolveChannelListWithOverride(""); !reflect.DeepEqual(got, []string{"tgram"}) {
		t.Fatalf("fallback empty: got %v, want [tgram] (HERALD_CHANNELS)", got)
	}
	if got := resolveChannelListWithOverride("   "); !reflect.DeepEqual(got, []string{"tgram"}) {
		t.Fatalf("fallback whitespace: got %v, want [tgram] (HERALD_CHANNELS)", got)
	}

	// (b') with no env AND no override, default ["tgram"].
	t.Setenv("HERALD_CHANNELS", "")
	if got := resolveChannelListWithOverride(""); !reflect.DeepEqual(got, []string{"tgram"}) {
		t.Fatalf("default: got %v, want [tgram]", got)
	}
}

// TestLoadListenConfigWiresResolverAndChannels exercises the FULL production
// seam loadListenConfigFromEnv: the --channels override drives the subscriber
// set AND a non-nil Resolver is built that consumes the per-channel operator
// env var. This is GAP G1/G2 + GAP G4 proven against the real config loader.
func TestLoadListenConfigWiresResolverAndChannels(t *testing.T) {
	// Fake creds so both adapters construct without a wire round-trip.
	t.Setenv("HERALD_TGRAM_BOT_TOKEN", "fake-tgram-token")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "12345")
	t.Setenv("HERALD_SLACK_BOT_TOKEN", "xoxb-fake")
	t.Setenv("HERALD_SLACK_APP_TOKEN", "xapp-fake")
	t.Setenv("HERALD_SLACK_CHANNEL_ID", "C0FAKE")
	// Operator usernames per channel — the resolver MUST consume these.
	t.Setenv("HERALD_TGRAM_OPERATOR_USERNAME", "@milos85vasic")
	t.Setenv("HERALD_SLACK_OPERATOR_USERNAME", "@milos.slack")
	// Env says tgram-only; the --channels override below must win.
	t.Setenv("HERALD_CHANNELS", "tgram")

	cfg, err := loadListenConfigFromEnv("slack,tgram")
	if err != nil {
		t.Fatalf("loadListenConfigFromEnv: %v", err)
	}

	// GAP G4: subscriber set reflects the override, not the env.
	gotChannels := make([]string, 0, len(cfg.Subscribers))
	for name := range cfg.Subscribers {
		gotChannels = append(gotChannels, name)
	}
	sort.Strings(gotChannels)
	if !reflect.DeepEqual(gotChannels, []string{"slack", "tgram"}) {
		t.Fatalf("override channels: subscribers = %v, want [slack tgram] (HERALD_CHANNELS=tgram must lose)", gotChannels)
	}

	// GAP G1/G2: a real Resolver is wired.
	if cfg.Resolver == nil {
		t.Fatal("cfg.Resolver is nil — attribution + clarify-tagging would be silently skipped for ALL channels (§11.4.104 regression)")
	}

	// (c) the operator handle is the PRIMARY-channel (Telegram) value.
	if got := cfg.Resolver.OperatorHandle(); got != "@milos85vasic" {
		t.Fatalf("OperatorHandle() = %q, want @milos85vasic (HERALD_TGRAM_OPERATOR_USERNAME, primary channel)", got)
	}

	// (c) the operator's SLACK @username resolves from HERALD_SLACK_OPERATOR_USERNAME.
	uname, ok := cfg.Resolver.UsernameFor("@milos85vasic", "slack")
	if !ok {
		t.Fatal("UsernameFor(operator, slack) not found — HERALD_SLACK_OPERATOR_USERNAME was not consumed by the resolver")
	}
	if uname != "@milos.slack" {
		t.Fatalf("UsernameFor(operator, slack) = %q, want @milos.slack (HERALD_SLACK_OPERATOR_USERNAME)", uname)
	}
}

// TestBuildResolverSlackOnly proves the resolver consumes the slack operator
// env var even when slack is the ONLY enabled channel (primaryChannel falls
// through to the first enabled channel when Telegram is absent).
func TestBuildResolverSlackOnly(t *testing.T) {
	t.Setenv("HERALD_SLACK_OPERATOR_USERNAME", "@slackboss")
	// Ensure no tgram operator leaks in from the ambient environment.
	t.Setenv("HERALD_TGRAM_OPERATOR_USERNAME", "")

	r := buildResolver([]string{"slack"})
	if r == nil {
		t.Fatal("buildResolver returned nil")
	}
	if got := r.OperatorHandle(); got != "@slackboss" {
		t.Fatalf("OperatorHandle() = %q, want @slackboss (HERALD_SLACK_OPERATOR_USERNAME as primary)", got)
	}
	if uname, ok := r.UsernameFor("@slackboss", "slack"); !ok || uname != "@slackboss" {
		t.Fatalf("UsernameFor(@slackboss, slack) = (%q,%v), want (@slackboss,true)", uname, ok)
	}
}

// TestBuildResolverUnknownSenderFallback proves the env-only resolver still
// attributes a first-contact (non-operator) sender by their raw @username —
// the §2 unknown-sender behaviour that makes returning a non-nil resolver
// unconditionally safe.
func TestBuildResolverUnknownSenderFallback(t *testing.T) {
	t.Setenv("HERALD_TGRAM_OPERATOR_USERNAME", "@op")
	r := buildResolver([]string{"tgram"})
	got := r.ResolveSender(string(commons.ChannelTelegram), "99999", "@randomuser")
	if got != "@randomuser" {
		t.Fatalf("ResolveSender(unknown) = %q, want @randomuser (raw-username fallback)", got)
	}
}
