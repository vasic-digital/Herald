// S15 — natural-language + emojis → query fallthrough.
//
// ACTION:   Send "Question with no prefix and emojis 🙂🚀".
// EXPECTED: Classification Type:'query', Confidence:0 (no prefix
//           match → default classification); cc.dispatch fires.
//
// Like S01, asserts Issues.md / Fixed.md unchanged.
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS15(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S15", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Question with no prefix and emojis 🙂🚀"

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
		r.FailureReason = fmt.Sprintf("await-classification (query/emoji): %v", cerr)
		return r
	}
	r.ClassificationSeen = "query"
	r.ActionSeen = "cc.dispatch"

	issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
	if _, derr := assertIssuesDelta(issuesBefore, issuesAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Issues.md mutated on emoji fallthrough: %v", derr)
		return r
	}
	if _, derr := assertIssuesDelta(fixedBefore, fixedAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Fixed.md mutated on emoji fallthrough: %v", derr)
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
	}
	return r
}
