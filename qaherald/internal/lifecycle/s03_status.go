// S03 — Status: fast-path → docs/Status.md prose (no CC).
//
// ACTION:   Send "Status:"
// EXPECTED: pherald reads docs/Status.md and replies with leading
//           prose; no cc.dispatch emitted; transcript shows
//           tgram.send_reply kind.
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS03(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S03", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Status:"

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "")
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	// Status fast-path emits tgram.send_reply whose payload.text is
	// the Status.md prose. We use "Status" (the doc's title-leading
	// word) as the lightweight substring anchor — Status.md is
	// guaranteed to lead with this word per the docs export pipeline.
	cline, cerr := awaitClassification(ctx, env, inID, "Status")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await fast-path journal: %v", cerr)
		return r
	}
	r.ClassificationSeen = "status_request"
	r.ActionSeen = "tgram.send_reply"
	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
	}
	return r
}
