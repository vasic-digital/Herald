package tgram_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// TestSendReplyEmitsReplyToMessageID pins Wave 6 T8's §107 anti-bluff anchor
// per docs/superpowers/plans/2026-05-22-wave6-inbound-runtime.md:
//
//	A SendReply that "compiles cleanly but the ReplyTo field's ID is always
//	0" would pass type-checks and even render as a non-error round-trip; the
//	test asserts the wire-byte payload contains reply_to_message_id with the
//	expected value. Wave 6 mutation gate (c) drops opts.ReplyTo; the
//	detector asserts this test catches it.
//
// The assertion targets the bytes-on-wire (raw request body) rather than
// the SendOptions struct. telebot.v3.3.8 sends sendMessage as JSON body
// (Content-Type: application/json — see submodules/telebot/bot_raw.go:48-52
// + the json.Encoder write at line 26), so we decode the body into a
// map[string]any and read params["reply_to_message_id"]. Per
// submodules/telebot/options.go:178, embedSendOptions writes the field as
// strconv.Itoa(opt.ReplyTo.ID) into the params map before the JSON encode
// step — so the on-wire value is the string "42", not the integer 42.
//
// Mocking the URL builder or asserting the SendOptions struct would not
// catch a regression in the telebot integration; the body-bytes assertion
// does.
func TestSendReplyEmitsReplyToMessageID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			// telebot.NewBot calls getMe synchronously; respond with a
			// minimal valid bot User payload so NewBot returns nil err.
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"TestBot"}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			// telebot encodes sendMessage params as JSON
			// (Content-Type: application/json) per bot_raw.go:22-52. Read
			// the raw body and decode — DO NOT rely on r.ParseForm() /
			// r.FormValue (those would silently return "" for a JSON
			// body, manufacturing a false PASS).
			body, _ := io.ReadAll(r.Body)
			defer r.Body.Close()
			var params map[string]any
			if err := json.Unmarshal(body, &params); err == nil {
				if v, ok := params["reply_to_message_id"]; ok {
					switch x := v.(type) {
					case string:
						got = x
					case float64:
						// JSON numerics decode to float64. embedSendOptions
						// writes the value with strconv.Itoa (options.go:179)
						// so the type is string in practice; this branch is
						// defensive against a telebot wire-format change.
						got = strconv.FormatInt(int64(x), 10)
					}
				}
			}
			// Canned sendMessage response with message_id=777 so the
			// caller can assert the returned ID.
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777,"chat":{"id":12345},"text":"hi"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	msgID, err := a.SendReply(context.Background(), 12345, "hi", 42, nil)
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if msgID != 777 {
		t.Fatalf("msgID: got %d want 777", msgID)
	}
	if got != "42" {
		t.Fatalf("reply_to_message_id: got %q want %q", got, "42")
	}
}

// recordedCall captures a single intercepted Bot API request — URL path
// (so the test can assert /sendMessage vs /sendPhoto vs /sendDocument /
// /sendVoice / /sendAudio routing), Content-Type header (so multipart
// uploads can be distinguished from JSON sendMessage bodies), and a
// short snippet of the body (so the file-part presence in multipart
// uploads can be asserted without buffering megabytes).
type recordedCall struct {
	Path        string
	ContentType string
	BodySnippet string // first 4 KiB of the request body
}

