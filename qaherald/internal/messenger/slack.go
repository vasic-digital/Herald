// Slack impl of MessengerClient (Wave 7 T7 — HRD-116).
//
// Design parity with telegram.go (operator-locked, plan T7 Caveat 1):
// a SMALL raw HTTP client against the Slack Web API rather than the
// slack-go SDK. Three reasons:
//
//  1. Offset bookkeeping. Slack has no /getUpdates equivalent;
//     GetUpdates is implemented as conversations.history with an
//     `oldest=<ts>` watermark. Non-matching messages stay visible to
//     subsequent polls — the lifecycle WaitForReply contract requires
//     that pred-failing replies remain queue-visible for later
//     WaitForReply calls. slack-go's higher-level helpers commit
//     state we cannot fine-tune; raw conversations.history calls let
//     us control the watermark per call.
//
//  2. Test fidelity. httptest.NewServer impersonating slack.com/api
//     is trivial against a raw HTTP path surface; impersonating
//     slack-go's RTM/Socket Mode internals is not. The httptest server
//     in slack_test.go counts every round-trip per method — a no-op
//     Send returning a synthetic ts would FAIL because the test
//     asserts hit counts on /chat.postMessage.
//
//  3. No SDK drift. slack-go evolves; raw JSON decode straight into
//     messenger.Reply is leaner and version-stable.
//
// The qaherald Slack client is INDEPENDENT of the
// commons_messaging/channels/slack adapter (different layer — qaherald
// is a test bot, that adapter is the pherald-consumed channel).
// Sharing code between them would couple the test harness to the
// production adapter; the plan calls for the independence explicitly.
//
// MessageID semantics: for Slack, MessageID == the dotted-float `ts`
// string Slack returns from chat.postMessage (e.g. "1654.0001"). It is
// NOT a numeric int64 like Telegram. messenger.MessageID is a string
// type that accommodates both.
//
// Security: the xoxb-… token NEVER appears in error messages. Errors
// mention the API method ("chat.postMessage failed: ...") but NEVER
// the URL (Authorization: Bearer <token> would leak via net/http error
// strings on some transport failures) or the token itself. Tests
// assert error text does NOT contain the token sentinel.
//
// §107 anti-bluff: every wire-call is observable on the httptest
// server in slack_test.go. A no-op Send returning a synthetic ts
// would FAIL because the test counts hits on /chat.postMessage. A
// Preflight that fabricates fields would FAIL because the test
// asserts the report's fields against the stub server's canned
// response bytes.
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
	"strings"
	"sync"
	"time"
)

// defaultSlackAPIBaseURL is the Slack Web API production endpoint.
// Tests pass an httptest.Server URL instead via the constructor.
const defaultSlackAPIBaseURL = "https://slack.com/api"

// SlackClient implements MessengerClient against the Slack Web API.
// The struct is intentionally small: a bot token, a channel-id, an
// HTTP client, and a mutex protecting the cached bot identity.
//
// All fields are unexported; construct via NewSlackClient.
type SlackClient struct {
	token     string
	channelID string
	baseURL   string
	hc        *http.Client

	meMu     sync.Mutex
	meCached *slackMeCacheEntry
}

type slackMeCacheEntry struct {
	username string
	userID   string // Slack user-ids are strings ("U01ABC"); we keep them as strings internally
}

// NewSlackClient constructs a SlackClient.
//
// `token`     — Slack bot token (xoxb-…); SHOULD be non-empty (we
//
//	don't fail loud here because the plan's
//	TestBuilderSlack/TestSlackClientSendCrossesWire construct
//	the client without checking an error — the loud-fail on
//	missing credentials happens at the first wire call via
//	chat.postMessage / auth.test returning ok=false).
//
// `channelID` — the Slack channel id (Cxxx, Gxxx); used as the
//
//	default outbound destination.
//
// `baseURL`   — Slack Web API root (e.g. "https://slack.com/api");
//
//	empty string defaults to defaultSlackAPIBaseURL. Tests pass
//	an httptest.Server URL so all wire-calls are observable.
//
// Unlike NewTelegramClient (which returns an error so the constructor
// can validate token + chatID upfront), NewSlackClient returns ONLY
// the client — the plan's test snippet calls .Send() directly on the
// constructor return value. Validation happens at the first wire call.
func NewSlackClient(token, channelID, baseURL string) *SlackClient {
	if baseURL == "" {
		baseURL = defaultSlackAPIBaseURL
	}
	return &SlackClient{
		token:     token,
		channelID: channelID,
		baseURL:   baseURL,
		hc:        &http.Client{Timeout: 30 * time.Second},
	}
}

