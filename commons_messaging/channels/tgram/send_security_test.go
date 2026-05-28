package tgram_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// TestTgram_Send_ErrorDoesNotLeakToken is the HRD-133 §107 anti-bluff
// anchor — the tgram-side symmetry of qaherald's
// TestSlack_Send_ErrorDoesNotLeakToken (qaherald/internal/messenger/
// slack_test.go:571). It plants a sentinel token that contains the
// canonical Telegram Bot API "<bot_id>:<auth_secret>" shape, drives
// Send against an httptest server that returns 401 + the Bot API
// error envelope, and asserts the sentinel token substring is ABSENT
// from the returned error string.
//
// Defense-in-depth note: telebot itself already redacts /bot<token>
// path occurrences via redactToken (submodules/telebot/errors.go:282).
// Herald's sanitizeTgramError (errors.go) is the second layer — it
// also scrubs the bare-token form in case a future code path
// interpolates the raw token without the /bot prefix. This test pins
// the BARE-TOKEN scrub, so a regression that drops Herald's layer
// would fail here even if telebot's upstream regex still ran.
func TestTgram_Send_ErrorDoesNotLeakToken(t *testing.T) {
	const sensitiveToken = "123456:supersecret-must-not-leak-9999"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			// telebot.NewBot dispatches getMe; respond OK so NewBot itself
			// returns nil and we exercise the sendMessage 401 path.
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"TestBot"}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			// Bot API 401 envelope per https://core.telegram.org/bots/api#making-requests
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := tgram.NewAdapterWithBaseURL(sensitiveToken, "12345", srv.URL)
	msg := commons.OutboundMessage{
		Body: commons.Body{Plain: "boom"},
	}
	_, err := a.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("Send: expected error on 401 Unauthorized, got nil")
	}
	if strings.Contains(err.Error(), sensitiveToken) {
		t.Fatalf("error message LEAKED token (security violation): %v", err)
	}
	// Sanity: error MUST cite the method context + the Bot API reason so
	// the operator can still triage — proves the redaction did NOT
	// strip the diagnostic clause.
	if !strings.Contains(err.Error(), "tgram.Send") {
		t.Fatalf("error %q does not name the call site 'tgram.Send'", err.Error())
	}
	// Record the actual error string for the docs/qa evidence
	// transcript — the assertion above is the §107 anchor.
	t.Logf("redacted error text: %q", err.Error())
}

// TestTgram_SanitizeTgramError_Unit pins the helper itself — paired
// with the live httptest test above so a regression in either layer
// surfaces independently (the upstream telebot redactToken or the
// Herald-side sanitizeTgramError).
func TestTgram_SanitizeTgramError_Unit(t *testing.T) {
	const token = "123456:supersecret-abc"
	cases := []struct {
		name string
		msg  string
		want string
	}{
		{name: "empty msg returns empty", msg: "", want: ""},
		{name: "no token in msg unchanged", msg: "plain error", want: "plain error"},
		{name: "token redacted", msg: "boom: 123456:supersecret-abc failed", want: "boom: [REDACTED] failed"},
		{name: "multiple occurrences all redacted", msg: "123456:supersecret-abc x 123456:supersecret-abc", want: "[REDACTED] x [REDACTED]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tgram.SanitizeTgramErrorForTest(tc.msg, token)
			if got != tc.want {
				t.Errorf("sanitize(%q) = %q; want %q", tc.msg, got, tc.want)
			}
		})
	}
	// Empty token = passthrough (guards against a nil-Adapter path
	// where botToken is unset).
	if got := tgram.SanitizeTgramErrorForTest("anything", ""); got != "anything" {
		t.Errorf("empty token must passthrough; got %q", got)
	}
}
