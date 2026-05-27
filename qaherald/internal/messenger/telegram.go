// Telegram impl of MessengerClient.
//
// Design choice (operator-locked, plan T2 Caveat 1): this impl uses a
// SMALL raw HTTP client against the Telegram Bot API directly rather
// than wrapping telebot.v3's LongPoller. Three reasons:
//
//  1. Offset bookkeeping. WaitForReply MUST advance the
//     server-side offset ONLY past matched updates; non-matched
//     updates remain in queue for subsequent calls. telebot's
//     LongPoller commits the offset on every drain, which would
//     consume non-matching updates. Raw getUpdates gives us
//     fine-grained offset control.
//
//  2. Test fidelity. httptest.NewServer impersonating
//     https://api.telegram.org is trivial against a raw HTTP path
//     surface; impersonating telebot's internal poll loop is not.
//
//  3. No telebot quirks. telebot's Bot.Send returns a fully-decoded
//     *tele.Message that the orchestrator has no use for; raw JSON
//     decode straight into our messenger.Reply is leaner.
//
// The existing qaherald/internal/tgram.Client (Wave 5 T3) STAYS in
// place — it serves the outbound Send path for the legacy `qaherald
// run` subcommand. Lifecycle scenarios (T3) construct TelegramClient
// directly. Both clients can coexist against the same bot token; the
// Bot API is stateless.
//
// Security: the token NEVER appears in error messages. Errors mention
// the API method ("sendMessage failed: ...") but never the URL
// (which contains the token) or the token itself. Tests assert error
// text does NOT contain the token sentinel.
//
// §107 anti-bluff: every wire-call is observable on the httptest
// server in telegram_test.go. A no-op Send returning a synthetic
// message_id would FAIL because the test counts hits on
// /sendMessage. A Preflight that fabricates fields would FAIL because
// the test asserts the report's fields against the stub server's
// canned response bytes.
package messenger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// defaultBotAPIBaseURL is the Telegram Bot API production endpoint.
// Tests pass an httptest.Server URL instead via the constructor.
const defaultBotAPIBaseURL = "https://api.telegram.org"

// TelegramClient implements MessengerClient against the Telegram Bot
// API. The struct is intentionally small: a token, a chat-id, an
// HTTP client, and a mutex protecting the cached bot identity.
//
// All fields are unexported; construct via NewTelegramClient.
type TelegramClient struct {
	token   string
	chatID  int64
	baseURL string
	hc      *http.Client

	meMu     sync.Mutex
	meCached *meCacheEntry
}

type meCacheEntry struct {
	username                string
	userID                  int64
	canReadAllGroupMessages bool
}

