// Package messenger is qaherald's messenger-agnostic interface for
// lifecycle scenarios.
//
// Telegram is the first implementation (T2). Slack and Email adapters
// land in Wave 7 without touching scenario code — they fill the same
// interface and the orchestrator constructs them via a builder keyed
// on the configured channel.
//
// Design contract:
//
//  1. No leak of underlying-messenger types. The Telegram impl wraps
//     telebot internally, but every method on MessengerClient returns
//     and accepts only the abstract types declared here (Reply,
//     Predicate, Attachment, MessageID, PreflightReport). A scenario
//     that imports gopkg.in/telebot.v3 directly is by definition out
//     of scope; tests can grep for that import to catch leakage.
//  2. Real-network bytes only. Every Send / SendPhoto / SendDocument /
//     SendVoice / GetUpdates / Download MUST cross an HTTPS socket to
//     the configured Bot API endpoint. The §107 anti-bluff anchor for
//     this package is the httptest server in telegram_test.go counting
//     real round-trips per method.
//  3. Predicate matching MUST NOT consume non-matching updates. The
//     long-poll offset is advanced PAST a matching update; non-matching
//     updates leave the offset at update.id-1 so a subsequent
//     GetUpdates call observes them again. This is the lifecycle
//     scenario contract — scenarios issue WaitForReply for THEIR
//     message_id without swallowing replies destined for other
//     scenarios run in parallel later.
//
// §107 anti-bluff: a no-op Preflight (e.g. `return PreflightReport{
// InChat: true}`) would FAIL TestPreflight_AssertsGetMeAndGetChatMember
// because that test impersonates api.telegram.org and counts hits on
// /getMe + /getChatMember. Constructing the report from anything other
// than real responses to those methods is detected.
package messenger

import (
	"context"
	"errors"
	"io"
	"time"
)

// MessageID is the messenger-side opaque identifier for a single
// message. Telegram uses int64; Slack uses dotted floats like
// "1654.123456". Treat as opaque.
type MessageID string

// Reply is a captured inbound message — a Telegram update with a
// Message body, a Slack RTM message, etc. Reply is the orchestrator's
// view; the raw wire bytes are preserved in Raw for transcript
// forensic anchor purposes.
type Reply struct {
	MessageID        MessageID    // unique per messenger
	ReplyToMessageID MessageID    // empty if not a reply
	SenderUsername   string       // bot or user username
	SenderIsBot      bool         // true if sender is a bot account
	Text             string       // text content (empty if attachment-only)
	Caption          string       // caption on attachment-bearing messages
	Attachments      []Attachment // file ids + mime types
	Timestamp        time.Time    // messenger-side timestamp
	Raw              []byte       // raw JSON for transcript forensic anchor
}

// Attachment describes a single file payload on a Reply.
type Attachment struct {
	FileID       string // messenger's file-id (Telegram file_id / Slack file id)
	ContentType  string // "image/jpeg", "application/pdf", "audio/ogg", ...
	SizeBytes    int64
	OriginalName string // best-effort; some messengers don't carry filename
	Kind         AttachmentKind
}

// AttachmentKind enumerates the orchestrator-visible attachment
// categories. The Telegram impl maps photo/document/voice/audio fields
// onto these.
type AttachmentKind string

const (
	AttachmentPhoto    AttachmentKind = "photo"
	AttachmentDocument AttachmentKind = "document"
	AttachmentVoice    AttachmentKind = "voice"
	AttachmentAudio    AttachmentKind = "audio"
)

// Predicate selects a Reply from the inbound stream. Returning true
// accepts the Reply; returning false leaves the Reply in the queue for
// a subsequent GetUpdates / WaitForReply call.
type Predicate func(Reply) bool