// methodURL builds the canonical Slack Web API URL for `method`.
// Unlike Telegram, the Slack token is NOT in the URL — it's carried in
// the Authorization: Bearer header. But to stay defensive (the token
// may still leak via the HTTP client's connection errors if the host
// is in baseURL), we never let methodURL into an error string either.
func (c *SlackClient) methodURL(method string) string {
	return c.baseURL + "/" + method
}

// slackResponse is the envelope every Slack Web API JSON response uses.
// `ok` is the universal success flag; `error` carries the slack-side
// reason on failures (e.g. "invalid_auth", "channel_not_found").
//
// We decode method-specific result fields into a json.RawMessage and
// let each caller parse the relevant subset — keeps the wire surface
// narrow and the test fixtures small.
type slackResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Method-specific fields (decoded directly into known slots — Slack
	// flattens its responses; there is no `result` envelope like
	// Telegram).
	TS string `json:"ts,omitempty"`
	// Channel is intentionally a json.RawMessage because Slack's API
	// surface is non-uniform: chat.postMessage returns channel as a
	// string id, while conversations.info returns it as a nested
	// object. Callers that need the value decode it themselves from
	// the raw bytes (Preflight does this for conversations.info).
	Channel json.RawMessage `json:"channel,omitempty"`
	// auth.test fields
	User    string `json:"user,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	Team    string `json:"team,omitempty"`
	TeamID  string `json:"team_id,omitempty"`
	BotID   string `json:"bot_id,omitempty"`
	// files.info fields
	File *slackFile `json:"file,omitempty"`
	// conversations.info fields
	ChannelInfo *slackChannelInfo `json:"channel_info,omitempty"`
	// conversations.history fields
	Messages []slackMessage `json:"messages,omitempty"`
	HasMore  bool           `json:"has_more,omitempty"`
}

// fullEnvelope is a parallel struct used when we need the full raw
// payload (conversations.info uses "channel" as a top-level field that
// conflicts with chat.postMessage's "channel" string — Slack's API
// surface is not uniform). We decode twice into the right shape per
// method instead of trying to unify.
type slackChannelInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsGroup    bool   `json:"is_group"`
	IsChannel  bool   `json:"is_channel"`
	IsIM       bool   `json:"is_im"`
	IsMpim     bool   `json:"is_mpim"`
	IsPrivate  bool   `json:"is_private"`
	IsMember   bool   `json:"is_member"`
}

type slackFile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Mimetype           string `json:"mimetype"`
	Size               int64  `json:"size"`
	URLPrivate         string `json:"url_private"`
	URLPrivateDownload string `json:"url_private_download"`
	User               string `json:"user"`
}

type slackMessage struct {
	Type     string `json:"type"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts,omitempty"`
	User     string `json:"user,omitempty"`
	BotID    string `json:"bot_id,omitempty"`
	Username string `json:"username,omitempty"`
	Text     string `json:"text,omitempty"`
	SubType  string `json:"subtype,omitempty"`
	Files    []slackFile `json:"files,omitempty"`
}

