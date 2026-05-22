// Wave 4a Task 7.5 — pherald serve unification onto cli.RunServe.
//
// Per §107: pherald's previous newServeCmd rolled its own http.Server,
// which silently diverged from the dual-listener (TCP/H2 + UDP/H3 +
// Brotli + Alt-Svc) engine every other flavor used after Wave 4a T6/T7.
// POST /v1/events shipped TCP-only despite the H3 mandate. The
// unification refactor delegates the listener body to cli.RunServe; the
// tests below assert the §107-load-bearing evidence that pherald's
// serve plane actually uses the shared engine:
//
//  1. TestPheraldServeCmd_FlagsMatchSharedEngine — pherald's `serve --help`
//     surfaces the SHARED cli.BindServeFlags strings (the dual-listener
//     "TCP+UDP port to bind" wording, --tls-cert/--tls-key/--no-brotli),
//     not the legacy TCP-only language. A regression here means pherald
//     drifted back to its own flag definitions and probably its own
//     http.Server too.
//
//  2. TestPheraldServeCmd_BrotliOnFlavorRoute — drives pherald-shaped
//     opts (Branding + a flavor Route) through cli.RunServe in TCP-only
//     mode and asserts Content-Encoding: br lands on the response when
//     the client sends Accept-Encoding: br. Brotli middleware is
//     auto-wired ONLY by cli.RunServe (the legacy pherald http.Server
//     had no such middleware); observing the br encoding is the engine-
//     identity probe.
//
// We do NOT spawn the full pherald `serve` subcommand in these tests
// because buildRunner requires HERALD_PG_DSN + HERALD_AUTH_MODE and
// the §107 covenant explicitly forbids skipping those — that's a
// production-mode contract test, not a unit test. The engine-identity
// assertion above is the load-bearing positive evidence that pherald's
// serve plane uses cli.RunServe.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/cli"
)

// TestPheraldServeCmd_FlagsMatchSharedEngine asserts pherald's `serve`
// subcommand surfaces the SHARED cli.BindServeFlags definitions, not
// its own bespoke flag set. The "TCP+UDP port to bind" wording is the
// fingerprint of cli.BindServeFlags; the legacy pherald-rolled flag
// said "TCP port to bind" (no UDP). A divergence here means newServeCmd
// regressed off the unified path.
func TestPheraldServeCmd_FlagsMatchSharedEngine(t *testing.T) {
	br := commons.DefaultBranding("p", "0.0.0-test")
	cmd := newServeCmd(br)
	if cmd == nil {
		t.Fatal("newServeCmd returned nil")
	}

	flag := cmd.Flags().Lookup("http-port")
	if flag == nil {
		t.Fatal("--http-port flag not registered")
	}
	if !strings.Contains(flag.Usage, "TCP+UDP port to bind") {
		t.Errorf("--http-port usage = %q, want shared dual-listener wording (TCP+UDP); pherald may have regressed off cli.BindServeFlags",
			flag.Usage)
	}

	// --tls-cert / --tls-key / --no-brotli also part of the shared flag
	// set; verify they're present so a future regression that drops one
	// of them (and silently breaks operator CLI muscle memory) is caught.
	for _, name := range []string{"tls-cert", "tls-key", "no-brotli"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s flag missing from pherald serve; cli.BindServeFlags drift",
				name)
		}
	}
}

// TestPheraldServeCmd_BrotliOnFlavorRoute is the engine-identity probe.
// It builds pherald-shaped ServeOpts (a flavor Route + branding) and
// runs cli.RunServe directly in TCP-only mode (DisableH3 via env, so no
// TLS cert needed). The Brotli middleware is AUTO-WIRED by cli.RunServe
// for every flavor route registered after r.Use(BrotliMiddleware(0));
// pherald's previous bespoke http.Server had NO such middleware. So
// observing Content-Encoding: br on the response — when the client
// sends Accept-Encoding: br — proves the request flowed through the
// shared cli.RunServe engine. A pherald that regressed back to its own
// http.Server would return uncompressed bytes.
//
// Why this beats an Alt-Svc probe: Alt-Svc middleware is a no-op when
// DisableH3=true (h3Port=0 ⇒ no header emitted), making it useless as
// an identity fingerprint in the hermetic TCP-only test mode. Brotli
// fires unconditionally on flavor routes regardless of H3 enablement.
func TestPheraldServeCmd_BrotliOnFlavorRoute(t *testing.T) {
	t.Setenv("HERALD_DISABLE_HTTP3", "1") // TCP-only for hermetic test
	br := commons.DefaultBranding("p", "0.0.0-test")

	port := freeTCPPort(t)
	br.DefaultPort = port

	// Register a flavor route that emits enough bytes to actually trigger
	// brotli compression (the middleware short-circuits sub-MTU
	// responses for efficiency). 4 KiB of zeros compress to ~50 bytes.
	payload := strings.Repeat("a", 4096)
	opts := cli.ServeOpts{
		Branding: br,
		Routes: []cli.Route{
			{
				Method: "GET",
				Path:   "/v1/probe",
				Handler: func(c *gin.Context) {
					c.Header("Content-Type", "text/plain")
					c.String(http.StatusOK, payload)
				},
				Description: "engine-identity probe route",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- cli.RunServe(ctx, opts, port) }()

	if err := waitForTCP(port, 3*time.Second); err != nil {
		cancel()
		t.Fatalf("listener did not come up: %v", err)
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/v1/probe", port), nil)
	if err != nil {
		cancel()
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "br")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET /v1/probe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// Engine-identity assertion: Content-Encoding: br is the
	// cli.RunServe fingerprint. The legacy pherald http.Server had no
	// Brotli middleware ⇒ this header would be absent.
	if enc := resp.Header.Get("Content-Encoding"); enc != "br" {
		t.Errorf("Content-Encoding = %q, want %q; pherald is not using cli.RunServe (engine identity probe FAILED)",
			enc, "br")
	}

	// Sanity: decode the brotli body and verify it matches our payload.
	// A bogus middleware that lied with the header but didn't actually
	// compress would FAIL this check.
	brReader := brotli.NewReader(resp.Body)
	decoded, err := io.ReadAll(brReader)
	if err != nil {
		t.Fatalf("brotli decode: %v", err)
	}
	if string(decoded) != payload {
		t.Errorf("decoded body len = %d, want %d (round-trip mismatch)", len(decoded), len(payload))
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("RunServe exit error = %v, want nil or context.Canceled", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("RunServe did not shut down within 7s")
	}
}

// freeTCPPort finds a port free on TCP and returns it. We don't need
// dual TCP+UDP free here because the test sets HERALD_DISABLE_HTTP3=1
// (TCP-only mode), so the UDP socket is never bound.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeTCPPort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// waitForTCP polls until the given port accepts a TCP connection or the
// deadline expires.
func waitForTCP(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timed out waiting for TCP listener")
}

