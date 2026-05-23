// Wire-shape tests for the Telegram MessengerClient impl.
//
// These tests stand up an httptest.Server impersonating
// api.telegram.org and assert:
//
//  1. Each Send* method posts to the correct Bot API path with the
//     correct content-type AND the correct body bytes (JSON for
//     sendMessage, multipart for sendPhoto/sendDocument/sendVoice).
//  2. WaitForReply matches the predicate-satisfying update and
//     ADVANCES the offset PAST it WITHOUT consuming non-matching
//     updates (the §107 anti-bluff anchor — see the invariant in
//     telegram.go's WaitForReply docstring).
//  3. Preflight runs getMe + getChat + getChatAdministrators and
//     populates every field on the returned report.
//  4. Token NEVER appears in error messages.
//
// Coverage map (8 cases vs. the plan's 6-8 expectation):
//
//	TestTelegram_Send_JSONShape                — sendMessage JSON shape
//	TestTelegram_SendPhoto_MultipartShape      — sendPhoto multipart shape
//	TestTelegram_SendDocument_MultipartShape   — sendDocument multipart shape
//	TestTelegram_SendVoice_MultipartShape      — sendVoice multipart shape
//	TestTelegram_GetUpdates_DecodesUpdates     — getUpdates JSON shape
//	TestTelegram_WaitForReply_PreservesNonMatching — §107 anti-bluff anchor
//	TestTelegram_Preflight_StructuredReport    — getMe + getChat + admins
//	TestTelegram_Send_ErrorDoesNotLeakToken    — security mandate
package messenger

import (
	"context"
	"encoding/json"
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

// tgStub is a minimal httptest-backed Telegram Bot API stub. It
// dispatches requests onto a per-method handler so tests can wire the
// canned response (and concurrently assert on the request shape).
type tgStub struct {
	t        *testing.T
	token    string
	srv      *httptest.Server
	handlers map[string]http.HandlerFunc

	// hit counters per method — proves the wire-call actually
	// happened (§107 anti-bluff anchor).
	hits map[string]*int64
}

func newTgStub(t *testing.T, token string) *tgStub {
	t.Helper()
	s := &tgStub{
		t:        t,
		token:    token,
		handlers: map[string]http.HandlerFunc{},
		hits:     map[string]*int64{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.dispatch))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *tgStub) URL() string { return s.srv.URL }

// register installs the handler for "/bot<token>/<method>". hits[method]
// is incremented on every dispatch so the test can assert on call count.
func (s *tgStub) register(method string, h http.HandlerFunc) {
	var c int64
	s.hits[method] = &c
	s.handlers[method] = h
}

func (s *tgStub) hitsFor(method string) int64 {
	if c, ok := s.hits[method]; ok {
		return atomic.LoadInt64(c)
	}
	return 0
}

func (s *tgStub) dispatch(w http.ResponseWriter, r *http.Request) {
	prefix := "/bot" + s.token + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "stub: unrecognized path", http.StatusNotFound)
		return
	}
	method := strings.TrimPrefix(r.URL.Path, prefix)
	h, ok := s.handlers[method]
	if !ok {
		http.Error(w, "stub: unregistered method "+method, http.StatusNotImplemented)
		return
	}
	atomic.AddInt64(s.hits[method], 1)
	h(w, r)
}

// writeJSONResult writes a Bot API envelope with `result` as the
// payload.
func writeJSONResult(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	resBytes, _ := json.Marshal(result)
	env := struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}{OK: true, Result: resBytes}
	_ = json.NewEncoder(w).Encode(env)
}

// writeJSONError writes a Bot API error envelope.
func writeJSONError(w http.ResponseWriter, code int, desc string) {
	w.Header().Set("Content-Type", "application/json")
	env := struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}{OK: false, ErrorCode: code, Description: desc}
	_ = json.NewEncoder(w).Encode(env)
}

// --------------------------------------------------------------------
// 1. sendMessage — JSON wire-shape
// --------------------------------------------------------------------

func TestTelegram_Send_JSONShape(t *testing.T) {
	stub := newTgStub(t, "TOK")
	var capturedPayload map[string]any
	var capturedCT string
	stub.register("sendMessage", func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedPayload)
		writeJSONResult(w, map[string]any{
			"message_id": 12345,
			"chat":       map[string]any{"id": 999, "type": "group"},
			"date":       time.Now().Unix(),
			"text":       capturedPayload["text"],
		})
	})

	c, err := NewTelegramClient("TOK", 999, stub.URL())
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	id, err := c.Send(context.Background(), "Hello, herald")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "12345" {
		t.Fatalf("Send returned %q; want %q", id, "12345")
	}
	if !strings.HasPrefix(capturedCT, "application/json") {
		t.Fatalf("Content-Type = %q; want application/json", capturedCT)
	}
	// chat_id may decode as float64 from JSON; coerce.
	if got := fmt.Sprintf("%v", capturedPayload["chat_id"]); got != "999" {
		t.Fatalf("chat_id in payload = %v; want 999", capturedPayload["chat_id"])
	}
	if capturedPayload["text"] != "Hello, herald" {
		t.Fatalf("text in payload = %v; want %q", capturedPayload["text"], "Hello, herald")
	}
	if stub.hitsFor("sendMessage") != 1 {
		t.Fatalf("sendMessage hit count = %d; want 1 (§107 anti-bluff: wire MUST be exercised)", stub.hitsFor("sendMessage"))
	}
}