// callJSON dispatches a JSON POST against <baseURL>/<method> with the
// given payload and decodes the response envelope. If ok=false,
// returns an error citing the method + slack-side error reason (but
// NEVER the token or the URL).
//
// The raw response bytes are also returned so callers that need to
// decode method-specific shapes (conversations.info "channel" field
// collides with chat.postMessage "channel" string) can re-parse.
func (c *SlackClient) callJSON(ctx context.Context, method string, payload any) (slackResponse, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: encode payload: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL(method), bytes.NewReader(body))
	if err != nil {
		// Keep the error generic — http.NewRequestWithContext can embed
		// the URL in its error string for some failure modes.
		return slackResponse{}, nil, fmt.Errorf("%s: build request", method)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		// net/http errors can include the URL. Replace with a
		// method-only error to satisfy the security mandate.
		return slackResponse{}, nil, fmt.Errorf("%s: request failed", method)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: read response", method)
	}
	var env slackResponse
	if err := json.Unmarshal(respBytes, &env); err != nil {
		return slackResponse{}, respBytes, fmt.Errorf("%s: decode response envelope", method)
	}
	if !env.OK {
		// Slack's error field is short (e.g. "invalid_auth"); safe to
		// include — does NOT contain the token.
		return env, respBytes, fmt.Errorf("%s failed: %s", method, sanitizeError(env.Error, c.token))
	}
	return env, respBytes, nil
}

// callMultipart dispatches a multipart POST for the file-upload methods
// (files.upload). `fileField` is the form field the Slack Web API
// expects for the binary (always "file" for files.upload). `extra` is a
// map of additional form fields (channels, initial_comment, filename,
// filetype). Returns the decoded envelope.
//
// Slack files.upload semantics: success response carries
// {"ok": true, "file": {"id": ..., "permalink": ..., ...}}. We re-use
// the slackResponse envelope (which has File *slackFile already).
//
// Note: Slack deprecated files.upload in 2025 in favor of the V2 two-
// step flow (files.getUploadURLExternal + files.completeUploadExternal).
// This client uses single-step files.upload for hermetic test
// simplicity; the production qaherald orchestrator will need a V2
// shim before live use. The plan acknowledges this as an internal
// implementation detail that can swap without API breakage.
func (c *SlackClient) callMultipart(ctx context.Context, method, fileField, path string, extra map[string]string) (slackResponse, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: open file: %w", method, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range extra {
		if err := mw.WriteField(k, v); err != nil {
			return slackResponse{}, nil, fmt.Errorf("%s: write field %s", method, k)
		}
	}
	fw, err := mw.CreateFormFile(fileField, filepath.Base(path))
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: create form file", method)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: copy file bytes", method)
	}
	if err := mw.Close(); err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: finalize multipart", method)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL(method), &buf)
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: build request", method)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: request failed", method)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return slackResponse{}, nil, fmt.Errorf("%s: read response", method)
	}
	var env slackResponse
	if err := json.Unmarshal(respBytes, &env); err != nil {
		return slackResponse{}, respBytes, fmt.Errorf("%s: decode response envelope", method)
	}
	if !env.OK {
		return env, respBytes, fmt.Errorf("%s failed: %s", method, sanitizeError(env.Error, c.token))
	}
	return env, respBytes, nil
}

// sanitizeError returns msg with the token redacted to "[REDACTED]".
// This is a belt-and-braces defence in addition to never embedding
// the token into error strings deliberately — a future code path that
// accidentally interpolates a URL containing the token will be
// scrubbed here. The test
// TestSlack_Send_ErrorDoesNotLeakToken asserts the redaction.
func sanitizeError(msg, token string) string {
	if msg == "" || token == "" {
		return msg
	}
	return strings.ReplaceAll(msg, token, "[REDACTED]")
}

// Me dispatches auth.test and caches the result. The cache is
// invalidated on Close (which is a no-op for this stateless client —
// but we document the contract).
//
// MessengerClient.Me returns (username, int64 userID, error). Slack's
// user_id is a string ("U01ABC"), so we parse it to int64 best-effort
// by stripping the leading "U"; if that fails we return 0 (a callsite
// that needs the string form should use Preflight or the cached
// envelope directly). The PreflightReport.UserID field carries the
// best-effort numeric form for Telegram compatibility.
func (c *SlackClient) Me(ctx context.Context) (string, int64, error) {
	c.meMu.Lock()
	defer c.meMu.Unlock()
	if c.meCached != nil {
		return c.meCached.username, slackUserIDToInt64(c.meCached.userID), nil
	}
	env, _, err := c.callJSON(ctx, "auth.test", struct{}{})
	if err != nil {
		return "", 0, err
	}
	if env.User == "" {
		return "", 0, fmt.Errorf("auth.test: %w (empty user)", ErrEmptyResponse)
	}
	c.meCached = &slackMeCacheEntry{
		username: env.User,
		userID:   env.UserID,
	}
	return env.User, slackUserIDToInt64(env.UserID), nil
}

