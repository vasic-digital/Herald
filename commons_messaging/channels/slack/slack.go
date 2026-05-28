// Package slack is the Wave 7 Slack channel adapter — the SECOND concrete
// channels.Channel implementation, proving the abstraction that Telegram
// (Wave 6 tgram) implicitly defined.
//
// Transport choices (spec §11 Slack rows + §32.2):
//
//   - Outbound: the Slack Web API (chat.postMessage, files.uploadV2,
//     files.info, auth.test).
//   - Inbound: Socket Mode (the WebSocket transport that is Slack's
//     equivalent of Telegram's getUpdates long-poll). Requires an app-level
//     token (xapp-…) in addition to the bot token (xoxb-…).
//
// §107 anti-bluff posture. Every public method that the channels.Channel
// contract exposes crosses the wire (or is gated on a constructor error,
// or returns an explicit echo-loop hazard error when the wire response is
// degenerate). The hermetic httptest suite (slack_test.go / send_test.go /
// selfidentity_test.go / attachments_test.go / subscribe_test.go) counts
// every round-trip — a Send/SendReply/BotSelfIdentity that compiled cleanly
// but never hit chat.postMessage / auth.test would pass type-checks and be
// caught immediately by the counted-hits assertions.
//
// Reply-method naming: SendReplyGeneric (not SendReply) matches the
// channels.Channel interface — see commons_messaging/channels/channel.go
// package doc for the divergence rationale (a Go type may hold only one
// method named SendReply, and tgram's native int64/int SendReply predates
// the generic interface).
package slack

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/slack-go/slack"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// Adapter is the Slack channel adapter — satisfies channels.Channel.
type Adapter struct {
	botToken  string // xoxb-… (chat.postMessage, files.*, auth.test)
	appToken  string // xapp-… (Socket Mode); empty disables Subscribe
	channelID string // default outbound destination (Cxxx)
	baseURL   string // httptest seam; "" => live Slack Web API
	api       *slack.Client

	// cached self-identity (auth.test → user_id). Wave 7 anti-echo-loop
	// guarantee — populated by BotSelfIdentity on first call and consulted
	// thereafter without an extra wire roundtrip.
	selfMu   sync.Mutex
	selfID   string // bot_user_id (U…) cached after first successful auth.test
}

// New constructs a live Slack adapter pointing at the real Slack Web API.
// botToken (xoxb-) is required for outbound + auth.test; appToken (xapp-)
// is required ONLY for Subscribe (Socket Mode). channelID is the default
// outbound destination — recipients may override it per-message.
func New(botToken, appToken, channelID string) *Adapter {
	return NewWithBaseURL(botToken, appToken, channelID, "")
}

// NewWithBaseURL is the httptest seam — baseURL overrides the Web API
// endpoint (slack-go appends method paths to APIURL, which MUST end with
// "/"). Production callers use New (baseURL == ""), which leaves slack-go
// pointed at https://slack.com/api/. The hermetic tests under this
// package construct an httptest server and pass its URL here so wire
// bytes can be asserted directly.
func NewWithBaseURL(botToken, appToken, channelID, baseURL string) *Adapter {
	opts := []slack.Option{}
	if baseURL != "" {
		u := baseURL
		if u[len(u)-1] != '/' {
			u += "/"
		}
		opts = append(opts, slack.OptionAPIURL(u))
	}
	return &Adapter{
		botToken:  botToken,
		appToken:  appToken,
		channelID: channelID,
		baseURL:   baseURL,
		api:       slack.New(botToken, opts...),
	}
}

// NewFromURL parses a slack:// URL of the form
//
//	slack://<bot-token>@<workspace>/<channel-id>[?app_token=<xapp-…>]
//
// Returns the constructed Adapter or an error if any required component is
// missing. The workspace is informational (mirrors a per-host convention so
// op-supplied config can carry it for diary annotation) and is NOT required
// to dial the Slack API. channelID + botToken are mandatory.
func NewFromURL(rawURL string) (*Adapter, error) {
	if !strings.HasPrefix(rawURL, "slack://") {
		return nil, errors.New("slack: URL must start with slack:// scheme")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("slack: parse URL: %w", err)
	}
	if u.Scheme != "slack" {
		return nil, fmt.Errorf("slack: scheme=%q want slack", u.Scheme)
	}
	botToken := ""
	if u.User != nil {
		botToken = u.User.Username()
	}
	if botToken == "" {
		return nil, errors.New("slack: bot token (xoxb-…) required as URL user component")
	}
	channelID := strings.TrimPrefix(u.Path, "/")
	if channelID == "" {
		return nil, errors.New("slack: channel id required as URL path (e.g. slack://xoxb-…@workspace/C0123)")
	}
	appToken := u.Query().Get("app_token")
	return New(botToken, appToken, channelID), nil
}

// Name returns the canonical channel ID — matches commons.ChannelSlack so
// the per-channel inbox (channels.InboxDir("slack")) keys correctly.
func (a *Adapter) Name() string { return string(commons.ChannelSlack) }

// Capabilities per spec §11 Slack rows + Wave 7 plan T6.
//
// Slack-specific values:
//
//   - Markdown: Slack mrkdwn (different dialect from MarkdownV2 — adapters
//     render the same body, but the wire payload uses Slack-flavored markup).
//   - HTML: false (Slack does not accept HTML directly).
//   - AttachmentMaxMiB: 1024 (Slack file upload limit per workspace plan).
//   - Threads: true (thread_ts, the §32.9 reply anchor).
//   - DeliveryCeiling: DeliveryRouted (postMessage 200 == platform stored).
func (a *Adapter) Capabilities() commons.Capabilities {
	return commons.Capabilities{
		Text:             true,
		Markdown:         true,
		HTML:             false,
		Attachments:      true,
		AttachmentMaxMiB: 1024,
		Threads:          true,
		InteractiveURL:   true,
		InteractiveCall:  false,
		DeliveryCeiling:  commons.DeliveryRouted,
	}
}

// init registers the Slack adapter with the channels registry (Wave 7 T6)
// so `pherald listen` can resolve "slack" by name at runtime. cfg.Token =
// bot token (xoxb-…); cfg.AppToken = app-level token (xapp-…, required for
// Subscribe); cfg.Target = channel id; cfg.BaseURL = httptest seam.
func init() {
	channels.Register(string(commons.ChannelSlack), func(cfg channels.Config) (channels.Channel, error) {
		if cfg.Token == "" {
			return nil, fmt.Errorf("slack: cfg.Token (xoxb- bot token) required")
		}
		return NewWithBaseURL(cfg.Token, cfg.AppToken, cfg.Target, cfg.BaseURL), nil
	})
}
