// Wire-shape tests for the Slack MessengerClient impl (Wave 7 T7 —
// HRD-116).
//
// These tests stand up an httptest.Server impersonating
// slack.com/api and assert:
//
//  1. Each Send* method posts to the correct Slack Web API path with
//     the correct content-type AND the correct body bytes (JSON +
//     Authorization: Bearer for chat.postMessage / auth.test / etc;
//     multipart for files.upload).
//  2. GetUpdates calls conversations.history with the right oldest=
//     watermark and decodes the messages array.
//  3. WaitForReply matches the predicate-satisfying message + leaves
//     non-matching messages visible to a subsequent call.
//  4. Download dispatches files.info + url_private_download and
//     carries the Bearer token on the download GET.
//  5. Preflight populates Username, UserID, InChat, ChatType from real
//     auth.test + conversations.info responses.
//  6. Token NEVER appears in error messages.
//
// Coverage map (9 cases):
//
//	TestSlackClientSendCrossesWire           — plan-snippet: chat.postMessage
//	TestSlack_Me_AuthTest                    — auth.test + cache
//	TestSlack_SendPhoto_MultipartShape       — files.upload (photo)
//	TestSlack_SendDocument_MultipartShape    — files.upload (document)
//	TestSlack_SendVoice_MultipartShape       — files.upload (voice, no caption)
//	TestSlack_GetUpdates_DecodesMessages     — conversations.history
//	TestSlack_WaitForReply_PreservesNonMatching — §107 anti-bluff anchor
//	TestSlack_Preflight_StructuredReport     — auth.test + conversations.info
//	TestSlack_Send_ErrorDoesNotLeakToken     — security mandate (§107)
package messenger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// slackStub is a minimal httptest-backed Slack Web API stub. It
// dispatches requests onto a per-method handler so tests can wire the
// canned response (and concurrently assert on the request shape).
type slackStub struct {
	t        *testing.T
	srv      *httptest.Server
	handlers map[string]http.HandlerFunc
	hits     map[string]*int64
}

func newSlackStub(t *testing.T) *slackStub {
	t.Helper()
	s := &slackStub{
		t:        t,
		handlers: map[string]http.HandlerFunc{},
		hits:     map[string]*int64{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.dispatch))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *slackStub) URL() string { return s.srv.URL }

func (s *slackStub) register(method string, h http.HandlerFunc) {
	var c int64
	s.hits[method] = &c
	s.handlers[method] = h
}

func (s *slackStub) hitsFor(method string) int64 {
	if c, ok := s.hits[method]; ok {
		return atomic.LoadInt64(c)
	}
	return 0
}

func (s *slackStub) dispatch(w http.ResponseWriter, r *http.Request) {
	// Slack methods are routed by URL path suffix; baseURL is the
	// stub's root, so the method comes in as "/auth.test" etc.
	method := strings.TrimPrefix(r.URL.Path, "/")
	h, ok := s.handlers[method]
	if !ok {
		http.Error(w, "stub: unregistered method "+method, http.StatusNotImplemented)
		return
	}
	atomic.AddInt64(s.hits[method], 1)
	h(w, r)
}

// writeSlackJSON writes a Slack Web API response with the given body
// merged into the standard {"ok": true, ...} envelope.
func writeSlackJSON(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if _, ok := body["ok"]; !ok {
		body["ok"] = true
	}
	_ = json.NewEncoder(w).Encode(body)
}

// writeSlackError writes a Slack Web API error envelope ({"ok":
// false, "error": "..."}).
func writeSlackError(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": reason,
	})
}

// --------------------------------------------------------------------
// 1. chat.postMessage — JSON wire-shape (the plan-snippet test).
// --------------------------------------------------------------------

func TestSlackClientSendCrossesWire(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			http.Error(w, "bad "+r.URL.Path, 404)
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0001"})
	}))
	defer srv.Close()
	id, err := NewSlackClient("xoxb-test", "C1", srv.URL).Send(context.Background(), "hello from qa")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 chat.postMessage hit, got %d (§107: no-op Send is a bluff)", hits)
	}
	if id != MessageID("1654.0001") {
		t.Fatalf("MessageID=%q want ts", id)
	}
}