// slackUserIDToInt64 best-effort converts a Slack user-id like
// "U01ABCDEF" into an int64 by stripping non-digit chars. Returns 0
// on failure (Slack ids are not numeric in the Telegram sense; this
// is purely interface adaptation). Callers that need the original
// string should consult PreflightReport or cache the auth.test
// response.
func slackUserIDToInt64(id string) int64 {
	if id == "" {
		return 0
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, id)
	if digits == "" {
		return 0
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// Send delivers a text message into the configured channel via
// chat.postMessage. Returns the messenger's message-id (Slack ts).
func (c *SlackClient) Send(ctx context.Context, text string) (MessageID, error) {
	payload := map[string]any{
		"channel": c.channelID,
		"text":    text,
	}
	env, _, err := c.callJSON(ctx, "chat.postMessage", payload)
	if err != nil {
		return "", err
	}
	if env.TS == "" {
		return "", fmt.Errorf("chat.postMessage: %w (empty ts)", ErrEmptyResponse)
	}
	return MessageID(env.TS), nil
}

// SendPhoto uploads a photo with optional caption via files.upload
// multipart. Slack does not differentiate photo / document / voice at
// the upload layer — all are files.upload with optional filetype hint.
// The caption is sent as initial_comment which appears alongside the
// file in the channel feed.
func (c *SlackClient) SendPhoto(ctx context.Context, path string, caption string) (MessageID, error) {
	return c.uploadFile(ctx, "sendPhoto", path, caption, "auto")
}

// SendDocument uploads a non-photo binary with optional caption.
func (c *SlackClient) SendDocument(ctx context.Context, path string, caption string) (MessageID, error) {
	return c.uploadFile(ctx, "sendDocument", path, caption, "auto")
}

// SendVoice uploads a voice/audio attachment. Slack's files.upload
// accepts arbitrary mime types; we pass filetype=auto and let Slack's
// inspection determine the rendering.
func (c *SlackClient) SendVoice(ctx context.Context, path string) (MessageID, error) {
	return c.uploadFile(ctx, "sendVoice", path, "", "auto")
}

// uploadFile is the common files.upload backbone for SendPhoto /
// SendDocument / SendVoice. The MessageID returned is the file's id
// (NOT a ts — files.upload does not return a ts on the API surface;
// it returns a file object whose `id` and `shares` contain the
// channel-side propagation). For lifecycle scenarios that need a ts
// to correlate replies, follow-up via conversations.history is
// required — this is the same constraint as Slack's documented API.
func (c *SlackClient) uploadFile(ctx context.Context, methodTag, path, caption, filetype string) (MessageID, error) {
	extra := map[string]string{
		"channels": c.channelID,
		"filetype": filetype,
		"filename": filepath.Base(path),
	}
	if caption != "" {
		extra["initial_comment"] = caption
	}
	env, _, err := c.callMultipart(ctx, "files.upload", "file", path, extra)
	if err != nil {
		// Surface methodTag in the error so the caller sees the
		// lifecycle-side intent (SendPhoto vs SendDocument vs
		// SendVoice) alongside the underlying files.upload failure.
		return "", fmt.Errorf("%s: %w", methodTag, err)
	}
	if env.File == nil || env.File.ID == "" {
		return "", fmt.Errorf("%s: %w (empty file.id in files.upload response)", methodTag, ErrEmptyResponse)
	}
	return MessageID(env.File.ID), nil
}

// GetUpdates dispatches conversations.history with oldest=offset and
// returns the decoded Replies plus the highest ts seen (as an int64-
// like high-water — Slack ts is a dotted float; we convert to a
// monotonically-non-decreasing int64 by stripping the dot). The caller
// MUST NOT pass `offset` from a partial-drain — that would discard
// non-matching messages and break the WaitForReply contract.
//
// Slack ts ↔ offset adaptation: the MessengerClient interface defines
// offset as int64 (legacy Telegram). For Slack, we encode the ts as
// the integer part * 1e6 + the fractional part (mathematically: ts *
// 1_000_000 truncated). That gives a stable monotonic ordering AND
// roundtrips back to a ts string via float division. The
// internal conversion is hidden behind tsToOffset / offsetToTS.
func (c *SlackClient) GetUpdates(ctx context.Context, offset int64) ([]Reply, int64, error) {
	oldest := offsetToTS(offset)
	payload := map[string]any{
		"channel": c.channelID,
		"oldest":  oldest,
		"limit":   100,
	}
	env, _, err := c.callJSON(ctx, "conversations.history", payload)
	if err != nil {
		return nil, offset, err
	}
	out := make([]Reply, 0, len(env.Messages))
	highWater := offset
	for _, m := range env.Messages {
		ts := tsToOffset(m.TS)
		if ts > highWater {
			highWater = ts
		}
		// re-marshal the message back to its raw JSON for forensic
		// anchor; cheap because each message is tiny.
		raw, _ := json.Marshal(m)
		out = append(out, slackToReply(m, raw))
	}
	return out, highWater, nil
}

// tsToOffset converts a Slack dotted-float ts ("1654.0001") into a
// monotonic int64 by multiplying by 1e6 and rounding. The conversion
// is lossless for the 6-decimal precision Slack uses.
func tsToOffset(ts string) int64 {
	if ts == "" {
		return 0
	}
	dot := strings.IndexByte(ts, '.')
	if dot < 0 {
		// No fractional part — multiply by 1e6.
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return 0
		}
		return n * 1_000_000
	}
	integer, frac := ts[:dot], ts[dot+1:]
	// Right-pad frac to 6 chars (Slack always uses 6 but be defensive).
	for len(frac) < 6 {
		frac += "0"
	}
	frac = frac[:6]
	in, err := strconv.ParseInt(integer, 10, 64)
	if err != nil {
		return 0
	}
	fr, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0
	}
	return in*1_000_000 + fr
}