// MessengerClient is the lifecycle-facing interface. Implementations
// MUST be safe for sequential use; concurrency across distinct
// scenarios is the orchestrator's responsibility (it serializes
// scenarios).
type MessengerClient interface {
	// Me returns the bot identity. Used for self-filter logic + report
	// attribution. MUST cross the wire on first call (getMe / auth.test
	// / equivalent); subsequent calls MAY cache.
	Me(ctx context.Context) (username string, userID int64, err error)

	// Send delivers a text message into the configured chat. Returns
	// the messenger's message-id. Implementations MUST cross the
	// network — no caching, no stub paths.
	Send(ctx context.Context, text string) (MessageID, error)

	// SendPhoto uploads a photo with optional caption. The file at
	// `path` is read and posted via multipart.
	SendPhoto(ctx context.Context, path string, caption string) (MessageID, error)

	// SendDocument uploads a non-photo binary with optional caption.
	SendDocument(ctx context.Context, path string, caption string) (MessageID, error)

	// SendVoice uploads a voice/audio attachment (ogg/opus for
	// Telegram). No caption (Telegram voice messages don't carry one).
	SendVoice(ctx context.Context, path string) (MessageID, error)

	// WaitForReply waits up to `timeout` for the first Reply that
	// satisfies `pred`. The `toMsgID` argument is advisory — pred can
	// inspect Reply.ReplyToMessageID if it wants strict reply-chain
	// matching; pred may also accept un-replied messages.
	//
	// Non-matching Replies remain in the messenger's update queue —
	// the implementation's offset bookkeeping advances PAST the
	// matched Reply only.
	//
	// Returns context.DeadlineExceeded on timeout (the test asserts
	// errors.Is against that sentinel).
	WaitForReply(ctx context.Context, toMsgID MessageID, pred Predicate, timeout time.Duration) (Reply, error)

	// GetUpdates pulls Replies with messenger.update_id > offset and
	// returns them PLUS the new high-water-mark offset. This is the
	// low-level primitive WaitForReply layers on top of; scenarios may
	// call it directly to drain residue at teardown.
	//
	// The implementation MUST NOT advance the server-side offset; the
	// caller decides when to commit (by passing a new offset to a
	// subsequent call).
	GetUpdates(ctx context.Context, offset int64) ([]Reply, int64, error)

	// Download fetches an attachment by file-id and returns its bytes
	// as an io.ReadCloser. Caller MUST Close.
	Download(ctx context.Context, fileID string) (io.ReadCloser, error)

	// Preflight runs a self-check against the messenger backend and
	// returns a structured report. Lifecycle's preflight validator
	// (T6) consumes this; missing capabilities (privacy mode enabled,
	// bot not in chat, wrong chat type) fail the run before any
	// scenario executes.
	Preflight(ctx context.Context, expectedChatID int64) (PreflightReport, error)

	// GetChatMember returns the membership status of userID in chatID
	// ("creator"/"administrator"/"member"/"restricted"/"left"/"kicked").
	// Used by preflight G1 to verify pherald-bot presence via a real
	// getChatMember call (works for non-admin members, unlike
	// getChatAdministrators).
	GetChatMember(ctx context.Context, chatID, userID int64) (status string, err error)

	// Close releases any background goroutines / connections. Idempotent.
	Close() error
}

// PreflightReport carries the structured output of MessengerClient.Preflight.
// Lifecycle's preflight validator (T6) consumes this to decide pass/fail
// before any scenario runs.
type PreflightReport struct {
	Username                string // bot username (no @)
	UserID                  int64  // bot user-id
	CanReadAllGroupMessages bool   // Telegram: getMe.can_read_all_group_messages — privacy-mode-disabled proof
	InChat                  bool   // Telegram: getChatMember returns non-error → bot is a member
	ChatType                string // "group", "supergroup", "channel"; lifecycle requires group|supergroup
	PheraldBotPresent       bool   // best-effort: getChatAdministrators contained pherald-bot-username
}

// Sentinel errors. Implementations MUST wrap (or return) these so
// callers can errors.Is against them.
var (
	// ErrPredicateNotSatisfied is returned by WaitForReply when the
	// context expired before any Reply satisfied the predicate.
	// Callers SHOULD errors.Is(err, context.DeadlineExceeded) too;
	// this sentinel is the canonical lifecycle-side classification.
	ErrPredicateNotSatisfied = errors.New("messenger: no Reply satisfied predicate before timeout")

	// ErrEmptyResponse indicates a wire-level success (HTTP 200) but a
	// response body with no usable payload (missing message_id, empty
	// updates array on a long-poll, etc.). Treated as a §107 bluff
	// guard — wire-bytes exist but they prove nothing.
	ErrEmptyResponse = errors.New("messenger: empty response from backend")
)
