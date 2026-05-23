// S01 — plain greeting → query fallthrough → CC dispatch.
//
// Operator mandate (verbatim mapping from
// tests/test_wave6.5_lifecycle.sh S1):
//
//	ACTION:   Send "Hi pherald, how are you?"
//	EXPECTED: pherald classifies Type:'query', Confidence:0;
//	          cc.dispatch fires; Issues.md / Fixed.md unchanged.
//
// §107 evidence: classification JSONL line is the per-PASS anchor;
// Issues.md / Fixed.md byte-equality is the negative-evidence anchor
// (no incidental mutation on a fallthrough query).
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS01(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S01", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Hi pherald, how are you?"

	issuesBefore, fixedBefore, err := snapshotIssuesFixed(env)
	if err != nil {
		r.FailureReason = fmt.Sprintf("snapshot: %v", err)
		return r
	}

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "")
	if err != nil {
		r.FailureReason = err.Error()
		r.InboundMessageID = inID
		return r
	}
	r.InboundMessageID = inID
	r.ReplyMessageID = reply.MessageID

	cline, err := awaitClassification(ctx, env, inID, "query")
	if err != nil {
		r.FailureReason = fmt.Sprintf("await-classification (query): %v", err)
		return r
	}
	r.ClassificationSeen = "query"
	r.ActionSeen = "cc.dispatch"

	issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
	if _, derr := assertIssuesDelta(issuesBefore, issuesAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Issues.md mutated on fallthrough: %v", derr)
		return r
	}
	if _, derr := assertIssuesDelta(fixedBefore, fixedAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Fixed.md mutated on fallthrough: %v", derr)
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
	}
	return r
}