// NewTelegramClient constructs a TelegramClient.
//
// `token`     — Bot API token; MUST be non-empty.
// `chatID`    — the target group/supergroup; MUST be non-zero.
// `baseURL`   — Bot API root (e.g. "https://api.telegram.org");
//
//	empty string defaults to defaultBotAPIBaseURL. Tests pass an
//	httptest.Server URL so all wire-calls are observable.
//
// The constructor does NOT dispatch any HTTP traffic — that would
// make NewTelegramClient indistinguishable from a wire-side smoke,
// which is the wrong contract for the lifecycle orchestrator
// (preflight is the explicit network step). To prove identity, call
// Me() or Preflight() explicitly.
func NewTelegramClient(token string, chatID int64, baseURL string) (*TelegramClient, error) {
	if token == "" {
		return nil, errors.New("messenger/telegram: token is empty (security: not echoed)")
	}
	if chatID == 0 {
		return nil, errors.New("messenger/telegram: chatID must be non-zero")
	}
	if baseURL == "" {
		baseURL = defaultBotAPIBaseURL
	}
	// Validate baseURL is parseable so we fail loud at construction
	// rather than at first request.
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("messenger/telegram: invalid baseURL: %w", err)
	}
	return &TelegramClient{
		token:   token,
		chatID:  chatID,
		baseURL: baseURL,
		hc:      &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// methodURL builds the canonical Bot API URL for `method`. The URL
// embeds the token — this string MUST NEVER be logged.
func (c *TelegramClient) methodURL(method string) string {
	return c.baseURL + "/bot" + c.token + "/" + method
}

// fileURL builds the canonical Bot API file-download URL for
// `filePath` (returned by getFile). The URL embeds the token —
// MUST NEVER be logged.
func (c *TelegramClient) fileURL(filePath string) string {
	return c.baseURL + "/file/bot" + c.token + "/" + filePath
}

// botAPIResponse is the envelope every Bot API JSON response uses.
// Generic over the Result type via json.RawMessage so each caller can
// decode into the method-specific struct.
type botAPIResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// callJSON dispatches a JSON POST against /bot<token>/<method> with
// the given payload and decodes the response envelope. If ok=false,
// returns an error citing the method + description (but NEVER the
// token or the URL).
func (c *TelegramClient) callJSON(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%s: encode payload: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL(method), bytes.NewReader(body))
	if err != nil {
		// Keep the error generic — http.NewRequestWithContext can
		// embed the URL in its error string for some failure modes.
		return nil, fmt.Errorf("%s: build request", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		// net/http errors can include the URL (with the token). Replace
		// with a method-only error to satisfy the security mandate.
		return nil, fmt.Errorf("%s: request failed", method)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response", method)
	}
	var env botAPIResponse
	if err := json.Unmarshal(respBytes, &env); err != nil {
		return nil, fmt.Errorf("%s: decode response envelope", method)
	}
	if !env.OK {
		return nil, fmt.Errorf("%s failed: %s (error_code=%d)", method, env.Description, env.ErrorCode)
	}
	return env.Result, nil
}

// callMultipart dispatches a multipart POST for the file-upload methods
// (sendPhoto, sendDocument, sendVoice). `fileField` is the form field
// the Bot API expects for the binary (e.g. "photo"). `extra` is a
// map of additional form fields (chat_id, caption, ...).
func (c *TelegramClient) callMultipart(ctx context.Context, method, fileField, path string, extra map[string]string) (json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: open file: %w", method, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range extra {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("%s: write field %s", method, k)
		}
	}
	fw, err := mw.CreateFormFile(fileField, filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("%s: create form file", method)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("%s: copy file bytes", method)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("%s: finalize multipart", method)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL(method), &buf)
	if err != nil {
		return nil, fmt.Errorf("%s: build request", method)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed", method)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response", method)
	}
	var env botAPIResponse
	if err := json.Unmarshal(respBytes, &env); err != nil {
		return nil, fmt.Errorf("%s: decode response envelope", method)
	}
	if !env.OK {
		return nil, fmt.Errorf("%s failed: %s (error_code=%d)", method, env.Description, env.ErrorCode)
	}
	return env.Result, nil
}

// tgMessage is the subset of Telegram's Message struct we decode.
// Fields we don't use are intentionally omitted — keeps the wire
// surface narrow and the test fixtures small.
type tgMessage struct {
	MessageID int     `json:"message_id"`
	From      *tgUser `json:"from,omitempty"`
	Chat      tgChat  `json:"chat"`
	Date      int64   `json:"date"`
	Text      string  `json:"text,omitempty"`
	Caption   string  `json:"caption,omitempty"`
	ReplyTo   *struct {
		MessageID int `json:"message_id"`
	} `json:"reply_to_message,omitempty"`
	Photo []struct {
		FileID   string `json:"file_id"`
		FileSize int64  `json:"file_size,omitempty"`
		Width    int    `json:"width,omitempty"`
		Height   int    `json:"height,omitempty"`
	} `json:"photo,omitempty"`
	Document *struct {
		FileID   string `json:"file_id"`
		FileName string `json:"file_name,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
		FileSize int64  `json:"file_size,omitempty"`
	} `json:"document,omitempty"`
	Voice *struct {
		FileID   string `json:"file_id"`
		MimeType string `json:"mime_type,omitempty"`
		Duration int    `json:"duration,omitempty"`
		FileSize int64  `json:"file_size,omitempty"`
	} `json:"voice,omitempty"`
	Audio *struct {
		FileID   string `json:"file_id"`
		MimeType string `json:"mime_type,omitempty"`
		FileName string `json:"file_name,omitempty"`
		Duration int    `json:"duration,omitempty"`
		FileSize int64  `json:"file_size,omitempty"`
	} `json:"audio,omitempty"`
}

type tgUser struct {
	ID                      int64  `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	Username                string `json:"username,omitempty"`
	FirstName               string `json:"first_name,omitempty"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages,omitempty"`
}

type tgChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message,omitempty"`
}

