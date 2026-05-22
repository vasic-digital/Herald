// Package cli — Wave 4b Task 2 smoke probe (POST upstream wire-up).
//
// This file is the §107 anti-bluff probe for the digital.vasic.toon
// submodule vendoring. At the Wave 4b T1 pin (fc2ab55) it asserted
// the upstream was an HONEST SENTINEL-ERROR SCAFFOLD: every
// Marshal/Unmarshal/Encoder/Decoder/Compare call returned
// ErrTOONEncodingNotImplemented. That contract is no longer correct.
//
// At the Wave 4b T2 pin (9ae61db), upstream wired
// github.com/toon-format/toon-go in place of the sentinel. The
// assertions below were FLIPPED accordingly:
//
//   - BEFORE T2: Marshal MUST return ErrTOONEncodingNotImplemented
//   - AFTER  T2: Marshal MUST return nil error AND bytes that do NOT
//                start with '{' or '[' (i.e. NOT JSON)
//
// This file is the seam where Herald enforces the upstream contract.
// If a future regression silently re-routes upstream Marshal back to
// encoding/json (the original 2026-05-17 bluff), THIS TEST FAILS
// IMMEDIATELY at the Herald build, before any commons/cli middleware
// or pherald-side wire-up gets a chance to observe JSON pretending
// to be TOON.
//
// §107 invariants asserted here (mirrored from upstream's own test
// suite but exercised across the submodule boundary so Herald gets
// independent confirmation):
//
//  1. The upstream package's public symbol surface is reachable
//     (compile-time references — would fail to build if upstream
//     renamed/removed Marshal/Unmarshal/Encoder/Decoder/etc.).
//
//  2. Marshal returns NON-NIL bytes AND nil error on a sample
//     struct — the sentinel-stub regression would surface here.
//
//  3. The bytes do NOT start with '{' or '[' — the JSON-fallback
//     regression would surface here.
//
//  4. Round-trip equality on a scalar struct — "no error" alone is
//     not sufficient; we deep-compare the decoded value to the
//     original.
//
// Subsequent Wave 4b tasks (T3..T7) consume this same upstream API
// through the commons/cli Gin middleware layer. Their tests rely on
// the contract this file pins.

package cli

import (
	"bytes"
	"reflect"
	"testing"

	toon "digital.vasic.toon/pkg/toon"
)

// smokeReceipt is a small struct shaped like Herald's real Receipt
// type. Used as the canonical round-trip target for this smoke.
type smokeReceipt struct {
	EventID string   `toon:"event_id"`
	Subject string   `toon:"subject"`
	Tags    []string `toon:"tags"`
	Count   int      `toon:"count"`
	Ok      bool     `toon:"ok"`
}

