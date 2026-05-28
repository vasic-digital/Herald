package slack_test

import (
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// TestSlackNewFromURLValid pins the URL parser's contract — the canonical
// slack://<bot-token>@<workspace>/<channel-id> shape lands as a valid
// adapter with the right pieces. The optional app_token query param wires
// the Socket Mode credential.
func TestSlackNewFromURLValid(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"plain", "slack://xoxb-abc123@example/C0123ABC"},
		{"with-app-token", "slack://xoxb-abc123@example/C0123ABC?app_token=xapp-xyz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := slack.NewFromURL(tc.url)
			if err != nil {
				t.Fatalf("NewFromURL(%q) err=%v", tc.url, err)
			}
			if a == nil {
				t.Fatalf("NewFromURL(%q) nil adapter", tc.url)
			}
			if a.Name() != string(commons.ChannelSlack) {
				t.Fatalf("Name()=%q want %q", a.Name(), commons.ChannelSlack)
			}
		})
	}
}

// TestSlackNewFromURLRejects asserts every component required by the URL
// scheme is mandatory — bare scheme without token / token without channel
// id / wrong scheme all error explicitly (§107 anti-bluff: a silent default
// for missing creds would be the canonical bluff class).
func TestSlackNewFromURLRejects(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"wrong-scheme", "https://xoxb-abc@example/C1"},
		{"no-token", "slack://@example/C1"},
		{"no-channel", "slack://xoxb-abc@example/"},
		{"no-channel-no-path", "slack://xoxb-abc@example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := slack.NewFromURL(tc.url); err == nil {
				t.Fatalf("NewFromURL(%q) want error, got nil", tc.url)
			}
		})
	}
}

// TestSlackSatisfiesChannel — interface-satisfaction pin per the Wave 7
// plan T6: *slack.Adapter MUST satisfy channels.Channel. A regression here
// (e.g. an interface widening that the adapter misses) is caught at compile
// time by the explicit `var _ channels.Channel = ...` form.
func TestSlackSatisfiesChannel(t *testing.T) {
	var c channels.Channel = slack.NewWithBaseURL("xoxb-test", "xapp-test", "C1", "http://localhost")
	if c.Name() != string(commons.ChannelSlack) {
		t.Fatalf("Name()=%q want %q", c.Name(), commons.ChannelSlack)
	}
	caps := c.Capabilities()
	if !caps.Threads {
		t.Fatal("Slack Capabilities().Threads must be true (thread_ts)")
	}
	if !caps.Attachments {
		t.Fatal("Slack Capabilities().Attachments must be true")
	}
	if caps.DeliveryCeiling != commons.DeliveryRouted {
		t.Fatalf("DeliveryCeiling=%v want DeliveryRouted", caps.DeliveryCeiling)
	}
}

// TestSlackRegistryWiring proves the init() registers "slack" with the
// channels registry so pherald listen can resolve it by name (Wave 7 T6
// step 5 — the registration is the runtime contract).
func TestSlackRegistryWiring(t *testing.T) {
	c, err := channels.New(string(commons.ChannelSlack), channels.Config{
		Token:   "xoxb-test",
		Target:  "C1",
		BaseURL: "http://localhost",
	})
	if err != nil {
		t.Fatalf("channels.New(slack) err=%v", err)
	}
	if c == nil {
		t.Fatal("channels.New(slack) returned nil channel")
	}
	if c.Name() != string(commons.ChannelSlack) {
		t.Fatalf("Name()=%q want %q", c.Name(), commons.ChannelSlack)
	}
}