// toReply converts a wire tgMessage + raw JSON bytes to a
// messenger.Reply. The raw bytes are preserved for transcript
// forensic anchor (§107 — full audit trail).
func toReply(m *tgMessage, raw []byte) Reply {
	r := Reply{
		MessageID: MessageID(strconv.Itoa(m.MessageID)),
		Text:      m.Text,
		Caption:   m.Caption,
		Timestamp: time.Unix(m.Date, 0).UTC(),
		Raw:       raw,
	}
	if m.From != nil {
		r.SenderUsername = m.From.Username
		r.SenderIsBot = m.From.IsBot
	}
	if m.ReplyTo != nil {
		r.ReplyToMessageID = MessageID(strconv.Itoa(m.ReplyTo.MessageID))
	}
	// Photo: pick the LARGEST size variant by file_size — matches Wave 5
	// tgram.Client.Upload convention (highest-fidelity round-trip).
	if len(m.Photo) > 0 {
		biggest := m.Photo[0]
		for _, p := range m.Photo[1:] {
			if p.FileSize > biggest.FileSize {
				biggest = p
			}
		}
		r.Attachments = append(r.Attachments, Attachment{
			FileID:      biggest.FileID,
			ContentType: "image/jpeg",
			SizeBytes:   biggest.FileSize,
			Kind:        AttachmentPhoto,
		})
	}
	if m.Document != nil {
		ct := m.Document.MimeType
		if ct == "" {
			ct = "application/octet-stream"
		}
		r.Attachments = append(r.Attachments, Attachment{
			FileID:       m.Document.FileID,
			ContentType:  ct,
			SizeBytes:    m.Document.FileSize,
			OriginalName: m.Document.FileName,
			Kind:         AttachmentDocument,
		})
	}
	if m.Voice != nil {
		ct := m.Voice.MimeType
		if ct == "" {
			ct = "audio/ogg"
		}
		r.Attachments = append(r.Attachments, Attachment{
			FileID:      m.Voice.FileID,
			ContentType: ct,
			SizeBytes:   m.Voice.FileSize,
			Kind:        AttachmentVoice,
		})
	}
	if m.Audio != nil {
		ct := m.Audio.MimeType
		if ct == "" {
			ct = "audio/mpeg"
		}
		r.Attachments = append(r.Attachments, Attachment{
			FileID:       m.Audio.FileID,
			ContentType:  ct,
			SizeBytes:    m.Audio.FileSize,
			OriginalName: m.Audio.FileName,
			Kind:         AttachmentAudio,
		})
	}
	return r
}

