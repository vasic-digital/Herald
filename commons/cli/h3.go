// Wave 4a Task 3 — HTTP/3 (QUIC) listener wrapper.
//
// startQUIC builds a *digital.vasic.http3/pkg/server.Server from the
// supplied cert + handler and starts it in the background. The caller
// owns the returned server and is responsible for shutting it down on
// SIGTERM via Server.Shutdown(ctx).
//
// Why a wrapper instead of importing the upstream package directly into
// serve.go? Two reasons:
//
//  1. serve.go's dual-listener orchestration (T6) stays readable — the
//     QUIC plumbing is one function, one return value.
//  2. The wrapper is unit-testable in isolation against a real quic-go
//     HTTP/3 client (h3_test.go) without dragging in the rest of the
//     serve.go setup.
//
// Per §107 / Sixth Law: a "QUIC started" PASS without observing a real
// handshake is a bluff. The h3_test.go TestStartQUIC_RealClientRoundTrip
// is the load-bearing positive evidence — it speaks real quic-go HTTP/3
// against a real listener and asserts the body byte-for-byte.

package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	h3srv "digital.vasic.http3/pkg/server"
)

// startQUIC binds a UDP listener at addr serving handler over HTTP/3
// using cert as the server certificate. The TLS config is built with
// MinVersion = TLS 1.3 (HTTP/3 requirement) and NextProtos = ["h3"]
// (ALPN for QUIC).
//
// Returns the running *server.Server so the caller can call
// Server.Shutdown(ctx) on SIGTERM. Start runs in a background goroutine
// so this function is non-blocking — the listener is bound (best
// effort, verified by a UDP probe with a short timeout) before return,
// so concurrent callers may dial immediately.
//
// Errors:
//
//   - nil cert            → returned before binding ("h3: cert is nil")
//   - empty cert.Certificate chain → returned before binding
//   - upstream server.New validation → wrapped, returned
//   - listener bind failure          → wrapped, returned (after a short wait)
//
// The ctx argument is reserved for future cancellation hooks; the
// upstream server's Start() loop runs in a goroutine and is shut down
// via Server.Shutdown(ctx), not by cancelling this ctx.
func startQUIC(ctx context.Context, addr string, handler http.Handler, cert *tls.Certificate) (*h3srv.Server, error) {
	if cert == nil {
		return nil, errors.New("h3: cert is nil — production deployments must supply --tls-cert; dev runs use commons_tls.LoadOrGenerate")
	}
	if len(cert.Certificate) == 0 {
		return nil, errors.New("h3: cert.Certificate chain is empty — refusing to start QUIC listener with an unusable certificate")
	}
	if handler == nil {
		return nil, errors.New("h3: handler is nil")
	}
	if addr == "" {
		return nil, errors.New("h3: addr is empty")
	}

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS13, // HTTP/3 requires TLS 1.3 on the wire.
		NextProtos:   []string{"h3"},   // ALPN identifier for HTTP/3 (RFC 9114).
	}

	srv, err := h3srv.New(h3srv.Config{
		Addr:    addr,
		Handler: handler,
		TLSConf: tlsConf, // upstream field is TLSConf, NOT TLSConfig
	})
	if err != nil {
		return nil, fmt.Errorf("h3: server.New: %w", err)
	}

	// Start blocks (calls http3.Server.ListenAndServe internally); run
	// it in a goroutine so we can return the handle to the caller.
	// The terminal error lands on srv.Done() and is the caller's
	// responsibility to drain (typically via the serve.go orchestration).
	go func() {
		_ = srv.Start()
	}()

	// Best-effort bind verification — give the listener up to 2 seconds
	// to come up. The QUIC stack binds asynchronously; without this,
	// a fast unit test can issue Get() before the UDP socket is open.
	if err := waitForUDPBind(ctx, addr, 2*time.Second); err != nil {
		// Tear down the half-started server so the caller doesn't leak.
		shutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil, fmt.Errorf("h3: listener did not come up on %s: %w", addr, err)
	}

	return srv, nil
}

// waitForUDPBind polls a UDP dial against addr until it succeeds or the
// timeout expires. UDP dial does not perform a handshake so this only
// proves the kernel will route packets to that address — it does NOT
// prove the QUIC stack is ready to handshake. The real handshake is
// the load-bearing assertion in TestStartQUIC_RealClientRoundTrip,
// which retries until success or the test's own deadline.
func waitForUDPBind(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		c, err := net.DialTimeout("udp", addr, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("timed out waiting for UDP listener")
}
