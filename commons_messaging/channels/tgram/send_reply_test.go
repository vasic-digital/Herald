package tgram_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