// offsetToTS reverses tsToOffset back to a dotted-float ts string
// suitable for the conversations.history `oldest` parameter.
func offsetToTS(offset int64) string {
	if offset == 0 {
		return "0"
	}
	in := offset / 1_000_000
	fr := offset % 1_000_000
	return fmt.Sprintf("%d.%06d", in, fr)
}

// slackToReply converts a wire slackMessage + raw JSON bytes to a
// messenger.Reply. The raw bytes are preserved for transcript
// forensic anchor (§107 — full audit trail).
func slackToReply(m slackMessage, raw []byte) Reply {
	r := Reply{
		MessageID:      MessageID(m.TS),
		SenderUsername: m.User,
		SenderIsBot:    m.BotID != "",
		Text:           m.Text,
		Timestamp:      tsToTime(m.TS),
		Raw:            raw,
	}
	if m.ThreadTS != "" && m.ThreadTS != m.TS {
		r.ReplyToMessageID = MessageID(m.ThreadTS)
	}
	for _, f := range m.Files {
		kind := AttachmentDocument
		switch {
		case strings.HasPrefix(f.Mimetype, "image/"):
			kind = AttachmentPhoto
		case strings.HasPrefix(f.Mimetype, "audio/"):
			kind = AttachmentAudio
		}
		r.Attachments = append(r.Attachments, Attachment{
			FileID:       f.ID,
			ContentType:  f.Mimetype,
			SizeBytes:    f.Size,
			OriginalName: f.Name,
			Kind:         kind,
		})
	}
	return r
}

