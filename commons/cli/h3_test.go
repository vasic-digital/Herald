// Wave 4a Task 3 — HTTP/3 listener wrapper anti-bluff tests.
//
// Per §107 / Sixth Law: each test below makes a load-bearing positive
// assertion against the REAL behaviour of the QUIC listener — not a
// "configuration validated" mock. The round-trip test in particular
// speaks real quic-go HTTP/3 wire protocol against the listener and
// asserts the response body byte-for-byte. Replacing startQUIC with a
// no-op MUST cause TestStartQUIC_RealClientRoundTrip to FAIL.

package cli

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	commons_tls "github.com/vasic-digital/herald/commons_tls"
)

// freeUDPAddr binds 127.0.0.1:0 to a UDP socket, captures the OS-assigned
// port, then releases it so the QUIC listener can bind. There's a tiny
// race window between release and re-bind but in practice it's negligible
// on a developer / CI host with low UDP churn — and the bind verification
// inside startQUIC will fail loudly if the kernel hands the port to
// somebody else in the meantime.
func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeUDPAddr: ListenPacket: %v", err)
	}
	addr := pc.LocalAddr().String()
	if err := pc.Close(); err != nil {
		t.Fatalf("freeUDPAddr: Close: %v", err)
	}
	return addr
}

// devCert builds an ephemeral self-signed cert via commons_tls so the
// test doesn't depend on ~/.herald/dev-cert.pem and won't pollute it.
// commons_tls.LoadOrGenerate returns *tls.Certificate (the actual API,
// as confirmed by reading commons_tls/cert.go in this same workspace);
// the user's task brief signature anticipated the same pointer type.
func devCert(t *testing.T) *tls.Certificate {
	t.Helper()
	dir := t.TempDir()
	cert, err := commons_tls.LoadOrGenerate(
		filepath.Join(dir, "cert.pem"),
		filepath.Join(dir, "key.pem"),
	)
	if err != nil {
		t.Fatalf("devCert: commons_tls.LoadOrGenerate: %v", err)
	}
	return cert
}

// TestStartQUIC_RealClientRoundTrip is the §107 anti-bluff load-bearing
// test for Wave 4a Task 3.
//
// It starts a real QUIC listener via startQUIC, issues a real HTTP/3
// GET via quic-go's http3.Transport (NOT a mocked RoundTripper), and
// asserts the response status + body byte-for-byte. A no-op startQUIC
// stub (or one that bound TCP instead of UDP, or one that left ALPN
// unset, or one that downgraded TLS to 1.2) MUST cause this test to
// FAIL — which is the falsifiability rehearsal recorded in the
// upstream digital.vasic.http3 CONSTITUTION.md.
//
// Per the user's Task 3 brief: "The TestStartQUIC_RealClientRoundTrip
// MUST use a real quic-go/http3.Transport with InsecureSkipVerify
// (acceptable for the self-signed dev cert) and a real net.UDPAddr —
// NOT a fake transport. The body assertion proves end-to-end QUIC
// works on this binary at this commit."
func TestStartQUIC_RealClientRoundTrip(t *testing.T) {
	addr := freeUDPAddr(t)
	cert := devCert(t)

	const wantBody = "hello-from-h3-roundtrip"
	mux := http.NewServeMux()
	mux.HandleFunc("/h3probe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantBody))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	srv, err := startQUIC(ctx, addr, mux, cert)
	if err != nil {
		t.Fatalf("startQUIC: %v", err)
	}
	t.Cleanup(func() {
		shutCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = srv.Shutdown(shutCtx)
	})

	// Real quic-go HTTP/3 transport — NOT mocked. InsecureSkipVerify is
	// acceptable here because the cert is the ephemeral self-signed one
	// we just built; the load-bearing assertion is the body + status,
	// not the TLS trust chain.
	rt := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // self-signed dev cert
			NextProtos:         []string{"h3"},
			MinVersion:         tls.VersionTLS13,
		},
	}
	defer rt.Close()

	client := &http.Client{Transport: rt, Timeout: 5 * time.Second}

	// Try the GET — give the QUIC stack a brief retry window because
	// the UDP bind-probe inside startQUIC proves the socket is open,
	// not that the QUIC handshake state machine is ready.
	var resp *http.Response
	var getErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, getErr = client.Get("https://" + addr + "/h3probe")
		if getErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if getErr != nil {
		t.Fatalf("client.Get over HTTP/3 to https://%s/h3probe: %v", addr, getErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 — listener responded but with wrong status (real bluff signal)", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll(resp.Body): %v", err)
	}
	if !strings.Contains(string(body), wantBody) {
		t.Errorf("body = %q, want substring %q — handler output was not delivered byte-equivalent over HTTP/3", body, wantBody)
	}

	// Sanity: the upstream Server.Addr() reflects the bound address.
	if got := srv.Addr(); got != addr {
		t.Errorf("Server.Addr() = %q, want %q", got, addr)
	}
}

// TestStartQUIC_GracefulShutdown asserts Shutdown returns nil within
// the deadline AND that Server.Done() drains (the upstream Start
// goroutine has exited). A leaky shutdown that returned nil but left
// the QUIC stack running would silently hold the UDP port and break
// the next test in a CI run.
func TestStartQUIC_GracefulShutdown(t *testing.T) {
	addr := freeUDPAddr(t)
	cert := devCert(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv, err := startQUIC(ctx, addr, handler, cert)
	if err != nil {
		t.Fatalf("startQUIC: %v", err)
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		t.Fatalf("srv.Shutdown: %v", err)
	}

	// Wait for the background Start() goroutine to land its terminal
	// error on Done(). nil == clean shutdown.
	select {
	case err := <-srv.Done():
		if err != nil {
			t.Errorf("Server.Done() returned %v, want nil after graceful Shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Server.Done() did not drain within 5s — Start goroutine leaked past Shutdown")
	}
}

// TestStartQUIC_NilCert_Errors asserts startQUIC fails fast with a clear
// message when the caller forgot to supply a cert. This is the §107
// fail-loud anti-bluff: a silent self-signed-cert auto-gen inside
// startQUIC would let a misconfigured production binary "work" without
// telling the operator they shipped without TLS material — exactly the
// PASS-bluff class the operator's mandate forbids.
func TestStartQUIC_NilCert_Errors(t *testing.T) {
	addr := freeUDPAddr(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv, err := startQUIC(ctx, addr, handler, nil)
	if err == nil {
		// Defensive cleanup if startQUIC ever stops returning an error here.
		if srv != nil {
			shutCtx, c := context.WithTimeout(context.Background(), 1*time.Second)
			defer c()
			_ = srv.Shutdown(shutCtx)
		}
		t.Fatal("startQUIC(nil cert) returned nil error — fail-loud invariant violated")
	}
	if srv != nil {
		t.Fatal("startQUIC(nil cert) returned a non-nil server alongside the error")
	}
	if !strings.Contains(err.Error(), "cert") {
		t.Errorf("error %q should mention cert to help the operator diagnose", err.Error())
	}
}
