// Package inbound — cc_adapter.go is the production binding between the
// inbound.CodeDispatcher interface and the concrete
// *claude_code.Dispatcher.
//
// The inbound package abstracts over the dispatcher (so the unit tests in
// dispatcher_test.go can stub it without dragging in the claude_code
// package, exec.Cmd, real session anchors, etc.). The real
// `claude_code.Dispatcher.Dispatch` already strips the <<<HERALD-REPLY>>>
// marker and returns a typed DispatchResponse. To keep ParseReply's
// uniform marker-scanning contract working at the inbound layer, this
// adapter re-emits a synthetic stdout line carrying <<<HERALD-REPLY>>>
// followed by a minimal JSON projection of the typed response.
//
// Wave 6 ships this with `action: "reply"` + `text: resp.Summary`. The
// operator may evolve this projection (e.g. route resp.Outcome=="rejected"
// to action="issue.open" with a generated payload) in HRD-NNN-W6c without
// touching the inbound.Dispatcher itself.
package inbound

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code"
)

// NewCCAdapter wraps a concrete claude_code.Dispatcher as a
// CodeDispatcher implementation suitable for inbound.Config.Code.
func NewCCAdapter(d *claude_code.Dispatcher) CodeDispatcher {
	return ccAdapter{d: d}
}

type ccAdapter struct {
	d *claude_code.Dispatcher
}

// Dispatch translates the inbound CodeRequest into a
// claude_code.DispatchRequest, runs the real CC dispatcher, and re-emits
// the response as marker-prefixed stdout so ParseReply can decode it
// uniformly.
//
// §107 anchor: the typed response from claude_code.Dispatch already
// guarantees a non-empty marker + JSON in the wire output (or it would
// have errored). Re-projecting it as `{"action":"reply","text":Summary}`
// is faithful — the alternative ("synthesize an empty reply") would be
// a §107 PASS-bluff.
func (a ccAdapter) Dispatch(ctx context.Context, req CodeRequest) (CodeResponse, error) {
	if a.d == nil {
		return CodeResponse{}, fmt.Errorf("inbound.ccAdapter: nil claude_code.Dispatcher")
	}
	ccReq := claude_code.DispatchRequest{
		InboundID:     req.InboundID,
		Sender:        req.Sender,
		Channel:       req.Channel,
		Conversation:  req.Conversation,
		Attachments:   req.Attachments,
		UserMessage:   req.UserMessage,
		ThreadContext: req.ThreadContext,
		Classification: claude_code.Classification{
			Type:        req.Classification.Type,
			Criticality: req.Classification.Criticality,
			Confidence:  req.Classification.Confidence,
		},
	}
	resp, err := a.d.Dispatch(ctx, ccReq)
	if err != nil {
		return CodeResponse{}, err
	}
	// Re-serialise the typed DispatchResponse back into a marker-bearing
	// stdout line so ParseReply's uniform contract holds.
	b, marshalErr := json.Marshal(map[string]any{
		"action": "reply",
		"text":   resp.Summary,
	})
	if marshalErr != nil {
		return CodeResponse{}, fmt.Errorf("inbound.ccAdapter: re-emit reply: %w", marshalErr)
	}
	out := append([]byte("<<<HERALD-REPLY>>> "), b...)
	return CodeResponse{Stdout: out}, nil
}
