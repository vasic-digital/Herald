// Wave 4b Task 3 — unit tests for contentnego.go.
//
// 11 sub-tests (the W4b plan target — see docs/superpowers/plans/
// 2026-05-22-wave4b-toon.md Task 3 §Tests) plus the env-default JSON
// case that exercises HERALD_DEFAULT_RESPONSE_CODEC=json. Each test
// targets one rung on the policy ladder documented at the top of
// contentnego.go.
//
// §107 anti-bluff anchors enforced here:
//
//   - TestMarshalChosen_TOON asserts the returned bytes do NOT begin with
//     '{' or '[' — the original 2026-05-17 PASS-bluff revision returned
//     JSON bytes from toon.Marshal; that regression would surface here.
//   - TestUnmarshalChosen_TOON_RoundTrip deep-compares the decoded value
//     to the original — "no error" alone is insufficient evidence per
//     Universal §11.4.
//   - TestNegotiateAccept_QValue asserts the higher-q codec wins even
//     when it is the non-Herald-default (i.e., JSON@q=1.0 beats
//     TOON@q=0.5); q-parameter respect is the precise RFC 7231 §5.3.1
//     contract callers depend on.
//   - TestUnmarshalChosen_UnknownContentType_ErrorsExplicitly asserts the
//     error message names the unsupported CT — the worst PASS-bluff at
//     this layer would be a silent JSON-fallback that misroutes the
//     body's interpretation without any observability.
package cli

import (
	"reflect"
	"strings"
	"testing"
)

// sample is the round-trip target. Tag-mirror the toon-format/toon-go
// convention (`toon:"name"`) and the encoding/json convention so both
// codecs see consistent field names.
type sample struct {
	EventID string   `json:"event_id" toon:"event_id"`
	Subject string   `json:"subject"  toon:"subject"`
	Tags    []string `json:"tags"     toon:"tags"`
	Count   int      `json:"count"    toon:"count"`
	Ok      bool     `json:"ok"       toon:"ok"`
}

func sampleFixture() sample {
	return sample{
		EventID: "evt_w4b_t3",
		Subject: "contentnego_test",
		Tags:    []string{"unit", "anti-bluff"},
		Count:   3,
		Ok:      true,
	}
}

// -------- Accept-header negotiation --------

// TestNegotiateAccept_ExplicitTOON — Accept: application/toon ⇒ TOON.
func TestNegotiateAccept_ExplicitTOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "") // ensure no env carry-over
	got, _ := NegotiateContentType("application/toon", "")
	if got != MediaTypeTOON {
		t.Errorf("Accept=application/toon → responseCT=%q, want %q", got, MediaTypeTOON)
	}
}

// TestNegotiateAccept_ExplicitJSON — Accept: application/json ⇒ JSON.
func TestNegotiateAccept_ExplicitJSON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	got, _ := NegotiateContentType("application/json", "")
	if got != MediaTypeJSON {
		t.Errorf("Accept=application/json → responseCT=%q, want %q", got, MediaTypeJSON)
	}
}

// TestNegotiateAccept_StarSlashStar_DefaultsTOON — Accept: */* ⇒ TOON
// (Herald default per W4b operator decision 1).
func TestNegotiateAccept_StarSlashStar_DefaultsTOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "") // default = toon
	got, _ := NegotiateContentType("*/*", "")
	if got != MediaTypeTOON {
		t.Errorf("Accept=*/* (default env) → responseCT=%q, want %q (Herald default)", got, MediaTypeTOON)
	}
}

// TestNegotiateAccept_Empty_DefaultsTOON — empty Accept ⇒ TOON default.
func TestNegotiateAccept_Empty_DefaultsTOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	got, _ := NegotiateContentType("", "")
	if got != MediaTypeTOON {
		t.Errorf("Accept=\"\" (default env) → responseCT=%q, want %q (Herald default)", got, MediaTypeTOON)
	}
}

