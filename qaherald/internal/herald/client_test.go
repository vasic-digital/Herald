// Wave 5 Task 4 — TOON round-trip hermetic test for the qaherald
// Herald REST client.
//
// Scope: this test does NOT touch a live pherald. It stands up an
// httptest.NewTLSServer whose handler:
//   - Asserts the inbound Authorization header carries a bearer token
//     (i.e. the client signed the JWT).
//   - Asserts the inbound Accept header equals application/toon.
//   - Asserts the inbound Content-Type header equals application/toon
//     (the client's encode() path stamped the right codec).
//   - Emits a 202 Accepted with a TOON-encoded Receipt body whose
//     EventID matches a unique sentinel.
//
// The client uses `c.http = srv.Client()` to share the test server's
// self-signed TLS trust, then PostEvents a CloudEvent stub with
// Accept = AcceptTOON. The test asserts:
//   - status == 202 (the verbatim code; T10 mutation gate b is the
//     mirror — a `return 202` shortcut would PASS here, the deny-path
//     scenario in T5 catches the bluff because it expects 403).
//   - The decoded Receipt.EventID equals the sentinel — proves the
//     client read TOON bytes from the wire AND ran them through the
//     real digital.vasic.toon codec (a hard-coded JSON branch would
//     fail json.Unmarshal of TOON bytes).
//
// Live end-to-end coverage (real pherald, real Telegram) lives in T7
// + T8; this test is the hermetic §107 anchor for the client itself.
package herald

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	toon "digital.vasic.toon/pkg/toon"
)

// TestPostEvent_TOONRoundTrip exercises the TOON-encoded request +
// TOON-encoded response path end-to-end against a hermetic TLS test
// server. See package comment for the §107 reasoning.
func TestPostEvent_TOONRoundTrip(t *testing.T) {
	t.Parallel()

	const wantEventID = "evt-toon-roundtrip-1"

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: want POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/events" {
			t.Errorf("path: want /v1/events, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != AcceptTOON {
			t.Errorf("Accept header: want %q, got %q", AcceptTOON, got)
		}
		if got := r.Header.Get("Content-Type"); got != AcceptTOON {
			t.Errorf("Content-Type header: want %q, got %q", AcceptTOON, got)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") || len(got) <= len("Bearer ") {
			t.Errorf("Authorization header: want non-empty Bearer token, got %q", got)
		}
		// Encode the reply through the REAL digital.vasic.toon codec.
		// §107: a mutation that wrote JSON bytes here under the TOON
		// CT would surface as the client's decode() path returning
		// EventID == "" because JSON bytes do not round-trip through
		// toon.Unmarshal.
		body, err := toon.Marshal(Receipt{
			EventID:    wantEventID,
			Recipients: 1,
			Status:     "accepted",
		})
		if err != nil {
			t.Fatalf("server toon.Marshal: %v", err)
		}
		w.Header().Set("Content-Type", AcceptTOON)
		w.WriteHeader(http.StatusAccepted)
		if _, err := w.Write(body); err != nil {
			t.Errorf("server write: %v", err)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, []byte("test-secret"))
	c.http = srv.Client() // share TLS trust with the httptest server

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rcpt, status, hdr, err := c.PostEvent(ctx, CloudEvent{
		SpecVersion: "1.0",
		ID:          "test-1",
		Source:      "qa",
		Type:        "test",
		Time:        time.Now().UTC(),
	}, AcceptTOON)

	if err != nil {
		t.Fatalf("PostEvent: %v", err)
	}
	if status != http.StatusAccepted {
		t.Fatalf("status: want %d, got %d", http.StatusAccepted, status)
	}
	if got := hdr.Get("Content-Type"); got != AcceptTOON {
		t.Errorf("response Content-Type: want %q, got %q", AcceptTOON, got)
	}
	if rcpt.EventID != wantEventID {
		t.Fatalf("Receipt.EventID: want %q, got %q (Recipients=%d Status=%q) — likely TOON decode regression",
			wantEventID, rcpt.EventID, rcpt.Recipients, rcpt.Status)
	}
	if rcpt.Recipients != 1 {
		t.Errorf("Receipt.Recipients: want 1, got %d", rcpt.Recipients)
	}
	if rcpt.Status != "accepted" {
		t.Errorf("Receipt.Status: want %q, got %q", "accepted", rcpt.Status)
	}
}
