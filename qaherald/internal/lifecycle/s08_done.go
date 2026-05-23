// S08 — Done: HRD-NNN by operator → Issues→Fixed migration.
//
// ACTION:   Send "Done: <HRD-NNN>" where HRD-NNN is the row allocated
//           by S05 or S06 (whichever ran last — env.LastOpenedHRD).
// EXPECTED: closure classification; fast-path Issues→Fixed atomic
//           migration; reply confirms migration.
//
// If env.LastOpenedHRD is empty (S05/S06 did not run / failed),
// FAIL the scenario LOUDLY — we cannot test closure of a ticket we
// did not allocate.
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS08(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S08", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	if env.LastOpenedHRD == "" {
		r.FailureReason = "S08 prerequisite missing: env.LastOpenedHRD is empty — S05 or S06 must allocate an HRD first"
		return r
	}

	input := "Done: " + env.LastOpenedHRD

	issuesBefore, fixedBefore, err := snapshotIssuesFixed(env)
	if err != nil {
		r.FailureReason = fmt.Sprintf("snapshot: %v", err)
		return r
	}

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, env.LastOpenedHRD)
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	cline, cerr := awaitClassification(ctx, env, inID, "closure")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (closure): %v", cerr)
		return r
	}
	r.ClassificationSeen = "closure"
	r.ActionSeen = "issues→fixed atomic"

	issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
	issuesHunk, derr := assertIssuesDelta(issuesBefore, issuesAfter, -1)
	if derr != nil {
		r.FailureReason = fmt.Sprintf("Issues.md delta (-1): %v", derr)
		r.Evidence = append(r.Evidence, EvidenceFragment{Kind: "issues-diff", Content: issuesHunk})
		return r
	}
	fixedHunk, derr := assertIssuesDelta(fixedBefore, fixedAfter, 1)
	if derr != nil {
		r.FailureReason = fmt.Sprintf("Fixed.md delta (+1): %v", derr)
		r.Evidence = append(r.Evidence, EvidenceFragment{Kind: "fixed-diff", Content: fixedHunk})
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
		{Kind: "issues-diff", Content: issuesHunk},
		{Kind: "fixed-diff", Content: fixedHunk},
		{Kind: "closed-hrd", Content: env.LastOpenedHRD},
	}
	return r
}
