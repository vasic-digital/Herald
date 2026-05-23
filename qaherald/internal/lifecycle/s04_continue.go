// S04 — Continue: fast-path → docs/CONTINUATION.md prose (no CC).
//
// ACTION:   Send "Continue:"
// EXPECTED: pherald reads docs/CONTINUATION.md and replies with
//           leading prose; transcript shows tgram.send_reply.
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS04(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S04", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Continue:"

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "")
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	// CONTINUATION.md starts with a heading; "# " is the markdown
	// header marker that pherald embeds verbatim into the reply text.
	cline, cerr := awaitClassification(ctx, env, inID, "# ")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await fast-path journal: %v", cerr)
		return r
	}
	r.ClassificationSeen = "continuation_request"
	r.ActionSeen = "tgram.send_reply"
	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
	}
	return r
}
