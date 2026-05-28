package slack

import (
	"context"

	"github.com/slack-go/slack/slackevents"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// DispatchMessageEventForTest exposes the unexported dispatchMessageEvent
// helper to the black-box _test package so subscribe_test.go can exercise
// the message → InboundEvent dispatch path without spinning up a real
// Socket Mode WebSocket. Production callers always reach this helper via
// Subscribe → the Events channel → the EventsAPI evt.Type switch.
//
// The wider Subscribe loop integration (WebSocket connect, RunContext,
// Ack, EventsAPI envelope unwrap) is exercised by the T11 chaos test
// against a live workspace + the qaherald lifecycle test (Wave 7 T9);
// pinning it here would require mocking the websocket transport, which
// is out of scope for the hermetic httptest matrix this file anchors.
func DispatchMessageEventForTest(a *Adapter, ctx context.Context, h commons.InboundHandler, inner *slackevents.MessageEvent, self channels.SelfIdentity) error {
	return a.dispatchMessageEvent(ctx, h, inner, self)
}
