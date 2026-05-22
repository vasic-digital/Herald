// Package cli — Wave 4a Task 1 smoke import.
//
// This file is the §107 anti-bluff probe for the digital.vasic.http3
// submodule vendoring: it references a real exported symbol from
// digital.vasic.http3/pkg/server so the build will FAIL if the submodule
// is missing, the replace directive is wrong, or the upstream API has
// drifted away from the names the Wave 4 design doc anticipates.
//
// A blank import (_ "digital.vasic.http3/pkg/server") would compile but
// prove nothing about API availability — it is itself a §11.4 PASS-bluff
// and is forbidden by the Wave 4a Task 1 brief. The compile-time
// reference to server.ErrAlreadyStarted below is the load-bearing
// positive evidence that the upstream package's public surface is
// reachable from commons.
//
// Subsequent Wave 4a tasks (T3 `commons/cli/h3.go`) replace this file
// with the real HTTP/3 listener wrapper that consumes server.New +
// server.Server.Start + server.Server.Shutdown.

package cli

import (
	"errors"
	"testing"

	h3srv "digital.vasic.http3/pkg/server"
)

// TestHTTP3SmokeImport asserts the digital.vasic.http3 submodule is
// reachable through the commons/go.mod replace directive and that
// the upstream package exposes the public symbols the Wave 4a plan
// (§7.3 T3) anticipates.
//
// Symbols referenced here MUST stay in lock-step with the upstream API.
// If upstream renames any of these, this test FAILs at compile time
// (not at runtime) — which is the desired anti-bluff signal: vendored
// API drift surfaces immediately, not at the next listener start.
func TestHTTP3SmokeImport(t *testing.T) {
	// Reference 1: the canonical sentinel error. This is a package-level
	// var so a compile-time reference to it proves the upstream package
	// loaded and exported the symbol with the expected name.
	if !errors.Is(h3srv.ErrAlreadyStarted, h3srv.ErrAlreadyStarted) {
		t.Fatal("h3srv.ErrAlreadyStarted failed errors.Is identity check")
	}
	if h3srv.ErrAlreadyStarted.Error() == "" {
		t.Fatal("h3srv.ErrAlreadyStarted has empty message — upstream changed sentinel shape")
	}

	// Reference 2: Config is the input struct New consumes. A zero-value
	// Config must fail Validate (Addr / Handler / TLSConf all required).
	// This proves Config is a real struct with a Validate method, not a
	// type alias that quietly disappeared during upstream refactors.
	var zero h3srv.Config
	if err := zero.Validate(); err == nil {
		t.Fatal("zero h3srv.Config validated clean — upstream Validate weakened")
	}

	// Reference 3: New is the constructor we'll use from commons/cli/h3.go
	// in Wave 4a Task 3. Calling it with a zero Config must propagate the
	// Validate error, proving New is wired to Validate.
	srv, err := h3srv.New(zero)
	if err == nil {
		t.Fatalf("h3srv.New(zero) returned (%v, nil) — upstream New skipped Validate", srv)
	}
}
