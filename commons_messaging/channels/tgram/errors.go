package tgram

import (
	"errors"
	"strings"

	telebot "gopkg.in/telebot.v3"
)

// sanitizeTgramError returns msg with the bot token redacted to
// "[REDACTED]". This is a belt-and-braces defence on top of telebot's
// own redactToken regex (submodules/telebot/errors.go:282) — a future
// Herald-side code path that accidentally interpolates a URL or message
// containing the raw token (e.g. via fmt.Errorf with the token spliced
// in) is scrubbed here. The token format is "<bot_id>:<auth_secret>"
// and the slack adapter's sanitizeError (qaherald/internal/messenger/
// slack.go:327) is the structural template.
//
// HRD-133 §107 anti-bluff anchor: TestTgram_Send_ErrorDoesNotLeakToken
// (send_security_test.go) plants a sentinel token, drives Send against
// an httptest server that 401s, and asserts the sentinel substring is
// absent from the returned error string.
func sanitizeTgramError(msg, token string) string {
	if msg == "" || token == "" {
		return msg
	}
	return strings.ReplaceAll(msg, token, "[REDACTED]")
}

// extractRetryAfter returns the retry_after seconds value and ok=true
// when err is (or wraps) a telebot.FloodError emitted on HTTP 429 with
// parameters.retry_after present. Otherwise returns (0, false).
//
// telebot.FloodError is a VALUE type (errors.go:17 — no pointer
// receiver on Error()); we therefore unwrap with errors.As against a
// FloodError zero value. submodules/telebot/bot_raw.go:329 is the only
// construction site; it embeds RetryAfter as the int seconds value
// from the API JSON `parameters.retry_after` field.
//
// HRD-134 anchor: this is the single point where the 429 → retry_after
// mapping is asserted. A switch to a different telebot version that
// renames RetryAfter or changes the error type would surface here as
// the only edit site needed.
func extractRetryAfter(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	var fe telebot.FloodError
	if errors.As(err, &fe) {
		if fe.RetryAfter > 0 {
			return fe.RetryAfter, true
		}
	}
	return 0, false
}