// TestTOONSmokeImport asserts the digital.vasic.toon submodule is
// reachable through the commons/go.mod replace directive and that
// the upstream package exposes the public symbols the Wave 4b plan
// (§7.4) anticipates.
//
// Symbols referenced here MUST stay in lock-step with the upstream
// API. If upstream renames any of these, this test FAILs at compile
// time (not at runtime) — which is the desired anti-bluff signal:
// vendored API drift surfaces immediately, not at the next request
// encode.
//
// Wave 4b T2 ANTI-BLUFF FLIP (2026-05-22): the assertions below now
// require REAL TOON encoding (non-nil bytes, non-JSON syntax, round-
// trip equality) rather than the sentinel-error contract the prior
// (fc2ab55) revision enforced. The flip is the structural marker
// that proves upstream is no longer the sentinel scaffold.
func TestTOONSmokeImport(t *testing.T) {
	// Reference 1: ContentType is the canonical MIME-type constant.
	// W4b-T3 (commons/cli/contentnego.go) keys on this string to
	// map Codec → media type. Runtime check pins the value.
	if toon.ContentType != "application/toon" {
		t.Fatalf("toon.ContentType = %q, want %q — upstream drift", toon.ContentType, "application/toon")
	}

	// Reference 2: IsTOONContentType is the honest helper that
	// W4b-T4 (commons/cli/toon.go) uses to decide between TOON and
	// JSON decode paths on inbound bodies. Spot-check the contract.
	if !toon.IsTOONContentType("application/toon") {
		t.Fatal("toon.IsTOONContentType(\"application/toon\") = false")
	}
	if !toon.IsTOONContentType("application/toon; charset=utf-8") {
		t.Fatal("toon.IsTOONContentType with charset param = false")
	}
	if toon.IsTOONContentType("application/json") {
		t.Fatal("toon.IsTOONContentType(\"application/json\") = true — upstream weakened")
	}

	// Reference 3: TokenEstimate is the honest heuristic helper
	// (~4 chars per token). Smoke-check that it returns a sensible
	// non-negative int for an empty input and scales linearly for
	// a non-trivial input.
	if got := toon.TokenEstimate(nil); got != 0 {
		t.Errorf("toon.TokenEstimate(nil) = %d, want 0", got)
	}
	if got := toon.TokenEstimate(make([]byte, 100)); got != 25 {
		t.Errorf("toon.TokenEstimate(100 bytes) = %d, want 25 (~4 chars/token)", got)
	}

	// Reference 4: ErrTOONEncodingNotImplemented retained for ABI.
	// Symbol must still resolve, but it is NO LONGER RETURNED by
	// any real entry point after the W4b T2 wire-up. We assert
	// presence here (compile-time) and absence-on-real-paths in
	// the dedicated checks below.
	if toon.ErrTOONEncodingNotImplemented == nil {
		t.Fatal("toon.ErrTOONEncodingNotImplemented is nil — upstream removed the ABI-compat symbol")
	}

	// Reference 5 — §107 ANTI-BLUFF FLIP: Marshal must succeed AND
	// produce non-JSON bytes. The prior pin (fc2ab55) asserted the
	// opposite (Marshal returned the sentinel + nil bytes).
	r := smokeReceipt{
		EventID: "evt_smoke",
		Subject: "wave-4b-t2-flip",
		Tags:    []string{"prod", "smoke"},
		Count:   42,
		Ok:      true,
	}
	data, err := toon.Marshal(r)
	if err != nil {
		t.Fatalf("§107 BLUFF/REGRESSION — toon.Marshal returned error: %v (expected real TOON bytes after W4b T2 wire-up)", err)
	}
	if len(data) == 0 {
		t.Fatal("§107 BLUFF — toon.Marshal returned empty bytes")
	}
	if data[0] == '{' || data[0] == '[' {
		t.Fatalf("§107 BLUFF REGRESSION — toon.Marshal output starts with JSON syntax byte %q; first 80 bytes: %q",
			string(data[0]), string(data[:minLen(80, len(data))]))
	}

	// Reference 6: Round-trip equality. Encode → decode → deep-equal.
	// "No error" alone is not sufficient (the original 2026-05-17
	// bluff returned nil error too); deep-equal is the load-bearing
	// invariant.
	var back smokeReceipt
	if err := toon.Unmarshal(data, &back); err != nil {
		t.Fatalf("toon.Unmarshal failed on real TOON bytes: %v; wire = %q", err, string(data))
	}
	if !reflect.DeepEqual(r, back) {
		t.Fatalf("§107 round-trip failed:\n  orig = %+v\n  back = %+v\n  wire = %q",
			r, back, string(data))
	}

	// Reference 7: Encoder + Decoder constructors still wire to the
	// streaming Writer/Reader API. Exercise round-trip through the
	// buffer-backed Encoder/Decoder so the io.Writer/Reader adapter
	// in upstream is covered too.
	var buf bytes.Buffer
	enc := toon.NewEncoder(&buf)
	if enc == nil {
		t.Fatal("toon.NewEncoder(&buf) returned nil")
	}
	if err := enc.Encode(r); err != nil {
		t.Fatalf("Encoder.Encode failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("Encoder wrote zero bytes")
	}
	if b := buf.Bytes(); b[0] == '{' || b[0] == '[' {
		t.Fatalf("§107 BLUFF REGRESSION — Encoder output starts with JSON syntax byte %q", string(b[0]))
	}

	dec := toon.NewDecoder(&buf)
	if dec == nil {
		t.Fatal("toon.NewDecoder(&buf) returned nil")
	}
	var streamBack smokeReceipt
	if err := dec.Decode(&streamBack); err != nil {
		t.Fatalf("Decoder.Decode failed: %v", err)
	}
	if !reflect.DeepEqual(r, streamBack) {
		t.Fatalf("§107 streaming round-trip failed:\n  orig = %+v\n  back = %+v",
			r, streamBack)
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
