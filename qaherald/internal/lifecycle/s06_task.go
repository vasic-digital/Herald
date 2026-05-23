// S06 — Task: prefix → CC task classification → HRD-NNN.
//
// ACTION:   Send "Task: refactor channel registry"
// EXPECTED: cc.dispatch with Type:'task', Confidence:1.0; reply
//           confirms HRD-NNN; new row in Issues.md.
//
// Like S05, allocates an HRD-NNN. If S05 has already set
// env.LastOpenedHRD we OVERWRITE — S08 uses the latest. (Operator
// can run S05 then S06 then S08 against S06's HRD; this is the
// documented convention in the Wave 6.5 shell driver.)
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS06(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S06", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Task: refactor channel registry"

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

	cline, cerr := awaitClassification(ctx, env, inID, "task")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (task): %v", cerr)
		return r
	}
	r.ClassificationSeen = "task"
	r.ActionSeen = "cc.dispatch + issue.open"

	hrdID, herr := extractHRDID(reply.Text)
	if herr != nil {
		r.FailureReason = fmt.Sprintf("extract HRD-NNN: %v", herr)
		return r
	}
	env.LastOpenedHRD = hrdID

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