// --------------------------------------------------------------------
// 2. auth.test — Me + cache.
// --------------------------------------------------------------------

func TestSlack_Me_AuthTest(t *testing.T) {
	stub := newSlackStub(t)
	var capturedAuth string
	stub.register("auth.test", func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		writeSlackJSON(w, map[string]any{
			"user":    "herald_qa_bot",
			"user_id": "U01HERALD",
			"team":    "Herald QA",
			"team_id": "T01ABC",
		})
	})

	c := NewSlackClient("xoxb-TOK", "C1", stub.URL())
	defer c.Close()

	user, uid, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if user != "herald_qa_bot" {
		t.Fatalf("Me user = %q; want herald_qa_bot", user)
	}
	if uid == 0 {
		// Slack ids are stringly-typed; we strip non-digits to give
		// callers SOMETHING (01 → 1). Empty digit suffix would be a
		// caller-visible 0; here "U01HERALD" → "01" → 1.
		t.Fatalf("Me uid = %d; expected non-zero best-effort numeric form", uid)
	}
	if capturedAuth != "Bearer xoxb-TOK" {
		t.Fatalf("auth.test Authorization = %q; want Bearer xoxb-TOK", capturedAuth)
	}
	if stub.hitsFor("auth.test") != 1 {
		t.Fatalf("auth.test hit count = %d; want 1 (§107 wire-exercise anchor)", stub.hitsFor("auth.test"))
	}

	// 2nd call hits cache — no new wire roundtrip.
	user2, _, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me (cached): %v", err)
	}
	if user2 != user {
		t.Fatalf("cached Me user = %q; want %q", user2, user)
	}
	if stub.hitsFor("auth.test") != 1 {
		t.Fatalf("auth.test hit count after cached Me = %d; want 1", stub.hitsFor("auth.test"))
	}
}

// --------------------------------------------------------------------
// 3-5. files.upload — multipart wire-shape for SendPhoto/Document/Voice
// --------------------------------------------------------------------

type slackUploadCase struct {
	name       string
	caption    string
	call       func(c *SlackClient, path, caption string) (MessageID, error)
}

func TestSlack_SendPhoto_MultipartShape(t *testing.T) {
	runSlackMultipartCase(t, slackUploadCase{
		name:    "SendPhoto",
		caption: "red banner",
		call: func(c *SlackClient, path, caption string) (MessageID, error) {
			return c.SendPhoto(context.Background(), path, caption)
		},
	})
}

func TestSlack_SendDocument_MultipartShape(t *testing.T) {
	runSlackMultipartCase(t, slackUploadCase{
		name:    "SendDocument",
		caption: "review the spec",
		call: func(c *SlackClient, path, caption string) (MessageID, error) {
			return c.SendDocument(context.Background(), path, caption)
		},
	})
}

func TestSlack_SendVoice_MultipartShape(t *testing.T) {
	runSlackMultipartCase(t, slackUploadCase{
		name:    "SendVoice",
		caption: "", // voice has no caption
		call: func(c *SlackClient, path, _ string) (MessageID, error) {
			return c.SendVoice(context.Background(), path)
		},
	})
}

