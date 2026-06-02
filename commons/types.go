// Package commons is Herald's L0 foundation. See spec V3 §10 + §11.0.
//
// This file defines the Channel interface every adapter implements and
// its complete set of value types. Adapters under commons_messaging/
// channels/<name>/ consume these types directly; no adapter is allowed
// to invent its own equivalent (spec §11.0).
package commons

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
)

// Channel is the interface every channel adapter implements (spec §11.0).
type Channel interface {
	Name() string               // "tgram", "slack", ...
	Capabilities() Capabilities // declarative feature flags
	Send(ctx context.Context, msg OutboundMessage) (Receipt, error)
	Subscribe(ctx context.Context, h InboundHandler) error // long-running, called by `serve`
	HealthCheck(ctx context.Context) error
}

// Capabilities advertises what an adapter supports at runtime (spec §11.0).
// Routers consult Capabilities before dispatching; mismatches are
// logged and the message is dropped or downgraded per the adapter's
// choice.
type Capabilities struct {
	Text             bool
	Markdown         bool // adapter's native markdown flavor
	HTML             bool
	Attachments      bool
	AttachmentMaxMiB int
	Threads          bool             // first-class reply threading
	InteractiveURL   bool             // URL buttons / link actions
	InteractiveCall  bool             // callback handlers (advanced tier)
	DeliveryCeiling  DeliveryEvidence // see §17 / R-05
}

// DeliveryEvidence enumerates the strongest reachable signal per
// channel (spec §11.0 + §17.4.1).
type DeliveryEvidence int

const (
	DeliveryUnknown   DeliveryEvidence = iota
	DeliveryAccepted                   // hop-by-hop ack (SMTP 250)
	DeliveryRouted                     // platform stored & broadcast (Telegram/Slack API ok)
	DeliveryDelivered                  // recipient transport confirmed (Email DSN, WA delivered)
	DeliveryRead                       // recipient read (Email MDN, Telegram Business read marker)
)

// String returns the lowercase enum name (matches the spec wording).
func (d DeliveryEvidence) String() string {
	switch d {
	case DeliveryAccepted:
		return "accepted"
	case DeliveryRouted:
		return "routed"
	case DeliveryDelivered:
		return "delivered"
	case DeliveryRead:
		return "read"
	default:
		return "unknown"
	}
}

// OutboundMessage is the value passed to Channel.Send (spec §11.0).
type OutboundMessage struct {
	EventID        string      // CloudEvents id (§4)
	IdempotencyKey string      // explicit; falls back to EventID
	TenantID       string      // UUID; matches `subscribers.tenant_id`
	To             []Recipient // resolved from preferences + tag fan-out
	Subject        string      // optional (Email, RSS-like channels)
	Body           Body        // rendered per-channel template output
	Attachments    []Attachment
	Thread         *ConversationRef // optional; per §12
	Priority       Priority         // ntfy-compatible 1..5
	Actions        []Action         // optional interactive buttons
	Branding       Branding         // per-flavor (§6.3)
	Trace          TraceContext     // OTel propagation
}

// Body carries one or more rendered representations; the adapter picks
// the best match for its Capabilities (Slack Block-Kit JSON, HTML email,
// Markdown for Telegram, etc.).
type Body struct {
	Plain    string
	Markdown string
	HTML     string
	Native   map[string]any // adapter-specific (Slack blocks, Adaptive Card JSON, ...)
}

// Attachment is a single attached file. Reader is lazy so the adapter
// streams without buffering the entire payload (spec §11.0).
type Attachment struct {
	Filename  string
	MIMEType  string
	SizeBytes int64
	Reader    func() (io.ReadCloser, error)
	CID       string // optional inline-image Content-ID (Email)
}

// Recipient is a resolved (channel, channel_user_id) pair (spec §11.0).
type Recipient struct {
	Channel       string // "tgram", "slack", "mailto", ...
	ChannelUserID string // chat_id, U0xxx, email address, ...
	DisplayName   string // best-effort, for templating
}

// Action is an interactive UI hint (URL button, callback button,
// Adaptive Card action, ntfy X-Action, etc.).
type Action struct {
	Type   ActionType
	Label  string
	URL    string // for ActionView / ActionURL / ActionHTTP
	Method string // "GET" | "POST" (ActionHTTP)
	Body   []byte // ActionHTTP
	Data   string // ActionCallback payload (provider-defined)
}