// Me dispatches getMe and caches the result. The cache is invalidated
// on Close (which is a no-op for this stateless client — but we
// document the contract).
func (c *TelegramClient) Me(ctx context.Context) (string, int64, error) {
	c.meMu.Lock()
	defer c.meMu.Unlock()
	if c.meCached != nil {
		return c.meCached.username, c.meCached.userID, nil
	}
	raw, err := c.callJSON(ctx, "getMe", struct{}{})
	if err != nil {
		return "", 0, err
	}
	var u tgUser
	if err := json.Unmarshal(raw, &u); err != nil {
		return "", 0, fmt.Errorf("getMe: decode result")
	}
	if u.Username == "" {
		return "", 0, fmt.Errorf("getMe: %w (empty username)", ErrEmptyResponse)
	}
	c.meCached = &meCacheEntry{
		username:                u.Username,
		userID:                  u.ID,
		canReadAllGroupMessages: u.CanReadAllGroupMessages,
	}
	return u.Username, u.ID, nil
}

// Send delivers a text message into the configured chat.
func (c *TelegramClient) Send(ctx context.Context, text string) (MessageID, error) {
	payload := map[string]any{
		"chat_id": c.chatID,
		"text":    text,
	}
	raw, err := c.callJSON(ctx, "sendMessage", payload)
	if err != nil {
		return "", err
	}
	var m tgMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("sendMessage: decode result")
	}
	if m.MessageID == 0 {
		return "", fmt.Errorf("sendMessage: %w (message_id=0)", ErrEmptyResponse)
	}
	return MessageID(strconv.Itoa(m.MessageID)), nil
}

// SendPhoto uploads a photo with optional caption via multipart.
func (c *TelegramClient) SendPhoto(ctx context.Context, path string, caption string) (MessageID, error) {
	extra := map[string]string{"chat_id": strconv.FormatInt(c.chatID, 10)}
	if caption != "" {
		extra["caption"] = caption
	}
	raw, err := c.callMultipart(ctx, "sendPhoto", "photo", path, extra)
	if err != nil {
		return "", err
	}
	var m tgMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("sendPhoto: decode result")
	}
	if m.MessageID == 0 {
		return "", fmt.Errorf("sendPhoto: %w (message_id=0)", ErrEmptyResponse)
	}
	return MessageID(strconv.Itoa(m.MessageID)), nil
}

// SendDocument uploads a non-photo binary with optional caption.
func (c *TelegramClient) SendDocument(ctx context.Context, path string, caption string) (MessageID, error) {
	extra := map[string]string{"chat_id": strconv.FormatInt(c.chatID, 10)}
	if caption != "" {
		extra["caption"] = caption
	}
	raw, err := c.callMultipart(ctx, "sendDocument", "document", path, extra)
	if err != nil {
		return "", err
	}
	var m tgMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("sendDocument: decode result")
	}
	if m.MessageID == 0 {
		return "", fmt.Errorf("sendDocument: %w (message_id=0)", ErrEmptyResponse)
	}
	return MessageID(strconv.Itoa(m.MessageID)), nil
}

// SendVoice uploads an ogg/opus voice attachment.
func (c *TelegramClient) SendVoice(ctx context.Context, path string) (MessageID, error) {
	extra := map[string]string{"chat_id": strconv.FormatInt(c.chatID, 10)}
	raw, err := c.callMultipart(ctx, "sendVoice", "voice", path, extra)
	if err != nil {
		return "", err
	}
	var m tgMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("sendVoice: decode result")
	}
	if m.MessageID == 0 {
		return "", fmt.Errorf("sendVoice: %w (message_id=0)", ErrEmptyResponse)
	}
	return MessageID(strconv.Itoa(m.MessageID)), nil
}

