// Webhook validator skeleton for the Telegram Bot API inbound path (HRD-136).
//
// Bot API §6 (https://core.telegram.org/bots/api#setwebhook) lets the operator
// hand Telegram a HTTPS URL + an opaque `secret_token` string. Telegram then
// stamps every POST hit with the header:
//
//	X-Telegram-Bot-Api-Secret-Token: <secret_token>
//
// Validating that header on EVERY request is mandatory — without it, anyone
// who guesses the webhook URL can impersonate Telegram and inject forged
// Update payloads into the bot. The compare MUST be constant-time
// (`subtle.ConstantTimeCompare`) to deny timing-oracle attackers a path to
// recover the secret one byte at a time.
//
// Status: VALIDATOR-SKELETON ONLY (HRD-136). Wave 6 ships the getUpdates
// long-poll path (pherald listen); the webhook receiver is the planned
// alternative ingress for operators who prefer a public HTTPS endpoint over
// outbound long-polling. This file lands the cryptographic-validation layer
// + handler glue so the future `pherald listen --webhook` wiring (V1.x) is
// pure plumbing: it constructs a NewWebhookHandler, registers it on its
// http.ServeMux, and the §107 anti-bluff path is already proven by the
// hermetic httptest suite alongside.
//
// What this file is NOT: a full pherald-listen webhook integration. It does
// not start an HTTP listener, mint TLS certificates, register the URL with
// the Bot API via setWebhook, or hook into the existing subscribe.go
// attachment-download / bot-self-filter pipeline. Those land in V1.x.
package tgram

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/vasic-digital/herald/commons"
	telebot "gopkg.in/telebot.v3"
)

// telegramSecretHeader is the canonical header Bot API stamps on every
// webhook POST when setWebhook(secret_token=...) was used at registration.
// Per the Bot API contract the value is 1..256 chars from [A-Za-z0-9_-].
const telegramSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

// okResponseBody is the JSON body Bot API conventionally expects from a
// webhook receiver — a 200 with `{"ok": true}` signals successful handoff.
var okResponseBody = []byte(`{"ok":true}`)

// WebhookHandler is the http.Handler that validates the Bot API secret_token
// header and dispatches decoded Updates to an inbound handler. It is the
// validator-skeleton peer of Adapter (subscribe.go / send.go) — both feed
// the same commons.InboundHandler boundary, but via different ingress
// shapes (long-poll vs. webhook).
//
// Construct via NewWebhookHandler; the zero value is NOT safe to use
// (secret is empty + handler is nil).
type WebhookHandler struct {
	// secret is the configured secret_token expected on every POST. Stored
	// as []byte to keep the constant-time compare path obvious — the
	// subtle.ConstantTimeCompare contract operates on equal-length byte
	// slices and we want zero implicit string→bytes conversions on the
	// hot path.
	secret []byte

	// handler receives the decoded InboundEvent. Errors returned from
	// Handle are logged + surfaced as 500 to Telegram (which will retry).
	handler commons.InboundHandler

	// logger is the failure sink. We intentionally NEVER write decode or
	// handler errors into the HTTP response body — leaking that detail to
	// the network gives an attacker a free observability channel into the
	// inbound pipeline. All such detail goes to logger only.
	logger *log.Logger
}