// ActionType selects how the channel adapter renders an Action.
type ActionType int

const (
	ActionView     ActionType = iota // open a URL
	ActionURL                        // synonym for ActionView; provider-specific styling
	ActionCallback                   // round-trip back into Herald via inbound handler
	ActionHTTP                       // fire-and-forget HTTP from the recipient device (ntfy)
	ActionCopy                       // copy text to clipboard (ntfy)
)

// Priority maps to ntfy 1..5 and to per-channel native priorities.
type Priority int

const (
	PriorityLow    Priority = 1
	PriorityNormal Priority = 3
	PriorityHigh   Priority = 4
	PriorityUrgent Priority = 5
)

// Receipt is what Channel.Send returns on success — the adapter's
// evidence of acceptance/routing/delivery so the router can decide
// whether to retry (spec §11.0).
type Receipt struct {
	Evidence      DeliveryEvidence
	ChannelMsgID  string // Slack ts; Telegram message_id; SMTP queue-id; ...
	SentAt        time.Time
	LatencyMillis int64
	Native        map[string]any // adapter-specific raw response (for diary)
}

// InboundHandler receives messages emitted by the adapter's Subscribe
// loop. Implementations enqueue events back into the router (§5).
type InboundHandler interface {
	Handle(ctx context.Context, ev InboundEvent) error
}

// InboundEvent is a CloudEvent constructed from a subscriber message.
type InboundEvent struct {
	EventID     string             // UUIDv7
	CloudEvent  CloudEventEnvelope // §4.1
	Sender      Recipient          // who sent it
	Subscriber  *Subscriber        // resolved via subscriber_aliases, nil if unknown
	Body        Body
	Attachments []Attachment
	Thread      *ConversationRef
	// ThreadContext carries the PRIOR messages of the thread this event belongs
	// to (oldest→newest), populated by the channel adapter when the inbound
	// message is itself inside a thread (Slack thread_ts present; Telegram a
	// reply_to chain). Empty for a fresh/top-level message. The dispatcher feeds
	// this to Claude so a reply is bound by the thread's MEANING and only made
	// when the thread's context warrants one — replies are contributions to a
	// thread, not isolated answers (operator mandate 2026-06-02). It excludes the
	// current inbound message itself (that is in Body).
	ThreadContext []ThreadMessage
	Raw           map[string]any // adapter-specific raw payload (for diary)
}

// ThreadMessage is one prior message in a thread, supplying the conversational
// context that binds a reply to the thread's meaning (operator mandate
// 2026-06-02). Adapters populate it from the channel's thread-history API
// (Slack conversations.replies; Telegram the reply_to chain).
type ThreadMessage struct {
	SenderHandle string    // resolved/raw sender handle; "Claude" for this bot's own prior messages
	SenderIsBot  bool      // true when authored by a bot (incl. this Herald bot)
	Text         string    // the message text
	Timestamp    time.Time // when it was sent (zero if the adapter cannot determine it)
}

// ConversationRef is the per-channel thread identifier (§12 + §11.0).
type ConversationRef struct {
	Channel         ChannelID
	ThreadID        string // Slack thread_ts; Telegram message_thread_id (forum); Email References[0]
	ParentMessageID string // Slack ts; Telegram reply_to_message_id; Email In-Reply-To
	RootMessageID   string // first message in the thread
}

// Subscriber is the in-memory projection of `subscribers` + its
// linked `subscriber_aliases` (spec §7.1 + §11.0).
type Subscriber struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Handle      string // empty if not operator-mapped
	DisplayName string
	Locale      string // BCP-47, e.g. "en-US", "sr-Latn-RS"
	Timezone    string // IANA, e.g. "Europe/Belgrade"
	Roles       []string
	Metadata    map[string]any
	Aliases     []SubscriberAlias
	Preferences *PreferenceSet
}

// SubscriberAlias is one row from `subscriber_aliases`.
type SubscriberAlias struct {
	Channel       string
	ChannelUserID string
	VerifiedAt    *time.Time
	LastSeenAt    *time.Time
}