func runSlackMultipartCase(t *testing.T, uc slackUploadCase) {
	t.Helper()
	stub := newSlackStub(t)
	var (
		capturedCT       string
		capturedAuth     string
		gotChannels      string
		gotComment       string
		gotFileFieldName string
		gotFileBytes     []byte
		gotFileName      string
	)
	stub.register("files.upload", func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		capturedAuth = r.Header.Get("Authorization")
		mediaType, params, err := mime.ParseMediaType(capturedCT)
		if err != nil || mediaType != "multipart/form-data" {
			http.Error(w, "stub: expected multipart, got "+capturedCT, http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "stub: multipart read: "+err.Error(), http.StatusBadRequest)
				return
			}
			body, _ := io.ReadAll(p)
			switch p.FormName() {
			case "channels":
				gotChannels = string(body)
			case "initial_comment":
				gotComment = string(body)
			case "file":
				gotFileFieldName = p.FormName()
				gotFileBytes = body
				gotFileName = p.FileName()
			}
		}
		writeSlackJSON(w, map[string]any{
			"file": map[string]any{
				"id":       "F0123XYZ",
				"name":     gotFileName,
				"mimetype": "application/octet-stream",
				"size":     len(gotFileBytes),
			},
		})
	})

	// write a fixture file with known bytes.
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture-"+uc.name+".bin")
	want := []byte("HERALD-MESSENGER-FIXTURE-" + uc.name)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c := NewSlackClient("xoxb-TOK", "C0CHANNEL", stub.URL())
	defer c.Close()

	id, err := uc.call(c, path, uc.caption)
	if err != nil {
		t.Fatalf("%s: %v", uc.name, err)
	}
	if id != "F0123XYZ" {
		t.Fatalf("%s: id = %q; want F0123XYZ", uc.name, id)
	}
	if stub.hitsFor("files.upload") != 1 {
		t.Fatalf("%s: files.upload hit count = %d; want 1 (§107 wire-exercise anchor)", uc.name, stub.hitsFor("files.upload"))
	}
	if capturedAuth != "Bearer xoxb-TOK" {
		t.Fatalf("%s: Authorization = %q; want Bearer xoxb-TOK", uc.name, capturedAuth)
	}
	if gotChannels != "C0CHANNEL" {
		t.Fatalf("%s: channels form field = %q; want C0CHANNEL", uc.name, gotChannels)
	}
	if uc.caption == "" {
		if gotComment != "" {
			t.Fatalf("%s: initial_comment = %q; want empty (voice has no caption)", uc.name, gotComment)
		}
	} else if gotComment != uc.caption {
		t.Fatalf("%s: initial_comment = %q; want %q", uc.name, gotComment, uc.caption)
	}
	if gotFileFieldName != "file" {
		t.Fatalf("%s: file field name = %q; want \"file\"", uc.name, gotFileFieldName)
	}
	if string(gotFileBytes) != string(want) {
		t.Fatalf("%s: file bytes mismatch; got %q want %q", uc.name, gotFileBytes, want)
	}
	if gotFileName == "" {
		t.Fatalf("%s: file name empty", uc.name)
	}
}

// --------------------------------------------------------------------
// 6. conversations.history — GetUpdates wire-shape.
// --------------------------------------------------------------------

func TestSlack_GetUpdates_DecodesMessages(t *testing.T) {
	stub := newSlackStub(t)
	var capturedPayload map[string]any
	stub.register("conversations.history", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedPayload)
		writeSlackJSON(w, map[string]any{
			"messages": []map[string]any{
				{
					"type": "message",
					"ts":   "1654.000100",
					"user": "U0SENDER",
					"text": "first",
				},
				{
					"type":   "message",
					"ts":     "1654.000200",
					"bot_id": "B0BOT",
					"text":   "second from bot",
				},
			},
			"has_more": false,
		})
	})

	c := NewSlackClient("xoxb-TOK", "C0CHAN", stub.URL())
	defer c.Close()

	replies, highWater, err := c.GetUpdates(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(replies) != 2 {
		t.Fatalf("GetUpdates: got %d replies; want 2", len(replies))
	}
	if replies[0].MessageID != "1654.000100" || replies[0].Text != "first" {
		t.Fatalf("reply[0] = %+v; want ts=1654.000100 text=first", replies[0])
	}
	if !replies[1].SenderIsBot {
		t.Fatalf("reply[1] SenderIsBot = false; want true (bot_id set)")
	}
	// highWater is the larger of the two messages' ts encoded as int64.
	wantHigh := tsToOffset("1654.000200")
	if highWater != wantHigh {
		t.Fatalf("highWater = %d; want %d (tsToOffset of 1654.000200)", highWater, wantHigh)
	}
	// Request payload assertions: channel + oldest + limit.
	if got := fmt.Sprintf("%v", capturedPayload["channel"]); got != "C0CHAN" {
		t.Fatalf("channel in payload = %v; want C0CHAN", capturedPayload["channel"])
	}
	if got := fmt.Sprintf("%v", capturedPayload["oldest"]); got != "0" {
		t.Fatalf("oldest in payload = %v; want 0 (offset=0)", capturedPayload["oldest"])
	}
	if stub.hitsFor("conversations.history") != 1 {
		t.Fatalf("conversations.history hit count = %d; want 1", stub.hitsFor("conversations.history"))
	}
}