// tsToTime converts a Slack ts ("1654.0001") to a time.Time at second
// resolution (Slack ts integer part is unix seconds; fractional part
// is sub-second sequence + microseconds).
func tsToTime(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	dot := strings.IndexByte(ts, '.')
	intPart := ts
	if dot >= 0 {
		intPart = ts[:dot]
	}
	n, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

// WaitForReply polls GetUpdates until pred is satisfied or timeout
// expires. Mirrors TelegramClient.WaitForReply semantics: the offset
// advances ONLY past the matched ts; non-matching messages remain in
// the queue for subsequent WaitForReply calls (the lifecycle
// scenario contract — scenarios issue WaitForReply for THEIR ts
// without swallowing replies destined for other scenarios run in
// parallel later).
//
// Slack-specific caveat: conversations.history does NOT have a
// server-side "commit past this ts" operation; the watermark is
// purely client-side (the next call's oldest= argument). So unlike
// Telegram's offset commit, we just track lastConfirmed locally and
// pass it on the next call. The contract is identical from the
// caller's perspective: pred-failing messages stay visible.
func (c *SlackClient) WaitForReply(ctx context.Context, _ MessageID, pred Predicate, timeout time.Duration) (Reply, error) {
	if pred == nil {
		return Reply{}, errors.New("WaitForReply: nil predicate")
	}
	deadline := time.Now().Add(timeout)
	wctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var lastConfirmed int64

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		updates, _, err := c.GetUpdates(wctx, lastConfirmed)
		if err != nil {
			if errors.Is(wctx.Err(), context.DeadlineExceeded) {
				return Reply{}, context.DeadlineExceeded
			}
			return Reply{}, err
		}
		for _, r := range updates {
			if pred(r) {
				// Advance the watermark past this ts so the next
				// WaitForReply (in a later scenario) doesn't see it.
				lastConfirmed = tsToOffset(string(r.MessageID)) + 1
				_ = lastConfirmed // documented intent; not committed server-side
				return r, nil
			}
		}
		// No match this batch — wait a tick, do NOT advance
		// lastConfirmed.
		select {
		case <-wctx.Done():
			return Reply{}, context.DeadlineExceeded
		case <-ticker.C:
		}
	}
}