// CloudEventEnvelope is Herald's typed projection of a CloudEvents
// v1.0 payload (spec §4.1 + §11.0). The boundary type is
// cloudevents.Event from github.com/cloudevents/sdk-go/v2; this struct
// is the in-process canonical form after parsing / validation.
type CloudEventEnvelope struct {
	SpecVersion     string
	ID              string // UUIDv7
	Source          string // URI
	Type            string // reverse-DNS
	Time            time.Time
	Subject         string            // tag:/channel:/empty
	DataContentType string            // e.g. "application/json"
	Data            []byte            // opaque payload
	Extensions      map[string]string // heraldtenant, heraldidempotencykey, heraldpriority, ...
}

// TraceContext carries OpenTelemetry trace propagation across Herald
// boundaries (spec §11.0 + §17.3).
type TraceContext struct {
	TraceID    string // 32-hex chars (W3C Trace Context)
	SpanID     string // 16-hex chars (the parent span at handoff)
	TraceFlags byte   // sampling flags (W3C)
	TraceState string // vendor-specific (W3C tracestate header)
	Baggage    string // W3C baggage header value
}

// Branding is the per-flavor visual identity (spec §6.3 + §11.0).
type Branding struct {
	AppName        string // "Project Herald", "System Herald", ...
	BinaryName     string // "pherald", "sherald", ...
	IconURL        string // for rich embeds
	AccentColorHex string // "#2C7BE5"
	DefaultFooter  string // "Sent by pherald 1.0 · github.com/vasic-digital/Herald"

	// Wave 2 per-flavor identity fields (design §3.5). Populated by
	// DefaultBranding and consumed by the shared commons/cli/ scaffold
	// to render --version / --help and to bind the default HTTP port.
	Flavor      string // single-letter (or short) flavor key: "p", "s", "b", "sc", ...
	Prefix      string // 3-letter prefix per §8.2 (e.g. "PHR", "SHR")
	DisplayName string // human-readable display name (typically == AppName)
	DefaultPort int    // default HTTP listen port (per-flavor, 70XXX range)
	Mission     string // one-line mission statement for --help / about
}

// ChannelID is the canonical channel identifier (spec §11.0). It MUST
// match the scheme used in `channel_addresses.address_url` and in the
// URL scheme registered by the adapter.
type ChannelID string

const (
	ChannelTelegram ChannelID = "tgram"
	ChannelMax      ChannelID = "max"
	ChannelSlack    ChannelID = "slack"
	ChannelDiscord  ChannelID = "discord"
	ChannelTeams    ChannelID = "teams"
	ChannelLark     ChannelID = "lark"
	ChannelWhatsApp ChannelID = "whatsapp"
	ChannelViber    ChannelID = "viber"
	ChannelEmail    ChannelID = "mailto"
	ChannelNtfy     ChannelID = "ntfy"
	ChannelGotify   ChannelID = "gotify"
	ChannelWebhook  ChannelID = "webhook"
	ChannelDiary    ChannelID = "diary"
	ChannelNull     ChannelID = "null" // §11.14 sandbox/no-op adapter for tests
)

// PreferenceSet is a typed view of the per-subscriber preferences JSON
// stored in `subscribers.metadata.preferences` (spec §7.2 + §11.0).
type PreferenceSet struct {
	Categories  map[string]CategoryPref // category_id → pref
	Workflows   map[string]WorkflowPref // CloudEvents type → pref
	QuietHours  *QuietHours             // nil ⇒ no quiet hours configured
	ChannelData map[ChannelID]any       // provider-specific routing data
}

// CategoryPref is the opt-in / channels for one category (spec §7.2).
type CategoryPref struct {
	Channels []ChannelID
	Muted    bool
}

// WorkflowPref is the opt-in / channels for one CloudEvents type.
type WorkflowPref struct {
	Channels []ChannelID // may be empty (= use Category default)
	Muted    bool
}

// QuietHours encodes the TZ-aware silence window (spec §7.3).
type QuietHours struct {
	TZ               string   // IANA TZ
	Start            string   // "HH:MM" 24h
	End              string   // "HH:MM" 24h
	ExemptCategories []string // categories that override quiet hours
}