// TestNegotiateAccept_QValue — explicit assertion that higher-q JSON
// wins over lower-q TOON (the RFC 7231 §5.3.1 contract).
//
// §107 anti-bluff: this test would FAIL if a naive implementation
// short-circuited on "first TOON entry seen → return TOON" without
// honouring q-values. The W4b policy explicitly defers to q when both
// codecs are present with quality parameters.
func TestNegotiateAccept_QValue(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	got, _ := NegotiateContentType("application/toon;q=0.5, application/json;q=1.0", "")
	if got != MediaTypeJSON {
		t.Errorf("Accept=toon;q=0.5,json;q=1.0 → responseCT=%q, want %q (higher q wins)", got, MediaTypeJSON)
	}
	// Symmetric — TOON@q=0.9 beats JSON@q=0.3 (also exercises the path
	// where q-bearing TOON still wins, not just the no-q tie-break).
	got2, _ := NegotiateContentType("application/toon;q=0.9, application/json;q=0.3", "")
	if got2 != MediaTypeTOON {
		t.Errorf("Accept=toon;q=0.9,json;q=0.3 → responseCT=%q, want %q (higher q wins)", got2, MediaTypeTOON)
	}
}

// TestNegotiateAccept_EnvDefault_JSON — HERALD_DEFAULT_RESPONSE_CODEC=json
// flips the `*/*` and empty-Accept fallback to JSON (W4b operator
// decision 3 opt-out).
func TestNegotiateAccept_EnvDefault_JSON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "json")
	// `*/*` rung.
	if got, _ := NegotiateContentType("*/*", ""); got != MediaTypeJSON {
		t.Errorf("Accept=*/* with env=json → responseCT=%q, want %q", got, MediaTypeJSON)
	}
	// Empty-Accept rung.
	if got, _ := NegotiateContentType("", ""); got != MediaTypeJSON {
		t.Errorf("Accept=\"\" with env=json → responseCT=%q, want %q", got, MediaTypeJSON)
	}
	// Explicit Accept must still win over the env default — env only
	// resolves the `*/*` / empty rung.
	if got, _ := NegotiateContentType("application/toon", ""); got != MediaTypeTOON {
		t.Errorf("Accept=application/toon with env=json → responseCT=%q, want %q (explicit Accept wins)",
			got, MediaTypeTOON)
	}
}

// -------- Content-Type (request body) negotiation --------

// TestNegotiateRequest_ExplicitTOON — Content-Type: application/toon
// ⇒ requestCT=TOON.
func TestNegotiateRequest_ExplicitTOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	_, got := NegotiateContentType("", "application/toon")
	if got != MediaTypeTOON {
		t.Errorf("Content-Type=application/toon → requestCT=%q, want %q", got, MediaTypeTOON)
	}
	// charset parameter MUST be tolerated.
	_, got2 := NegotiateContentType("", "application/toon; charset=utf-8")
	if got2 != MediaTypeTOON {
		t.Errorf("Content-Type=application/toon;charset=utf-8 → requestCT=%q, want %q (param tolerated)",
			got2, MediaTypeTOON)
	}
}

// TestNegotiateRequest_EmptyContentType_DefaultsJSON — no CT ⇒ JSON.
// Backwards-compat with curl -d '...' usage (W4b operator decision 2).
func TestNegotiateRequest_EmptyContentType_DefaultsJSON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	_, got := NegotiateContentType("", "")
	if got != MediaTypeJSON {
		t.Errorf("Content-Type=\"\" → requestCT=%q, want %q (default)", got, MediaTypeJSON)
	}
}

// -------- Marshal / Unmarshal dispatch --------