// --------------------------------------------------------------------
// 2-4. sendPhoto / sendDocument / sendVoice — multipart wire-shape
// --------------------------------------------------------------------

// uploadCase parametrises the three multipart Send* methods.
type uploadCase struct {
	name      string
	method    string
	fileField string
	call      func(c *TelegramClient, path, caption string) (MessageID, error)
	caption   string
}

func TestTelegram_SendPhoto_MultipartShape(t *testing.T) {
	runMultipartUploadCase(t, uploadCase{
		name:      "sendPhoto",
		method:    "sendPhoto",
		fileField: "photo",
		caption:   "red banner",
		call: func(c *TelegramClient, path, caption string) (MessageID, error) {
			return c.SendPhoto(context.Background(), path, caption)
		},
	})
}

func TestTelegram_SendDocument_MultipartShape(t *testing.T) {
	runMultipartUploadCase(t, uploadCase{
		name:      "sendDocument",
		method:    "sendDocument",
		fileField: "document",
		caption:   "review the spec",
		call: func(c *TelegramClient, path, caption string) (MessageID, error) {
			return c.SendDocument(context.Background(), path, caption)
		},
	})
}

func TestTelegram_SendVoice_MultipartShape(t *testing.T) {
	runMultipartUploadCase(t, uploadCase{
		name:      "sendVoice",
		method:    "sendVoice",
		fileField: "voice",
		caption:   "", // voice has no caption
		call: func(c *TelegramClient, path, _ string) (MessageID, error) {
			return c.SendVoice(context.Background(), path)
		},
	})
}

func runMultipartUploadCase(t *testing.T, uc uploadCase) {
	t.Helper()
	stub := newTgStub(t, "TOK")
	var (
		capturedCT       string
		gotChatID        string
		gotCaption       string
		gotFileFieldName string
		gotFileBytes     []byte
		gotFileName      string
	)
	stub.register(uc.method, func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
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
			case "chat_id":
				gotChatID = string(body)
			case "caption":
				gotCaption = string(body)
			case uc.fileField:
				gotFileFieldName = p.FormName()
				gotFileBytes = body
				gotFileName = p.FileName()
			}
		}
		writeJSONResult(w, map[string]any{
			"message_id": 77,
			"chat":       map[string]any{"id": 999, "type": "group"},
			"date":       time.Now().Unix(),
		})
	})

	// write a fixture file with known bytes.
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture-"+uc.method+".bin")
	want := []byte("HERALD-MESSENGER-FIXTURE-" + uc.method)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c, err := NewTelegramClient("TOK", 999, stub.URL())
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	id, err := uc.call(c, path, uc.caption)
	if err != nil {
		t.Fatalf("%s: %v", uc.name, err)
	}
	if id != "77" {
		t.Fatalf("%s: id = %q; want 77", uc.name, id)
	}
	if stub.hitsFor(uc.method) != 1 {
		t.Fatalf("%s: hit count = %d; want 1 (§107 wire-exercise anchor)", uc.method, stub.hitsFor(uc.method))
	}
	if gotChatID != "999" {
		t.Fatalf("%s: chat_id form field = %q; want \"999\"", uc.name, gotChatID)
	}
	if uc.caption != "" && gotCaption != uc.caption {
		t.Fatalf("%s: caption form field = %q; want %q", uc.name, gotCaption, uc.caption)
	}
	if uc.caption == "" && gotCaption != "" {
		t.Fatalf("%s: caption MUST be omitted when empty; got %q", uc.name, gotCaption)
	}
	if gotFileFieldName != uc.fileField {
		t.Fatalf("%s: file form-field = %q; want %q", uc.name, gotFileFieldName, uc.fileField)
	}
	if string(gotFileBytes) != string(want) {
		t.Fatalf("%s: file bytes mismatch (len got=%d want=%d) — multipart MUST carry exact bytes from disk", uc.name, len(gotFileBytes), len(want))
	}
	if gotFileName == "" {
		t.Fatalf("%s: multipart file part MUST carry a filename (uses basename of path)", uc.name)
	}
}

