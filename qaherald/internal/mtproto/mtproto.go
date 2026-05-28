// Package mtproto provides a real-user Telegram client for fully-autonomous
// closed-loop testing of Herald's bot-driven flows, per HelixConstitution
// §11.4.98 (Full-Automation Anti-Bluff Mandate) + Herald §108.m.
//
// Why MTProto and not Bot API: Telegram's bot privacy boundary blocks bots
// from seeing each other's messages in non-DM contexts. Empirically verified
// 2026-05-28 in group -4946584787: a second bot sent a message; the target
// bot's getUpdates saw 0 updates. The only autonomous driver for inbound
// flows is a real user account, which speaks MTProto (the same wire protocol
// Telegram's mobile/desktop apps use).
//
// Scope:
//   - Client interface — Send / WaitForReply / Close
//   - Session file management (persisted to ~/.config/herald/mtproto.session;
//     chmod 600; never committed)
//   - First-run interactive bootstrap via `qaherald mtproto login` (the
//     §11.4.98(B) permitted one-time exception — configuration, not test
//     driving). Subsequent runs are fully autonomous.
//
// Credentials: HERALD_MTPROTO_APP_ID + HERALD_MTPROTO_APP_HASH +
// HERALD_MTPROTO_PHONE (+ HERALD_MTPROTO_PASSWORD if 2FA enabled). See
// docs/requirements/blockers/missing_env_variables.md for the operator
// step-by-step.
//
// Security: HRD-133 parity — every error return is wrapped by
// sanitizeMTProtoError so api_hash, session bytes, and 2FA password text
// NEVER appear in committed logs or docs/qa/ transcripts.
//
// Status (2026-05-28): scaffold landed. Live wiring blocks on operator
// credential bootstrap. Once .env is populated + `qaherald mtproto login`
// runs once, this package replaces the manual-dep test paths.
package mtproto

import (
	"context"
	"errors"
	"time"
)

// Config carries the operator-supplied credentials + runtime knobs. All
// values originate from environment variables (resolved at the call site
// via os.Getenv); this package never reads env directly so tests can inject
// alternate values.
type Config struct {
	// AppID and AppHash come from https://my.telegram.org/apps. AppID is a
	// small integer (5-8 digits); AppHash is a 32-char lowercase hex string.
	AppID   int
	AppHash string

	// Phone is the E.164 phone of the Telegram USER account driving QA tests
	// (NOT the bot — bots can't drive other bots).
	Phone string

	// Password is the cloud 2FA password if the QA account has 2FA enabled.
	// Empty when 2FA is disabled.
	Password string

	// SessionFile is the absolute path to the persisted session. Default
	// resolved by DefaultSessionFile() if empty: ~/.config/herald/mtproto.session.
	SessionFile string

	// Now provides time.Now-equivalent for hermetic tests. nil → time.Now.
	Now func() time.Time
}

// Message is a Telegram chat message — the unit Client.WaitForReply returns.
// Subset of Telegram's Message schema relevant to Herald QA flows.
type Message struct {
	// ID is Telegram's message_id (int64; positive integer).
	ID int64

	// ChatID is the destination chat. Group/supergroup IDs are negative.
	ChatID int64

	// FromUserID is Telegram's user_id of the sender. Bot replies have
	// FromUserID == the bot's user_id.
	FromUserID int64

	// IsBot reports whether the sender is a bot.
	IsBot bool

	// Text is the message body. Captions on photo/document messages also
	// land here.
	Text string

	// ReplyToMessageID is the message_id this message is a reply to, or 0
	// if the message is not a reply.
	ReplyToMessageID int64

	// Date is the message timestamp.
	Date time.Time
}