// GetUpdates dispatches /getUpdates with offset and returns the
// decoded Replies plus the highest update_id seen (callers pass
// highWater+1 on their next call to commit). The caller MUST NOT pass
// `offset` from a partial-drain — that would discard non-matching
// updates and break the WaitForReply contract.
func (c *TelegramClient) GetUpdates(ctx context.Context, offset int64) ([]Reply, int64, error) {
	payload := map[string]any{
		"offset":  offset,
		"timeout": 0, // short-poll; WaitForReply loops at scenario cadence
	}
	raw, err := c.callJSON(ctx, "getUpdates", payload)
	if err != nil {
		return nil, offset, err
	}
	var updates []tgUpdate
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, offset, fmt.Errorf("getUpdates: decode result")
	}
	out := make([]Reply, 0, len(updates))
	highWater := offset
	for _, u := range updates {
		if u.UpdateID > highWater {
			highWater = u.UpdateID
		}
		if u.Message == nil {
			continue
		}
		// re-marshal the message back to its raw JSON for forensic
		// anchor; cheap because each message is tiny.
		raw, _ := json.Marshal(u)
		out = append(out, toReply(u.Message, raw))
	}
	return out, highWater, nil
}

// WaitForReply polls getUpdates until pred is satisfied or timeout
// expires. The offset starts at 0 (drains backlog) and advances ONLY
// past the matched update. Non-matching updates leave the offset at
// match-offset_baseline so a subsequent caller observes them.
//
// Contract: this implementation tracks an INTERNAL offset that is
// advanced only past matched updates. The internal offset is reset
// between calls — non-matching updates from THIS call are visible to
// the NEXT call. (This is the documented "predicate matching MUST NOT
// consume non-matching updates" invariant from the plan.)
//
// Implementation: getUpdates(offset=lastConfirmed). For each update,
// if pred → return + record (matched.update_id+1) as the new
// lastConfirmed (commit). If pred fails → DO NOT commit; the next
// getUpdates with offset=lastConfirmed will return the same update
// again. We poll at 250ms cadence until ctx expires.
func (c *TelegramClient) WaitForReply(ctx context.Context, _ MessageID, pred Predicate, timeout time.Duration) (Reply, error) {
	if pred == nil {
		return Reply{}, errors.New("WaitForReply: nil predicate")
	}
	deadline := time.Now().Add(timeout)
	wctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// lastConfirmed is the offset we've COMMITTED past — i.e. updates
	// with update_id <= lastConfirmed are gone from the server. Start
	// at 0 to drain any backlog.
	var lastConfirmed int64

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		// fetch one batch.
		updates, _, err := c.GetUpdates(wctx, lastConfirmed)
		if err != nil {
			// If the context expired during the call, surface it.
			if errors.Is(wctx.Err(), context.DeadlineExceeded) {
				return Reply{}, context.DeadlineExceeded
			}
			return Reply{}, err
		}
		for _, r := range updates {
			if pred(r) {
				// Commit past this update so the next WaitForReply (in
				// a later scenario) doesn't see it. We have to decode
				// the update_id from r.Raw to advance the offset
				// correctly.
				var u tgUpdate
				if jerr := json.Unmarshal(r.Raw, &u); jerr == nil {
					_, _, _ = c.GetUpdates(wctx, u.UpdateID+1)
				}
				return r, nil
			}
		}
		// No match this batch — wait a tick, do NOT commit lastConfirmed.
		select {
		case <-wctx.Done():
			return Reply{}, context.DeadlineExceeded
		case <-ticker.C:
		}
	}
}