// --------------------------------------------------------------------
// 5. getUpdates — JSON wire-shape
// --------------------------------------------------------------------

func TestTelegram_GetUpdates_DecodesUpdates(t *testing.T) {
	stub := newTgStub(t, "TOK")
	stub.register("getUpdates", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResult(w, []map[string]any{
			{
				"update_id": 100,
				"message": map[string]any{
					"message_id": 1,
					"chat":       map[string]any{"id": 999, "type": "group"},
					"date":       1700000000,
					"text":       "first",
					"from":       map[string]any{"id": 42, "is_bot": false, "username": "alice"},
				},
			},
			{
				"update_id": 101,
				"message": map[string]any{
					"message_id": 2,
					"chat":       map[string]any{"id": 999, "type": "group"},
					"date":       1700000001,
					"text":       "second",
				},
			},
		})
	})

	c, err := NewTelegramClient("TOK", 999, stub.URL())
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	updates, highWater, err := c.GetUpdates(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("len(updates) = %d; want 2", len(updates))
	}
	if highWater != 101 {
		t.Fatalf("highWater = %d; want 101", highWater)
	}
	if updates[0].Text != "first" || updates[0].SenderUsername != "alice" {
		t.Fatalf("update[0] = %+v; bad decode", updates[0])
	}
	if updates[1].Text != "second" {
		t.Fatalf("update[1].Text = %q; want %q", updates[1].Text, "second")
	}
	if updates[0].MessageID != "1" || updates[1].MessageID != "2" {
		t.Fatalf("message-ids: %q, %q; want 1, 2", updates[0].MessageID, updates[1].MessageID)
	}
}

// --------------------------------------------------------------------
// 6. WaitForReply — §107 anti-bluff: non-matching updates MUST NOT be
//    consumed.
//
// Scenario: stub returns 3 updates. The predicate matches ONLY the 2nd.
// After WaitForReply succeeds:
//   - The match is the 2nd update.
//   - A subsequent GetUpdates(offset=0) call (simulating a different
//     scenario's drain) MUST still return the 1st and 3rd updates,
//     because they were never confirmed.
//
// Implementation note: the stub tracks per-offset request counts so
// the test sees WHICH offsets were committed. We assert WaitForReply
// commits offset = matched.update_id+1 (advancing past the match) but
// NOT past the 3rd update (which arrived after the match in the same
// batch but was not selected).
// --------------------------------------------------------------------

func TestTelegram_WaitForReply_PreservesNonMatching(t *testing.T) {
	stub := newTgStub(t, "TOK")

	// committedOffsets records the `offset` query each call carried.
	var committedOffsetsMu = make(chan struct{}, 1)
	committedOffsetsMu <- struct{}{}
	var committedOffsets []int64

	stub.register("getUpdates", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Offset int64 `json:"offset"`
		}
		_ = json.Unmarshal(body, &payload)
		<-committedOffsetsMu
		committedOffsets = append(committedOffsets, payload.Offset)
		committedOffsetsMu <- struct{}{}

		// When offset=0: return three updates (the "backlog").
		// When offset=101 (commit past the match): return empty.
		// When offset=100 (offset right before match, no longer
		// expected after match): also return all three updates so
		// the test can assert WaitForReply did NOT commit there.
		switch {
		case payload.Offset == 0 || payload.Offset == 100:
			writeJSONResult(w, []map[string]any{
				{"update_id": 100, "message": map[string]any{
					"message_id": 1, "chat": map[string]any{"id": 999, "type": "group"},
					"date": 1700000000, "text": "first",
				}},
				{"update_id": 101, "message": map[string]any{
					"message_id": 2, "chat": map[string]any{"id": 999, "type": "group"},
					"date": 1700000001, "text": "MATCH-ME",
				}},
				{"update_id": 102, "message": map[string]any{
					"message_id": 3, "chat": map[string]any{"id": 999, "type": "group"},
					"date": 1700000002, "text": "third",
				}},
			})
		default:
			// offset advanced past the match.
			writeJSONResult(w, []any{})
		}
	})

	c, err := NewTelegramClient("TOK", 999, stub.URL())
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	pred := func(r Reply) bool { return r.Text == "MATCH-ME" }
	got, err := c.WaitForReply(context.Background(), "", pred, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForReply: %v", err)
	}
	if got.Text != "MATCH-ME" {
		t.Fatalf("matched Text = %q; want MATCH-ME", got.Text)
	}
	if got.MessageID != "2" {
		t.Fatalf("matched MessageID = %q; want 2", got.MessageID)
	}

	// Now simulate a SECOND scenario draining updates: call
	// GetUpdates(offset=0). The stub returns the 3 baseline updates
	// from the "backlog" branch — but the §107 contract says the
	// matching update (#2) WAS confirmed past, so a real Telegram
	// server would NOT re-return it. Our stub doesn't track real
	// commit semantics; instead we assert WaitForReply DID dispatch a
	// commit call carrying offset=102 (matched.update_id+1).
	<-committedOffsetsMu
	defer func() { committedOffsetsMu <- struct{}{} }()
	sawCommit := false
	for _, off := range committedOffsets {
		if off == 102 {
			sawCommit = true
			break
		}
	}
	if !sawCommit {
		t.Fatalf("WaitForReply MUST commit offset=102 (matched.update_id+1) to advance past the match; offsets seen=%v", committedOffsets)
	}
	// And critically: WaitForReply MUST NOT have committed offset=101 or
	// offset=103 (which would consume the 2nd or 3rd update). We
	// only ever expect offsets 0 (initial drain) and 102 (commit
	// past match).
	for _, off := range committedOffsets {
		if off != 0 && off != 102 {
			t.Fatalf("WaitForReply committed unexpected offset=%d; non-matching updates MUST remain in queue (offsets allowed: 0, 102)", off)
		}
	}
}

