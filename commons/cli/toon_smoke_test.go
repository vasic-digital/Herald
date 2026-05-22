// Package cli — Wave 4b Task 1 smoke import.
//
// This file is the §107 anti-bluff probe for the digital.vasic.toon
// submodule vendoring: it references real exported symbols from
// digital.vasic.toon/pkg/toon so the build will FAIL if the submodule
// is missing, the replace directive is wrong, or the upstream API has
// drifted away from the names the Wave 4b design doc anticipates.
//
// A blank import (_ "digital.vasic.toon/pkg/toon") would compile but
// prove nothing about API availability — it is itself a §11.4 PASS-bluff
// and is forbidden by the Wave 4b Task 1 brief. The compile-time + runtime
// references below are the load-bearing positive evidence that the
// upstream package's public surface is reachable from commons.
//
// At the W4b-T1 pin (fc2ab55), the upstream is an honest sentinel-error
// scaffold: Marshal/Unmarshal/MarshalIndent/Encoder/Decoder/Compare all
// return ErrTOONEncodingNotImplemented. The helpers ContentType,
// IsTOONContentType, TokenEstimate ARE working. The smoke test below
// asserts the contract for BOTH groups so a future regression that
// silently re-introduces JSON-fallback (the round-27 bluff) FAILs here.
//
// Subsequent Wave 4b tasks:
//   - T2 replaces the sentinel-error scaffold with real toon-format/toon-go
//     delegation upstream; this smoke test must be updated then (the
//     Marshal/Unmarshal assertions will flip from "expect sentinel" to
//     "expect round-trip equality").
//   - T3 (commons/cli/contentnego.go) + T4 (commons/cli/toon.go) consume
//     the upstream package via the Gin middleware layer.

package cli

import (
	"errors"
	"testing"

	toon "digital.vasic.toon/pkg/toon"
)

// TestTOONSmokeImport asserts the digital.vasic.toon submodule is
// reachable through the commons/go.mod replace directive and that
// the upstream package exposes the public symbols the Wave 4b plan
// (§7.4 T1) anticipates.
//
// Symbols referenced here MUST stay in lock-step with the upstream API.
// If upstream renames any of these, this test FAILs at compile time
// (not at runtime) — which is the desired anti-bluff signal: vendored
// API drift surfaces immediately, not at the next request encode.
func TestTOONSmokeImport(t *testing.T) {
	// Reference 1: the canonical sentinel error. This is a package-level
	// var so a compile-time reference to it proves the upstream package
	// loaded and exported the symbol with the expected name.
	if !errors.Is(toon.ErrTOONEncodingNotImplemented, toon.ErrTOONEncodingNotImplemented) {
		t.Fatal("toon.ErrTOONEncodingNotImplemented failed errors.Is identity check")
	}
	if toon.ErrTOONEncodingNotImplemented.Error() == "" {
		t.Fatal("toon.ErrTOONEncodingNotImplemented has empty message — upstream changed sentinel shape")
	}

	// Reference 2: ContentType is the canonical MIME type constant. W4b-T3
	// (commons/cli/contentnego.go) keys on this string to map Codec → media
	// type. A compile-time reference proves the constant exists; a runtime
	// check proves the value hasn't drifted.
	if toon.ContentType != "application/toon" {
		t.Fatalf("toon.ContentType = %q, want %q — upstream drift", toon.ContentType, "application/toon")
	}

	// Reference 3: IsTOONContentType is the honest helper that W4b-T4
	// (commons/cli/toon.go) will use to decide between TOON and JSON decode
	// paths on inbound bodies. Spot-check the contract.
	if !toon.IsTOONContentType("application/toon") {
		t.Fatal("toon.IsTOONContentType(\"application/toon\") = false")
	}
	if !toon.IsTOONContentType("application/toon; charset=utf-8") {
		t.Fatal("toon.IsTOONContentType with charset param = false")
	}
	if toon.IsTOONContentType("application/json") {
		t.Fatal("toon.IsTOONContentType(\"application/json\") = true — upstream weakened")
	}

	// Reference 4: TokenEstimate is the honest heuristic helper (~4 chars
	// per token). Smoke-check that it returns a sensible non-negative int
	// for an empty input and scales linearly for a non-trivial input.
	if got := toon.TokenEstimate(nil); got != 0 {
		t.Errorf("toon.TokenEstimate(nil) = %d, want 0", got)
	}
	if got := toon.TokenEstimate(make([]byte, 100)); got != 25 {
		t.Errorf("toon.TokenEstimate(100 bytes) = %d, want 25 (~4 chars/token)", got)
	}

	// Reference 5: Marshal/Unmarshal honest sentinel.
	//
	// §107 anti-bluff: the round-27 audit caught a prior revision that
	// silently delegated Marshal to encoding/json while claiming TOON
	// encoding. The sentinel-error scaffold is the anti-regression
	// marker. As long as upstream stays at the sentinel pin, BOTH calls
	// MUST return ErrTOONEncodingNotImplemented. W4b-T2 will flip these
	// assertions when the real toon-format/toon-go wire-up lands.
	b, err := toon.Marshal(map[string]int{"x": 1})
	if !errors.Is(err, toon.ErrTOONEncodingNotImplemented) {
		t.Fatalf("toon.Marshal at sentinel pin returned err=%v, want ErrTOONEncodingNotImplemented (BLUFF REGRESSION if err is nil)", err)
	}
	if b != nil {
		t.Fatalf("toon.Marshal at sentinel pin returned bytes=%q, want nil (BLUFF REGRESSION)", b)
	}

	var dst map[string]int
	if err := toon.Unmarshal([]byte("ignored"), &dst); !errors.Is(err, toon.ErrTOONEncodingNotImplemented) {
		t.Fatalf("toon.Unmarshal at sentinel pin returned err=%v, want ErrTOONEncodingNotImplemented (BLUFF REGRESSION if err is nil)", err)
	}

	// Reference 6: Encoder + Decoder constructors return non-nil values
	// whose Encode/Decode methods honour the sentinel contract. This
	// proves the streaming API surface is wired (it would silently
	// disappear if upstream removed the types).
	enc := toon.NewEncoder(nil)
	if enc == nil {
		t.Fatal("toon.NewEncoder returned nil")
	}
	if err := enc.Encode(map[string]int{"x": 1}); !errors.Is(err, toon.ErrTOONEncodingNotImplemented) {
		t.Fatalf("Encoder.Encode at sentinel pin returned err=%v, want ErrTOONEncodingNotImplemented", err)
	}

	dec := toon.NewDecoder(nil)
	if dec == nil {
		t.Fatal("toon.NewDecoder returned nil")
	}
	if err := dec.Decode(&dst); !errors.Is(err, toon.ErrTOONEncodingNotImplemented) {
		t.Fatalf("Decoder.Decode at sentinel pin returned err=%v, want ErrTOONEncodingNotImplemented", err)
	}
}