// Download fetches an attachment by file-id via the two-step
// getFile + /file/bot<token>/<file_path> dance.
func (c *TelegramClient) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	raw, err := c.callJSON(ctx, "getFile", map[string]any{"file_id": fileID})
	if err != nil {
		return nil, err
	}
	var f struct {
		FileID   string `json:"file_id"`
		FilePath string `json:"file_path"`
		FileSize int64  `json:"file_size,omitempty"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("getFile: decode result")
	}
	if f.FilePath == "" {
		return nil, fmt.Errorf("getFile: %w (empty file_path)", ErrEmptyResponse)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fileURL(f.FilePath), nil)
	if err != nil {
		return nil, fmt.Errorf("getFile: build download request")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getFile: download request failed")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("getFile: download HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Preflight runs the structured self-check. Sequence:
//
//	getMe                          → username, user_id, can_read_all_group_messages
//	getChat(expectedChatID)        → ChatType + InChat (bot is a member iff getChat succeeds)
//	getChatAdministrators(chatID)  → PheraldBotPresent (best-effort; if the
//	                                  bot is non-admin the API errors and we
//	                                  set PheraldBotPresent=false WITHOUT
//	                                  failing preflight — pherald-bot may
//	                                  legitimately be a non-admin member)
func (c *TelegramClient) Preflight(ctx context.Context, expectedChatID int64) (PreflightReport, error) {
	var report PreflightReport

	// getMe
	raw, err := c.callJSON(ctx, "getMe", struct{}{})
	if err != nil {
		return report, err
	}
	var u tgUser
	if err := json.Unmarshal(raw, &u); err != nil {
		return report, fmt.Errorf("getMe: decode result")
	}
	report.Username = u.Username
	report.UserID = u.ID
	report.CanReadAllGroupMessages = u.CanReadAllGroupMessages

	// also seed the Me cache so a subsequent Me() doesn't double the
	// roundtrip.
	c.meMu.Lock()
	c.meCached = &meCacheEntry{
		username:                u.Username,
		userID:                  u.ID,
		canReadAllGroupMessages: u.CanReadAllGroupMessages,
	}
	c.meMu.Unlock()

	// getChat
	raw, err = c.callJSON(ctx, "getChat", map[string]any{"chat_id": expectedChatID})
	if err != nil {
		// Bot not in chat is a structured non-fatal — record and
		// return; the lifecycle preflight validator (T6) decides
		// whether to abort.
		report.InChat = false
		return report, nil
	}
	var chat tgChat
	if err := json.Unmarshal(raw, &chat); err == nil {
		report.InChat = true
		report.ChatType = chat.Type
	}

	// PheraldBotPresent — best-effort; ignore errors.
	raw, err = c.callJSON(ctx, "getChatAdministrators", map[string]any{"chat_id": expectedChatID})
	if err == nil {
		var admins []struct {
			User *tgUser `json:"user"`
		}
		if jerr := json.Unmarshal(raw, &admins); jerr == nil {
			for _, a := range admins {
				if a.User != nil && a.User.IsBot && a.User.Username != "" && a.User.Username != u.Username {
					// Any OTHER bot in the admin list is treated as
					// "pherald-bot present" — the lifecycle preflight
					// validator narrows this with the configured
					// pherald-bot-username.
					report.PheraldBotPresent = true
					break
				}
			}
		}
	}
	return report, nil
}

// GetChatMember dispatches getChatMember and returns the membership
// status string ("creator"/"administrator"/"member"/"restricted"/
// "left"/"kicked"). Unlike getChatAdministrators, this works for ANY
// member — including non-admin regular members like pherald-bot — so
// it is the REAL anti-bluff membership proof preflight G1 uses when a
// user-id is supplied.
//
// On API error returns ("", err). The token never appears in the
// error (callJSON enforces this).
func (c *TelegramClient) GetChatMember(ctx context.Context, chatID, userID int64) (string, error) {
	raw, err := c.callJSON(ctx, "getChatMember", map[string]any{
		"chat_id": chatID,
		"user_id": userID,
	})
	if err != nil {
		return "", err
	}
	var member struct {
		Status string `json:"status"`
	}
	if jerr := json.Unmarshal(raw, &member); jerr != nil {
		return "", fmt.Errorf("getChatMember: decode result")
	}
	if member.Status == "" {
		return "", fmt.Errorf("getChatMember: %w (empty status)", ErrEmptyResponse)
	}
	return member.Status, nil
}

// Close is a no-op for the raw HTTP client (no goroutines, no
// connections to drain — http.Client manages its own pool). Provided
// for interface compliance.
func (c *TelegramClient) Close() error { return nil }

// Compile-time check: TelegramClient satisfies MessengerClient.
var _ MessengerClient = (*TelegramClient)(nil)