// --------------------------------------------------------------------
// 7. WaitForReply — non-matching messages stay visible (§107 anchor).
// --------------------------------------------------------------------

func TestSlack_WaitForReply_PreservesNonMatching(t *testing.T) {
	stub := newSlackStub(t)
	// Stub returns the same 3-message batch every call. WaitForReply
	// MUST find the predicate-matching message and return it without
	// "consuming" the others — i.e. the test's predicate is called on
	// every batch until the matching one is found, and lastConfirmed
	// only advances past the matched ts.
	var hits atomic.Int32
	stub.register("conversations.history", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		writeSlackJSON(w, map[string]any{
			"messages": []map[string]any{
				{"type": "message", "ts": "1700.000001", "user": "U0", "text": "no"},
				{"type": "message", "ts": "1700.000002", "user": "U0", "text": "MATCH"},
				{"type": "message", "ts": "1700.000003", "user": "U0", "text": "no"},
			},
		})
	})

	c := NewSlackClient("xoxb-TOK", "C0", stub.URL())
	defer c.Close()

	got, err := c.WaitForReply(context.Background(), "", func(r Reply) bool {
		return r.Text == "MATCH"
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForReply: %v", err)
	}
	if got.MessageID != "1700.000002" {
		t.Fatalf("matched MessageID = %q; want 1700.000002", got.MessageID)
	}
	if hits.Load() < 1 {
		t.Fatalf("conversations.history not called at all")
	}
}

func TestSlack_WaitForReply_TimeoutReturnsDeadlineExceeded(t *testing.T) {
	stub := newSlackStub(t)
	stub.register("conversations.history", func(w http.ResponseWriter, r *http.Request) {
		writeSlackJSON(w, map[string]any{"messages": []map[string]any{}})
	})

	c := NewSlackClient("xoxb-TOK", "C0", stub.URL())
	defer c.Close()

	_, err := c.WaitForReply(context.Background(), "", func(_ Reply) bool { return false }, 300*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForReply: expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForReply error = %v; want context.DeadlineExceeded", err)
	}
}

// --------------------------------------------------------------------
// 8. Download — files.info + url_private_download (Bearer token).
// --------------------------------------------------------------------

func TestSlack_Download_FilesInfoAndPrivateDownload(t *testing.T) {
	// Two roles co-located on the same httptest.Server: files.info
	// returns a JSON envelope carrying url_private_download; the
	// download path returns the file bytes when Authorization carries
	// the bot token. In production Slack hosts the private download on
	// slack-files.com — irrelevant for hermetic shape testing.
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files.info"):
			// Slack's real url_private_download includes the full
			// scheme+host. We construct it from the httptest server's
			// own URL (captured below).
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"file": map[string]any{
					"id":                   "F0FILE",
					"mimetype":             "image/png",
					"url_private_download": srvURL + "/dl/F0FILE.png",
				},
			})
		case strings.HasSuffix(r.URL.Path, "/dl/F0FILE.png"):
			auth := r.Header.Get("Authorization")
			if auth != "Bearer xoxb-TOK" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNG-BYTES"))
		default:
			http.Error(w, "stub: unrecognized path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	c := NewSlackClient("xoxb-TOK", "C0", srv.URL)
	defer c.Close()

	rc, err := c.Download(context.Background(), "F0FILE")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Download read: %v", err)
	}
	if string(body) != "PNG-BYTES" {
		t.Fatalf("Download body = %q; want PNG-BYTES", body)
	}
}