// TestMarshalChosen_TOON — bytes MUST NOT start with '{' or '[' (the
// original 2026-05-17 PASS-bluff signature).
//
// §107 anti-bluff: this is the load-bearing assertion. The 2026-05-17
// revision of digital.vasic.toon silently delegated to encoding/json
// while claiming TOON encoding; that bluff would produce a byte stream
// here beginning with '{'. Wave 4b T2 wired the real
// github.com/toon-format/toon-go upstream; this test pins that wire-up
// at the Herald-consumer boundary.
func TestMarshalChosen_TOON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	out, err := MarshalChosen(sampleFixture(), MediaTypeTOON)
	if err != nil {
		t.Fatalf("MarshalChosen(TOON) error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("MarshalChosen(TOON) returned empty bytes (§107 bluff: prior sentinel scaffold returned nil)")
	}
	if out[0] == '{' || out[0] == '[' {
		t.Fatalf("§107 BLUFF REGRESSION — MarshalChosen(TOON) returned JSON-looking bytes (first byte=%q); first 80 bytes: %q",
			string(out[0]), string(out[:min(80, len(out))]))
	}
}

// TestMarshalChosen_JSON — bytes MUST start with '{' (encoding/json
// always emits an opening brace for a struct value).
func TestMarshalChosen_JSON(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	out, err := MarshalChosen(sampleFixture(), MediaTypeJSON)
	if err != nil {
		t.Fatalf("MarshalChosen(JSON) error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("MarshalChosen(JSON) returned empty bytes")
	}
	if out[0] != '{' {
		t.Fatalf("MarshalChosen(JSON) first byte = %q, want '{' (encoding/json struct output)", string(out[0]))
	}
}

// TestUnmarshalChosen_TOON_RoundTrip — encode → decode → reflect.DeepEqual.
// "No error" alone is NOT sufficient evidence per Universal §11.4; the
// load-bearing invariant is byte-level round-trip equality of the
// in-memory value.
func TestUnmarshalChosen_TOON_RoundTrip(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	original := sampleFixture()
	encoded, err := MarshalChosen(original, MediaTypeTOON)
	if err != nil {
		t.Fatalf("MarshalChosen(TOON) error: %v", err)
	}
	// Sanity: still not JSON-looking after marshal (defence-in-depth
	// against a future regression in MarshalChosen itself).
	if encoded[0] == '{' || encoded[0] == '[' {
		t.Fatalf("§107 BLUFF — encoded TOON bytes start with JSON syntax byte %q; first 80 bytes: %q",
			string(encoded[0]), string(encoded[:min(80, len(encoded))]))
	}
	var back sample
	if err := UnmarshalChosen(encoded, &back, MediaTypeTOON); err != nil {
		t.Fatalf("UnmarshalChosen(TOON) error: %v; wire=%q", err, string(encoded))
	}
	if !reflect.DeepEqual(original, back) {
		t.Fatalf("§107 round-trip failed:\n  orig=%+v\n  back=%+v\n  wire=%q", original, back, string(encoded))
	}
}

// TestUnmarshalChosen_UnknownContentType_ErrorsExplicitly — application/unknown
// returns an explicit error naming the unsupported CT.
//
// §107 anti-bluff: a silent JSON-fallback here would mask a misrouted
// body. The error MUST surface the bad CT so the caller (Gin handler →
// 400 response) can include it in the operator-visible message.
func TestUnmarshalChosen_UnknownContentType_ErrorsExplicitly(t *testing.T) {
	t.Setenv(EnvDefaultResponseCodec, "")
	var dst sample
	err := UnmarshalChosen([]byte("anything"), &dst, "application/unknown")
	if err == nil {
		t.Fatal("UnmarshalChosen(application/unknown) returned nil error — should reject explicitly")
	}
	msg := err.Error()
	if !strings.Contains(msg, "application/unknown") {
		t.Errorf("error %q does not name the unsupported CT 'application/unknown'", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "unsupported") &&
		!strings.Contains(strings.ToLower(msg), "unknown") {
		t.Errorf("error %q does not classify the failure (expected 'unsupported' or 'unknown' in message)", msg)
	}
}

// min returns the smaller of a, b. Local helper to avoid a Go 1.21+
// builtin dependency without affecting commons' declared go directive.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
