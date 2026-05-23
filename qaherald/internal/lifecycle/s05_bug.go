// S05 — Bug: prefix → CC issue.open path → HRD-NNN appended.
//
// ACTION:   Send "Bug: telemetry pipe drops every hour"
// EXPECTED: cc.dispatch with Type:'bug', Confidence:1.0; reply
//           confirms HRD-NNN allocation; new row in Issues.md.
//
// Side effect: stores the allocated HRD-NNN into env.LastOpenedHRD
// for S08 (Done:) to consume. The orchestrator preserves this field
// across scenarios in the same run.
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS05(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S05", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Bug: telemetry pipe drops every hour"

	issuesBefore, fixedBefore, err := snapshotIssuesFixed(env)
	if err != nil {
		r.FailureReason = fmt.Sprintf("snapshot: %v", err)
		return r
	}

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "HRD-")
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	cline, cerr := awaitClassification(ctx, env, inID, "bug")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (bug): %v", cerr)
		return r
	}
	r.ClassificationSeen = "bug"
	r.ActionSeen = "cc.dispatch + issue.open"

	hrdID, herr := extractHRDID(reply.Text)
	if herr != nil {
		r.FailureReason = fmt.Sprintf("extract HRD-NNN: %v", herr)
		return r
	}
	env.LastOpenedHRD = hrdID // S08/S10 will consume this

	issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
	hunk, derr := assertIssuesDelta(issuesBefore, issuesAfter, 1)
	if derr != nil {
		r.FailureReason = fmt.Sprintf("Issues.md delta: %v", derr)
		r.Evidence = append(r.Evidence, EvidenceFragment{Kind: "issues-diff", Content: hunk})
		return r
	}
	if _, derr := assertIssuesDelta(fixedBefore, fixedAfter, 0); derr != nil {
		r.FailureReason = fmt.Sprintf("Fixed.md mutated unexpectedly: %v", derr)
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
		{Kind: "issues-diff", Content: hunk},
		{Kind: "allocated-hrd", Content: hrdID},
	}
	return r
}