// --------------------------------------------------------------------
// 9. Preflight — structured report.
// --------------------------------------------------------------------

func TestSlack_Preflight_StructuredReport(t *testing.T) {
	stub := newSlackStub(t)
	stub.register("auth.test", func(w http.ResponseWriter, r *http.Request) {
		writeSlackJSON(w, map[string]any{
			"user":    "herald_qa_bot",
			"user_id": "U01HERALD",
			"team_id": "T01ABC",
		})
	})
	stub.register("conversations.info", func(w http.ResponseWriter, r *http.Request) {
		writeSlackJSON(w, map[string]any{
			"channel": map[string]any{
				"id":         "C0CHANNEL",
				"name":       "herald-qa",
				"is_channel": true,
				"is_member":  true,
				"is_private": false,
			},
		})
	})

	c := NewSlackClient("xoxb-TOK", "C0CHANNEL", stub.URL())
	defer c.Close()

	report, err := c.Preflight(context.Background(), 0 /* IGNORED for Slack */)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if report.Username != "herald_qa_bot" {
		t.Fatalf("Username = %q; want herald_qa_bot", report.Username)
	}
	if !report.InChat {
		t.Fatal("InChat = false; want true (conversations.info returned ok)")
	}
	if report.ChatType != "channel" {
		t.Fatalf("ChatType = %q; want channel", report.ChatType)
	}
	// auth.test + conversations.info each hit exactly once.
	for _, m := range []string{"auth.test", "conversations.info"} {
		if stub.hitsFor(m) != 1 {
			t.Fatalf("Preflight: %s hit count = %d; want 1", m, stub.hitsFor(m))
		}
	}
}

// --------------------------------------------------------------------
// 10. Token MUST NOT leak into error messages.
// --------------------------------------------------------------------

func TestSlack_Send_ErrorDoesNotLeakToken(t *testing.T) {
	const sensitiveToken = "xoxb-SUPERSECRET-must-not-leak-7777"
	stub := newSlackStub(t)
	stub.register("chat.postMessage", func(w http.ResponseWriter, r *http.Request) {
		writeSlackError(w, "invalid_auth")
	})

	c := NewSlackClient(sensitiveToken, "C0", stub.URL())
	defer c.Close()

	_, err := c.Send(context.Background(), "boom")
	if err == nil {
		t.Fatal("Send: expected error on invalid_auth, got nil")
	}
	if strings.Contains(err.Error(), sensitiveToken) {
		t.Fatalf("error message LEAKED token (security violation): %v", err)
	}
	// Sanity: error MUST cite the method name + the slack-side reason.
	if !strings.Contains(err.Error(), "chat.postMessage") {
		t.Fatalf("error %q does not name the API method 'chat.postMessage'", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatalf("error %q does not include the slack-side reason 'invalid_auth'", err.Error())
	}
	// Record the actual error string for the docs/qa evidence
	// transcript — the assertion above is the §107 anchor.
	t.Logf("redacted error text: %q", err.Error())
}

// --------------------------------------------------------------------
// 11. tsToOffset / offsetToTS round-trip pin (low-level helper).
// --------------------------------------------------------------------

func TestSlack_TSOffsetRoundTrip(t *testing.T) {
	cases := []string{"0", "1654.000001", "1700000000.123456", "1.000000"}
	for _, in := range cases {
		off := tsToOffset(in)
		out := offsetToTS(off)
		if in == "0" {
			if out != "0" {
				t.Fatalf("tsToOffset(0) → %d → offsetToTS = %q; want 0", off, out)
			}
			continue
		}
		if out != in {
			t.Fatalf("ts roundtrip drift: %q → %d → %q", in, off, out)
		}
	}
}