// newBotAPIStub returns an httptest.Server impersonating the Telegram
// Bot API + a slice pointer the test can read after the SendReply
// returns. It handles:
//   - /getMe   → telebot.NewBot synchronous probe.
//   - /sendMessage → text reply (JSON body, application/json).
//   - /sendPhoto / /sendDocument / /sendVoice / /sendAudio / /sendVideo →
//     attachment fan-out (multipart/form-data body).
//
// Canned responses include the nested photo/document/voice/audio/video
// object so telebot's *p = *msg.Photo / similar stealRef assignments
// (sendable.go:38-42 etc.) don't NPE on the response. Per
// submodules/telebot/media.go:120-142 Photo.UnmarshalJSON accepts both
// JSON object and JSON array — we return the array form to exercise the
// custom unmarshal path (matches what the real Bot API does).
func newBotAPIStub(t *testing.T) (*httptest.Server, *[]recordedCall) {
	t.Helper()
	var (
		mu    sync.Mutex
		calls []recordedCall
	)
	record := func(r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		snippet := body
		if len(snippet) > 4096 {
			snippet = snippet[:4096]
		}
		mu.Lock()
		calls = append(calls, recordedCall{
			Path:        r.URL.Path,
			ContentType: r.Header.Get("Content-Type"),
			BodySnippet: string(snippet),
		})
		mu.Unlock()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			// Do not record getMe — it's the telebot ctor probe, not a
			// SendReply call.
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"TestBot"}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			record(r)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777,"chat":{"id":12345},"text":"hi"}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			record(r)
			// photo as a JSON array (the Bot API real shape — a list of
			// PhotoSize objects). Photo.UnmarshalJSON picks the hi-res
			// entry (media.go:128-134).
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":778,"chat":{"id":12345},"photo":[{"file_id":"FP","file_unique_id":"FPU","width":1,"height":1,"file_size":1}]}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendDocument"):
			record(r)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":779,"chat":{"id":12345},"document":{"file_id":"FD","file_unique_id":"FDU","file_name":"x","mime_type":"application/pdf","file_size":1}}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendVoice"):
			record(r)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":780,"chat":{"id":12345},"voice":{"file_id":"FV","file_unique_id":"FVU","duration":1,"mime_type":"audio/ogg","file_size":1}}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendAudio"):
			record(r)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":781,"chat":{"id":12345},"audio":{"file_id":"FA","file_unique_id":"FAU","duration":1,"mime_type":"audio/mpeg","file_size":1}}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendVideo"):
			record(r)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":782,"chat":{"id":12345},"video":{"file_id":"FVI","file_unique_id":"FVIU","width":1,"height":1,"duration":1,"file_size":1}}}`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// writeTempFile creates a temp file under t.TempDir() with the given
// content, returns the absolute path. The path is what
// commons.Attachment.Filename carries on the Wave 6 inbound side
// (subscribe.go's buildEventWithAttachment writes the on-disk
// content-addressed path); Wave 6.5 T6's outbound fan-out reads from
// the same field.
func writeTempFile(t *testing.T, suffix, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "att"+suffix)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	return p
}

// TestSendReply_NilAttachments_BackwardCompat pins Wave 6.5 T6's
// backward-compatibility contract: passing nil for attachments MUST
// behave identically to Wave 6 (text-only reply, single /sendMessage
// call, no extra wire traffic). A regression that issues a phantom
// sendDocument for nil would fail here.
func TestSendReply_NilAttachments_BackwardCompat(t *testing.T) {
	srv, calls := newBotAPIStub(t)
	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	msgID, err := a.SendReply(context.Background(), 12345, "hi", 99, nil)
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if msgID != 777 {
		t.Fatalf("msgID: got %d want 777", msgID)
	}
	if got := len(*calls); got != 1 {
		t.Fatalf("expected 1 wire call (text only); got %d: %+v", got, *calls)
	}
	if !strings.HasSuffix((*calls)[0].Path, "/sendMessage") {
		t.Errorf("expected /sendMessage; got: %s", (*calls)[0].Path)
	}
}