// Client is the §11.4.98-compliant Telegram user driver. Implementations
// MUST be safe for concurrent use by multiple goroutines.
//
// Lifecycle: operator runs `qaherald mtproto login` once to perform the
// interactive bootstrap (Telegram sends a login code; operator enters it
// at the CLI; session persists). Every subsequent test invocation calls
// Connect → uses Send / WaitForReply → calls Close. No human action at any
// point in that subsequent flow.
type Client interface {
	// Connect establishes a Telegram MTProto connection using the persisted
	// session at Config.SessionFile. Returns ErrNoSession if the session
	// file is missing or invalid — caller must run `qaherald mtproto login`
	// first. Connect is idempotent: repeated calls return nil if already
	// connected.
	Connect(ctx context.Context) error

	// SendMessage posts text to chatID as the authenticated user. Returns
	// the Telegram-assigned message_id on success.
	//
	// chatID is the canonical Telegram chat identifier (positive for
	// private DMs, negative for groups, -100<id> for supergroups). The user
	// MUST be a member of chatID; otherwise Telegram returns CHAT_WRITE_FORBIDDEN.
	SendMessage(ctx context.Context, chatID int64, text string) (messageID int64, err error)

	// WaitForReply blocks until a message satisfying matcher arrives in
	// chatID, or until ctx expires. The matcher is invoked for every new
	// message in the chat (including the user's own SendMessage echoes —
	// callers MUST filter those by FromUserID if not desired).
	//
	// Returns the first matching Message, or context.DeadlineExceeded /
	// context.Canceled if ctx ends first.
	//
	// Implementations MUST filter out messages with Date <= start time so
	// stale history doesn't trigger spurious matches.
	WaitForReply(ctx context.Context, chatID int64, matcher func(Message) bool) (Message, error)

	// WhoAmI returns the authenticated user's identity for verification
	// (e.g. via `qaherald mtproto whoami` before a CI campaign). Cheap —
	// no message sent.
	WhoAmI(ctx context.Context) (userID int64, username string, err error)

	// Close releases connections + flushes session state to disk. Safe to
	// call multiple times. After Close, the Client is invalid; create a
	// new one to reconnect.
	Close() error
}

// Sentinel errors. Callers use errors.Is to distinguish them.
var (
	// ErrNoSession indicates the session file is missing or unreadable;
	// the operator must run `qaherald mtproto login` first.
	ErrNoSession = errors.New("mtproto: no session — run `qaherald mtproto login` first")

	// ErrInvalidConfig means Config is missing required fields (AppID,
	// AppHash, or Phone). The error message names the missing field; the
	// credentials themselves are never echoed.
	ErrInvalidConfig = errors.New("mtproto: invalid Config (see error detail for missing field)")

	// ErrSessionPasswordNeeded means the QA account has 2FA enabled but
	// Config.Password is blank. Operator must set HERALD_MTPROTO_PASSWORD
	// in .env.
	ErrSessionPasswordNeeded = errors.New("mtproto: SESSION_PASSWORD_NEEDED — set HERALD_MTPROTO_PASSWORD")

	// ErrFloodWait wraps Telegram's FLOOD_WAIT_<N> response. Use
	// errors.As to extract the retry-after duration.
	ErrFloodWait = errors.New("mtproto: FLOOD_WAIT")
)

// FloodWaitError carries the retry-after duration. Returned wrapped in
// ErrFloodWait so callers can pattern-match via errors.As.
type FloodWaitError struct {
	RetryAfter time.Duration
}

func (e *FloodWaitError) Error() string {
	return "mtproto: FLOOD_WAIT_" + e.RetryAfter.String()
}

func (e *FloodWaitError) Is(target error) bool {
	return target == ErrFloodWait
}

// Validate returns nil iff Config has all required fields populated.
// AppID > 0, AppHash 32 lowercase hex chars, Phone E.164 (+countrycode + digits).
// Password is optional.
func (c *Config) Validate() error {
	if c.AppID <= 0 {
		return sanitizeMTProtoError(errors.New("AppID missing or non-positive"))
	}
	if len(c.AppHash) != 32 {
		return sanitizeMTProtoError(errors.New("AppHash wrong length (want 32 hex chars)"))
	}
	for _, r := range c.AppHash {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return sanitizeMTProtoError(errors.New("AppHash contains non-hex character"))
		}
	}
	if c.Phone == "" {
		return sanitizeMTProtoError(errors.New("Phone missing"))
	}
	if c.Phone[0] != '+' {
		return sanitizeMTProtoError(errors.New("Phone not in E.164 format (must start with +)"))
	}
	return nil
}

// New is defined in client_live.go — it returns a *liveClient (the
// gotd/td-backed implementation). The scaffoldClient that used to live
// here has been removed in favour of the live wiring.