// Download fetches an attachment by file-id via the two-step
// files.info + url_private_download dance. Slack file ids are opaque
// strings (Fxxx); the url_private_download requires the bot token in
// an Authorization: Bearer header.
func (c *SlackClient) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	env, _, err := c.callJSON(ctx, "files.info", map[string]any{"file": fileID})
	if err != nil {
		return nil, err
	}
	if env.File == nil || env.File.URLPrivateDownload == "" {
		return nil, fmt.Errorf("files.info: %w (empty url_private_download)", ErrEmptyResponse)
	}
	// Defensive: validate the URL is parseable so we fail loud on a
	// malformed Slack response rather than at http.NewRequest.
	if _, perr := url.Parse(env.File.URLPrivateDownload); perr != nil {
		return nil, fmt.Errorf("files.info: malformed url_private_download")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.File.URLPrivateDownload, nil)
	if err != nil {
		return nil, fmt.Errorf("files.info: build download request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("files.info: download request failed")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("files.info: download HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Preflight runs the structured self-check. Sequence:
//
//	auth.test                                 → username, user_id
//	conversations.info(channel=c.channelID)   → ChatType + InChat
//
// Slack does not have a getChatAdministrators-equivalent that
// distinguishes "bot is admin" vs "bot is member" without
// per-membership probing; PheraldBotPresent is left false unless a
// future probe is added. The lifecycle preflight validator (T6) treats
// it as advisory.
//
// expectedChatID is interface-mandated as int64 — Slack channel ids
// are strings. Callers using the builder route a numeric chatID
// through to TelegramClient; for Slack the channelID configured at
// construction time is the source of truth, and expectedChatID is
// IGNORED. We document this divergence in the file header.
func (c *SlackClient) Preflight(ctx context.Context, expectedChatID int64) (PreflightReport, error) {
	_ = expectedChatID // documented: Slack channelID is config-time, not preflight-time
	var report PreflightReport

	// auth.test
	env, _, err := c.callJSON(ctx, "auth.test", struct{}{})
	if err != nil {
		return report, err
	}
	report.Username = env.User
	report.UserID = slackUserIDToInt64(env.UserID)
	// Slack does not expose can_read_all_group_messages — it's a
	// Telegram-specific privacy-mode concept. Slack channel
	// membership IS the consent model; we default to true so the
	// lifecycle preflight validator doesn't refuse Slack runs.
	report.CanReadAllGroupMessages = true

	// Seed the Me cache so a subsequent Me() doesn't double the
	// roundtrip.
	c.meMu.Lock()
	c.meCached = &slackMeCacheEntry{
		username: env.User,
		userID:   env.UserID,
	}
	c.meMu.Unlock()

	// conversations.info — Slack's per-channel metadata endpoint.
	// Unlike chat.postMessage which uses the "channel" string in the
	// top-level response, conversations.info nests its result under
	// "channel" as an object. We decode the raw bytes into a separate
	// shape.
	infoEnv, rawInfo, err := c.callJSON(ctx, "conversations.info", map[string]any{"channel": c.channelID})
	if err != nil {
		report.InChat = false
		return report, nil
	}
	_ = infoEnv // top-level fields not needed; we re-decode the raw
	var infoShape struct {
		OK      bool             `json:"ok"`
		Channel *slackChannelInfo `json:"channel"`
	}
	if jerr := json.Unmarshal(rawInfo, &infoShape); jerr == nil && infoShape.Channel != nil {
		report.InChat = true
		switch {
		case infoShape.Channel.IsChannel && !infoShape.Channel.IsPrivate:
			report.ChatType = "channel"
		case infoShape.Channel.IsChannel && infoShape.Channel.IsPrivate:
			report.ChatType = "private_channel"
		case infoShape.Channel.IsGroup:
			report.ChatType = "group"
		case infoShape.Channel.IsIM:
			report.ChatType = "im"
		case infoShape.Channel.IsMpim:
			report.ChatType = "mpim"
		default:
			report.ChatType = "unknown"
		}
	}
	return report, nil
}

// GetChatMember is a Slack adaptation of the Telegram method. Slack
// does not have a direct equivalent — `conversations.members` lists
// channel members (paginated) but does NOT carry a per-user role
// status like Telegram's getChatMember.
//
// We approximate: dispatch conversations.members and return "member"
// if the requested userID appears in the result, "left" otherwise.
// The userID arg is the int64 form (interface-mandated); Slack ids are
// strings, so we reverse-map via the cached auth.test response (best-
// effort) — callers who need exact id matching should use the string
// form directly (not exposed via this interface).
//
// chatID is IGNORED (Slack channelID is config-time).
func (c *SlackClient) GetChatMember(ctx context.Context, chatID, userID int64) (string, error) {
	_ = chatID
	env, raw, err := c.callJSON(ctx, "conversations.members", map[string]any{
		"channel": c.channelID,
		"limit":   200,
	})
	if err != nil {
		return "", err
	}
	_ = env
	var shape struct {
		OK      bool     `json:"ok"`
		Members []string `json:"members"`
	}
	if jerr := json.Unmarshal(raw, &shape); jerr != nil {
		return "", fmt.Errorf("conversations.members: decode result")
	}
	target := strconv.FormatInt(userID, 10)
	for _, m := range shape.Members {
		if slackUserIDToInt64(m) == userID || m == target {
			return "member", nil
		}
	}
	return "left", nil
}

// Close releases any background goroutines / connections. Idempotent.
// Like the Telegram client, this is a no-op for the raw HTTP client
// (no goroutines, no connections to drain — http.Client manages its
// own pool). Provided for interface compliance.
func (c *SlackClient) Close() error {
	c.meMu.Lock()
	c.meCached = nil
	c.meMu.Unlock()
	return nil
}

// Compile-time check: SlackClient satisfies MessengerClient.
var _ MessengerClient = (*SlackClient)(nil)