// TestSendReply_OnePhoto_TwoCalls is the canonical §107 attachment
// fan-out anchor: passing one image/png attachment MUST produce exactly
// 2 wire calls (1 sendMessage + 1 sendPhoto), the sendPhoto MUST be
// multipart/form-data (proves the file is actually uploaded — a stub
// that just sends a metadata-only JSON would fail here). Wave 6.5 T11
// mutation gate M5 plants a SendReply that skips the attachment loop;
// this assertion catches it.
func TestSendReply_OnePhoto_TwoCalls(t *testing.T) {
	srv, calls := newBotAPIStub(t)
	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	// 4-byte PNG signature header is enough for the multipart upload to
	// carry real bytes; the stub doesn't validate the image — we only
	// assert the multipart envelope is present in the wire body.
	p := writeTempFile(t, ".png", "\x89PNG")
	atts := []commons.Attachment{{Filename: p, MIMEType: "image/png", SizeBytes: 4}}
	msgID, err := a.SendReply(context.Background(), 12345, "see pic", 99, atts)
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if msgID != 777 {
		t.Fatalf("msgID: got %d want 777 (text reply ID, not attachment ID)", msgID)
	}
	if got := len(*calls); got != 2 {
		t.Fatalf("expected 2 wire calls (text + sendPhoto); got %d: %+v", got, *calls)
	}
	if !strings.HasSuffix((*calls)[0].Path, "/sendMessage") {
		t.Errorf("call[0] path: got %q want suffix /sendMessage", (*calls)[0].Path)
	}
	if !strings.HasSuffix((*calls)[1].Path, "/sendPhoto") {
		t.Errorf("call[1] path: got %q want suffix /sendPhoto", (*calls)[1].Path)
	}
	if !strings.HasPrefix((*calls)[1].ContentType, "multipart/form-data") {
		t.Errorf("call[1] Content-Type: got %q want prefix multipart/form-data (proves real file upload, not metadata-only)", (*calls)[1].ContentType)
	}
}

// TestSendReply_MultipleAttachments_RoutedByMIME exercises the MIME
// routing table in buildTelebotMedia. One image + one PDF + one Ogg
// voice MUST produce 4 wire calls (1 sendMessage + 1 sendPhoto + 1
// sendDocument + 1 sendVoice), each multipart, each with the expected
// URL path. A regression that routed everything through sendDocument
// (the safe fallback) would fail the per-call path assertions.
func TestSendReply_MultipleAttachments_RoutedByMIME(t *testing.T) {
	srv, calls := newBotAPIStub(t)
	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	pPNG := writeTempFile(t, ".png", "\x89PNG")
	pPDF := writeTempFile(t, ".pdf", "%PDF")
	pOGG := writeTempFile(t, ".ogg", "OggS")
	atts := []commons.Attachment{
		{Filename: pPNG, MIMEType: "image/png", SizeBytes: 4},
		{Filename: pPDF, MIMEType: "application/pdf", SizeBytes: 4},
		{Filename: pOGG, MIMEType: "audio/ogg", SizeBytes: 4},
	}
	msgID, err := a.SendReply(context.Background(), 12345, "see all", 99, atts)
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if msgID != 777 {
		t.Fatalf("msgID: got %d want 777", msgID)
	}
	if got := len(*calls); got != 4 {
		t.Fatalf("expected 4 wire calls (text + 3 attachments); got %d: %+v", got, *calls)
	}
	wantSuffix := []string{"/sendMessage", "/sendPhoto", "/sendDocument", "/sendVoice"}
	for i, want := range wantSuffix {
		if !strings.HasSuffix((*calls)[i].Path, want) {
			t.Errorf("call[%d] path: got %q want suffix %q", i, (*calls)[i].Path, want)
		}
		if i == 0 {
			continue // sendMessage is application/json, not multipart
		}
		if !strings.HasPrefix((*calls)[i].ContentType, "multipart/form-data") {
			t.Errorf("call[%d] Content-Type: got %q want prefix multipart/form-data", i, (*calls)[i].ContentType)
		}
	}
}

// TestSendReply_EmptyFilenameRejected pins the explicit-error contract:
// an attachment with no on-disk path is rejected with a clear error
// rather than silently uploading nothing (§107: no silent-degrade). The
// Wave 6 inbound side (subscribe.go buildEventWithAttachment) always
// sets Filename to the ~/.herald/inbox/<sha>.<ext> path, so a missing
// Filename indicates a programming error upstream.
func TestSendReply_EmptyFilenameRejected(t *testing.T) {
	srv, _ := newBotAPIStub(t)
	a := tgram.NewAdapterWithBaseURL("TESTTOKEN", "12345", srv.URL)
	atts := []commons.Attachment{{Filename: "", MIMEType: "image/png"}}
	_, err := a.SendReply(context.Background(), 12345, "hi", 99, atts)
	if err == nil {
		t.Fatal("expected error for empty Filename; got nil")
	}
	if !strings.Contains(err.Error(), "Filename empty") {
		t.Errorf("expected error to mention 'Filename empty'; got: %v", err)
	}
}