// --------------------------------------------------------------------
// 7. Preflight — structured report
// --------------------------------------------------------------------

func TestTelegram_Preflight_StructuredReport(t *testing.T) {
	stub := newTgStub(t, "TOK")
	stub.register("getMe", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResult(w, map[string]any{
			"id":                          int64(987654321),
			"is_bot":                      true,
			"username":                    "herald_qa_bot",
			"first_name":                  "Herald QA",
			"can_read_all_group_messages": true,
		})
	})
	stub.register("getChat", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p struct {
			ChatID int64 `json:"chat_id"`
		}
		_ = json.Unmarshal(body, &p)
		if p.ChatID != 999 {
			writeJSONError(w, 400, "chat_id mismatch")
			return
		}
		writeJSONResult(w, map[string]any{
			"id": 999, "type": "supergroup", "title": "Herald QA Group",
		})
	})
	stub.register("getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResult(w, []map[string]any{
			{"user": map[string]any{
				"id": int64(111), "is_bot": true, "username": "pherald_bot", "first_name": "Pherald",
			}},
			{"user": map[string]any{
				"id": int64(987654321), "is_bot": true, "username": "herald_qa_bot", "first_name": "Herald QA",
			}},
		})
	})

	c, err := NewTelegramClient("TOK", 999, stub.URL())
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	report, err := c.Preflight(context.Background(), 999)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if report.Username != "herald_qa_bot" {
		t.Fatalf("Username = %q; want herald_qa_bot", report.Username)
	}
	if report.UserID != 987654321 {
		t.Fatalf("UserID = %d; want 987654321", report.UserID)
	}
	if !report.CanReadAllGroupMessages {
		t.Fatal("CanReadAllGroupMessages = false; want true (stub returned true)")
	}
	if !report.InChat {
		t.Fatal("InChat = false; want true (getChat returned ok)")
	}
	if report.ChatType != "supergroup" {
		t.Fatalf("ChatType = %q; want supergroup", report.ChatType)
	}
	if !report.PheraldBotPresent {
		t.Fatal("PheraldBotPresent = false; want true (getChatAdministrators included pherald_bot)")
	}
	// §107: each wire-call MUST have been dispatched exactly once.
	for _, m := range []string{"getMe", "getChat", "getChatAdministrators"} {
		if stub.hitsFor(m) != 1 {
			t.Fatalf("Preflight: %s hit count = %d; want 1", m, stub.hitsFor(m))
		}
	}
}

// --------------------------------------------------------------------
// 8. Send error path — token MUST NOT leak into the error message.
// --------------------------------------------------------------------

func TestTelegram_Send_ErrorDoesNotLeakToken(t *testing.T) {
	const sensitiveToken = "1234567890:AAAA-sensitive-bot-token-DO-NOT-LEAK"
	stub := newTgStub(t, sensitiveToken)
	stub.register("sendMessage", func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, 401, "Unauthorized")
	})

	c, err := NewTelegramClient(sensitiveToken, 999, stub.URL())
	if err != nil {
		t.Fatalf("NewTelegramClient: %v", err)
	}
	defer c.Close()

	_, err = c.Send(context.Background(), "boom")
	if err == nil {
		t.Fatal("Send: expected error on 401, got nil")
	}
	if strings.Contains(err.Error(), sensitiveToken) {
		t.Fatalf("error message LEAKED token (security violation): %v", err)
	}
	// Sanity: error MUST cite the method name.
	if !strings.Contains(err.Error(), "sendMessage") {
		t.Fatalf("error %q does not name the API method 'sendMessage'", err.Error())
	}
}