// NewWebhookHandler builds a validating webhook receiver bound to the given
// secret + InboundHandler.
//
// secret MUST be non-empty — an empty secret means "accept any caller as
// Telegram", which is the §107 footgun this whole file exists to prevent.
// We surface that as a loud constructor error rather than a silent runtime
// 401-everything (which would look like a different bug).
//
// handler MUST be non-nil — a webhook receiver with no inbound handler is
// likewise a configuration mistake we refuse to paper over.
//
// logger MAY be nil — defaults to log.Default(). Tests usually pass a
// log.New wrapping a bytes.Buffer to assert against the failure trace.
func NewWebhookHandler(secret string, handler commons.InboundHandler, logger *log.Logger) (*WebhookHandler, error) {
	if secret == "" {
		return nil, errors.New("tgram.NewWebhookHandler: secret must be non-empty (empty secret = accept-any-caller, §107 impersonation hazard)")
	}
	if handler == nil {
		return nil, errors.New("tgram.NewWebhookHandler: handler must be non-nil (receiver with no sink is a configuration bug)")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &WebhookHandler{
		secret:  []byte(secret),
		handler: handler,
		logger:  logger,
	}, nil
}

// ServeHTTP implements http.Handler. The flow is:
//
//  1. POST-only (405 on anything else). Telegram only ever issues POST; any
//     other method is either an operator probe or an attacker fingerprint
//     attempt — reject both at the door.
//  2. Constant-time-compare the X-Telegram-Bot-Api-Secret-Token header
//     against the configured secret. ANY mismatch — wrong value, wrong
//     length, missing header — returns 401 with NO body detail. We give
//     attackers zero oracle bits: same status, same body, no Content-Type
//     differentiation, no logged hint to a network observer.
//  3. Decode the JSON body into telebot.Update. Malformed body returns 400.
//  4. Convert Update → InboundEvent and call the configured handler. A
//     handler error returns 500 (Telegram will retry) but the error
//     message is logged only — never echoed to the response.
//  5. Success returns 200 with body `{"ok":true}` per Bot API convention.
//
// The constant-time-compare is the load-bearing crypto: a naïve `==` short-
// circuits on the first mismatched byte, leaking secret prefix length one
// timing-measurable byte at a time. subtle.ConstantTimeCompare(equal-length
// inputs) takes O(len) time regardless of where the bytes differ. We feed
// it equal-length inputs by checking len() first AND returning the same
// 401 path for length mismatch, so an attacker who flips between
// equal-length-different-content and short-content sees only "401, no
// body" in both cases — no observable distinction to exploit.
func (w *WebhookHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// 405 carries no body — same shape as 401 below — to keep the
		// attacker-observable surface uniform.
		rw.Header().Set("Allow", http.MethodPost)
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Pull the header. A missing header is the equivalent of an
	// empty-string presented token — both flunk the constant-time compare
	// against a non-empty configured secret, both yield 401 with no
	// distinguishing body. We do NOT short-circuit on "missing" with a
	// different code path — keeping the codepath uniform is what denies
	// the timing oracle.
	presented := r.Header.Get(telegramSecretHeader)

	// We DELIBERATELY compare equal-length-or-401-uniform. If the
	// presented header is the wrong length we still pay the constant-time
	// cost against a same-length scratch buffer to keep the timing
	// profile flat. subtle.ConstantTimeCompare returns 0 when the
	// lengths differ, so we route the same-length and different-length
	// paths through one comparator + one return.
	if !secretMatches(w.secret, []byte(presented)) {
		// 401 with EMPTY body — no JSON, no Content-Type, nothing for an
		// attacker to grep against. The configured secret is NEVER
		// referenced in the response (no echo-back, no length hint).
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	// At this point the caller has presented the correct secret. Decode
	// the Update — a malformed body from a "trusted" caller is still a
	// 400 (Telegram itself never sends malformed JSON, but a misconfigured
	// reverse proxy or a replay-test harness might).
	var update telebot.Update
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // belt-and-braces: an Update with garbage
	// fields is most likely a misconfigured client, not a Bot API drift.
	// If a future Bot API field needs accepting we'll relax this
	// deliberately, but defaulting strict denies the body-padding-as-
	// covert-channel trick.
	if err := dec.Decode(&update); err != nil {
		w.logger.Printf("tgram.WebhookHandler: body decode failed: %v", err)
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	// Convert to InboundEvent and dispatch.
	ev, ok := updateToInboundEvent(&update)
	if !ok {
		// An Update with no recognised inner payload (no Message, no
		// EditedMessage, no Callback, ...) is unusual but not erroneous —
		// Bot API sends fully-empty heartbeat-ish updates in edge cases.
		// Acknowledge with 200 + ok:true so Telegram doesn't retry.
		writeOK(rw, w.logger)
		return
	}

	if err := w.handler.Handle(r.Context(), ev); err != nil {
		// Log full detail, but DO NOT echo the error to the response —
		// leaking handler errors out the network gives an attacker a
		// free signal channel ("which sub-handler did my forged Update
		// reach?"). Telegram's retry path interprets 500 as "try again";
		// that's the correct semantics for a transient backend hiccup.
		w.logger.Printf("tgram.WebhookHandler: inbound handler error: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeOK(rw, w.logger)
}

// secretMatches is the constant-time comparator. It first checks that the
// two byte slices are the same length (subtle.ConstantTimeCompare returns
// 0 on length mismatch, but we want a single comparator codepath that
// always pays the same-length-compare cost on the configured secret to
// keep the timing flat). When the presented length differs we still
// compare against a same-length zero-filled scratch to pay the same
// cycles before returning false.
//
// IMPORTANT: this function MUST be kept free of `==` short-circuits on
// secret content. Any future modification that introduces an early-return
// based on byte content reintroduces the timing-oracle attack — that's
// what TestWebhook_ConstantTimeCompare exists to catch.
func secretMatches(configured, presented []byte) bool {
	if len(configured) == len(presented) {
		// Equal length — direct constant-time compare. Result == 1 iff
		// every byte matches; result == 0 on any mismatch.
		return subtle.ConstantTimeCompare(configured, presented) == 1
	}
	// Different length: pay an equivalent-length compare against a
	// zero-buffer of the configured length so the timing-profile of
	// "wrong length" and "right length, wrong content" stay similar.
	// The result is irrelevant — different-length is always a fail.
	scratch := make([]byte, len(configured))
	_ = subtle.ConstantTimeCompare(configured, scratch)
	return false
}

// writeOK writes the conventional Bot API webhook ack — 200 with body
// `{"ok": true}`. The Bot API does not require this exact shape (any 2xx
// is accepted), but matching the convention keeps the response shape
// identical to api.telegram.org's own responses, which simplifies
// debugging when an operator wireshark-traces the path.
func writeOK(rw http.ResponseWriter, logger *log.Logger) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(okResponseBody); err != nil {
		// Body-write failure here means the network already collapsed.
		// Log + move on — there's no useful recovery.
		logger.Printf("tgram.WebhookHandler: writing ok body failed: %v", err)
	}
}

// updateToInboundEvent translates a telebot.Update into a commons.InboundEvent
// using the same field-shape as subscribe.go's OnText handler — Sender from
// Chat.ID, Body.Plain from Message.Text, Raw populated with message_id /
// chat_id / message_thread_id / text. Returns ok=false when the Update has
// no inner Message we can address (e.g. a callback_query only — those will
// flow through a separate Callback-path in V1.x). Keeping the conversion in
// one helper makes it trivial to keep the long-poll and webhook ingress
// shapes identical at the InboundEvent boundary.
func updateToInboundEvent(u *telebot.Update) (commons.InboundEvent, bool) {
	if u == nil {
		return commons.InboundEvent{}, false
	}
	// Prefer Message, then EditedMessage, then ChannelPost — the same
	// precedence telebot's own router uses internally.
	msg := u.Message
	if msg == nil {
		msg = u.EditedMessage
	}
	if msg == nil {
		msg = u.ChannelPost
	}
	if msg == nil {
		return commons.InboundEvent{}, false
	}
	ev := commons.InboundEvent{
		Sender: commons.Recipient{
			Channel:       string(commons.ChannelTelegram),
			ChannelUserID: strconv.FormatInt(msg.Chat.ID, 10),
		},
		Body: commons.Body{
			Plain: msg.Text,
		},
		Raw: map[string]any{
			"update_id":         u.ID,
			"message_id":        msg.ID,
			"chat_id":           msg.Chat.ID,
			"message_thread_id": msg.ThreadID,
			"text":              msg.Text,
		},
	}
	if msg.ThreadID != 0 {
		ev.Thread = &commons.ConversationRef{
			Channel:  commons.ChannelTelegram,
			ThreadID: strconv.Itoa(msg.ThreadID),
		}
	}
	return ev, true
}

// HeaderName returns the canonical header Bot API uses to stamp the
// secret_token on every webhook POST. Exposed for tests + future
// `pherald listen --webhook` wiring that may want to log the header name
// without re-deriving it from the spec.
func HeaderName() string { return telegramSecretHeader }

// compile-time check: *WebhookHandler implements http.Handler.
var _ http.Handler = (*WebhookHandler)(nil)

// fmtForwardCompat is a placeholder reference to the fmt import — kept here
// only so future error-wrapping additions (e.g. wrapping handler errors with
// %w for a non-leaky log line) don't require touching the import block.
// Removed in a follow-up if still unused at V1.x landing.
var _ = fmt.Sprintf
