// S07 — Query: prefix → explicit query classification (Confidence:1).
//
// ACTION:   Send "Query: what tag is current?"
// EXPECTED: cc.dispatch with Type:'query', Confidence:1.0 (explicit
//           prefix); Issues.md / Fixed.md unchanged (query is not a
//           ticket-opening operation).
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS07(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S07", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Query: what tag is current?"

	issuesBefore, fixedBefore, err := snapshotIssuesFixed(env)
	if err != nil {
		r.FailureReason = fmt.Sprintf("snapshot: %v", err)
		return r
	}

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "")
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	cline, cerr := awaitClassification(ctx, env, inID, "query")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (query): %v", cerr)
		return r
	}
	r.ClassificationSeen = "query"
	r.ActionSeen = "cc.dispatch"

	issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
	if _, derr := assertIssuesDelta(issuesBefore, issuesAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Issues.md unexpectedly mutated on Query:: %v", derr)
		return r
	}
	if _, derr := assertIssuesDelta(fixedBefore, fixedAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Fixed.md unexpectedly mutated on Query:: %v", derr)
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
	}
	return r
}
