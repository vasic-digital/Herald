// Package channels is the Wave 7 generic messenger-channel abstraction.
//
// It extends the spec §11.0 commons.Channel contract with the inbound-
// runtime methods that the Telegram adapter (Wave 6) implicitly defined:
// SendReplyGeneric (quoted reply with attachment fan-out), BotSelfIdentity
// (the anti-echo-loop self-identity), and DownloadAttachment (content-
// addressed inbox write). Slack (T6), and later Max/Email (Wave 8), satisfy
// the same interface so pherald listen + qaherald never rewrite their core
// loops.
//
// Why a NEW package rather than widening commons.Channel: commons.Channel
// is the §11.0 contract every flavor's `serve` (outbound) path depends on;
// widening it would ripple into every flavor. The inbound runtime is a
// strictly richer need, so channels.Channel embeds commons.Channel and adds
// only the inbound methods.
//
// Reply-method naming (T1 implementation note, divergence from plan Step 4
// text — surfaced for T2/T5/T6 authors). The plan's Step 4 interface text
// names the generic reply method SendReply; its Step 5 directs the tgram
// adapter to add SendReplyGeneric while KEEPING its native int64/int
// SendReply (a method a Go type may hold only once). Those two cannot both
// be true for *tgram.Adapter — a type cannot expose two methods named
// SendReply. The hard T1 invariant is that `var _ channels.Channel =
// (*tgram.Adapter)(nil)` compiles WITHOUT touching the native, behaviour-
// critical, §107-mutation-gate-anchored tgram.SendReply (whose migration to
// the generic signature is explicitly T5's scope). The only resolution that
// keeps T1 a PURE refactor (zero cross-package churn, all tgram tests green)
// is to name the interface method SendReplyGeneric — matching the tgram
// method Step 5 adds. The separate inbound.Replier interface (T5) may keep
// the SendReply name independently; the slack adapter (T6) names its method
// SendReplyGeneric to satisfy THIS interface.
package channels

import (
	"context"

	"github.com/vasic-digital/herald/commons"
)

// IdentityKind discriminates how an adapter computed its bot self-identity.
// The generalized self-filter (selffilter.go, T4) compares the right field
// per kind without knowing the concrete channel.
type IdentityKind string

const (
	// IdentityUsername — Telegram: bot.Me.Username (the @handle).
	IdentityUsername IdentityKind = "username"
	// IdentityUserID — Slack: the bot_user_id (e.g. "U01ABC").
	IdentityUserID IdentityKind = "user_id"
	// IdentityAddress — Email: the From address Herald sends as.
	IdentityAddress IdentityKind = "address"
)

// SelfIdentity is the channel-native identity the inbound runtime uses to
// drop self-echoes. The adapter returns it from BotSelfIdentity; the
// generalized filter (T4) compares InboundEvent.Sender against it.
type SelfIdentity struct {
	Kind  IdentityKind
	Value string // username (no @), user-id, or address — per Kind
}

// Channel is the Wave 7 richer adapter interface — embeds §11.0
// commons.Channel (Name/Capabilities/Send/Subscribe/HealthCheck) + the three
// inbound-runtime methods Telegram implicitly defined.
type Channel interface {
	commons.Channel

	// SendReplyGeneric posts a reply quoting the message identified by
	// replyToID, then fans out each attachment as its own reply at the same
	// thread depth. recipient.ChannelUserID is the channel-native target
	// (Telegram chat_id, Slack channel id). replyToID == "" => no-reply
	// sentinel (fresh message). Returns the channel-native message id of the
	// text reply.
	//
	// Named SendReplyGeneric (not SendReply) so *tgram.Adapter satisfies this
	// interface while retaining its native int64/int SendReply unchanged —
	// see the package-doc divergence note.
	SendReplyGeneric(ctx context.Context, recipient commons.Recipient, body string, replyToID string, attachments []commons.Attachment) (string, error)

	// BotSelfIdentity returns the channel-native bot identity. MUST cross the
	// wire on first call (getMe / auth.test); MAY cache. Empty Value is an
	// echo-loop hazard — Subscribe refuses to boot without a self-identity.
	BotSelfIdentity(ctx context.Context) (SelfIdentity, error)

	// DownloadAttachment streams the channel-hosted file externalID into
	// ~/.herald/inbox/<channel>/<sha256>.<ext> while hashing inline; returns
	// (finalPath, sha256Hex, error). Idempotent (content == proof of presence).
	DownloadAttachment(ctx context.Context, externalID string, mime string) (string, string, error)
}
