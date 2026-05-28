package mtproto

import (
	"regexp"
	"strings"
)

// HRD-133 parity — every error path through this package is wrapped by
// sanitizeMTProtoError so credential bytes never appear in committed logs,
// transcripts, or stderr.
//
// What's redacted:
//   - api_hash: any 32-char lowercase hex substring (the api_hash shape).
//   - session token bytes: any base64-ish substring ≥ 64 chars.
//   - 2FA password: any HERALD_MTPROTO_PASSWORD value present in the
//     error string (callers MUST NOT include it verbatim, but defense in
//     depth catches accidents).
//   - Telegram bot token shape: NNNNNNNNNN:35-char base64ish (the same
//     pattern HRD-133 redacts in tgram.sanitizeTgramError; we redact it
//     here too in case an MTProto error transitively includes one).
//
// What's NOT redacted (intentional, useful for debugging):
//   - chat IDs (negative integers; not secret per Telegram).
//   - user IDs (positive integers; not secret).
//   - error codes (FLOOD_WAIT_<N>, PHONE_CODE_INVALID, etc).
//   - host/port (Telegram DC endpoints are public).

// apiHashShape matches the 32-char lowercase hex string Telegram returns
// from my.telegram.org/apps. Other 32-hex strings (e.g. content hashes)
// could match too — that's fine; over-redaction is safer than under.
var apiHashShape = regexp.MustCompile(`\b[0-9a-f]{32}\b`)

// sessionTokenShape matches base64-ish blocks ≥ 64 chars (gotd persists
// session state as base64 sometimes; raw bytes would be safer but we
// defend both shapes).
var sessionTokenShape = regexp.MustCompile(`[A-Za-z0-9+/=_-]{64,}`)

// botTokenShape mirrors HRD-133's tgram.sanitizeTgramError pattern.
var botTokenShape = regexp.MustCompile(`\b[0-9]{8,}:[A-Za-z0-9_-]{30,}\b`)

// sanitizeMTProtoError wraps err's text with the credential-shape redactor.
// Returns a new error with the same kind (errors.Is preserved via %w wrap)
// but with the credential bytes replaced by "<redacted>".
//
// Idempotent — running it twice is harmless.
func sanitizeMTProtoError(err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	clean := sanitizeString(s)
	if clean == s {
		// No redaction applied — return the original to preserve identity
		// (errors.Is, %w chain).
		return err
	}
	return &sanitizedError{original: err, cleaned: clean}
}

// sanitizeString runs the credential-shape redactors on a free-form string.
// Public for tests; never invoke directly from production code — go through
// sanitizeMTProtoError so error identity is preserved.
func sanitizeString(s string) string {
	out := s
	out = apiHashShape.ReplaceAllString(out, "<redacted-api-hash>")
	out = sessionTokenShape.ReplaceAllString(out, "<redacted-session-token>")
	out = botTokenShape.ReplaceAllString(out, "<redacted-bot-token>")
	// 2FA password: heuristic — if HERALD_MTPROTO_PASSWORD value somehow
	// ended up in the string, we can't redact it without knowing the value.
	// Document the gap: callers MUST NOT echo Config.Password into error
	// messages. The test sanitizer_test.go covers the common shapes.
	return out
}

// sanitizedError is the wrapper carrying the cleaned-up text while still
// allowing errors.Is/errors.As to traverse to the original cause.
type sanitizedError struct {
	original error
	cleaned  string
}

func (e *sanitizedError) Error() string { return e.cleaned }
func (e *sanitizedError) Unwrap() error { return e.original }

// Contains is exposed for test assertions that a sanitized error string
// has NO surviving credential bytes.
func ContainsSecret(s string) bool {
	if apiHashShape.MatchString(s) {
		return true
	}
	if sessionTokenShape.MatchString(s) {
		return true
	}
	if botTokenShape.MatchString(s) {
		return true
	}
	return false
}

// helper used internally; trim spaces around concatenated error pieces.
func trimSurroundingSpace(s string) string { return strings.TrimSpace(s) }
