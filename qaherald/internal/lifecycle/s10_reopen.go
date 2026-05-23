// S10 — Reopen: HRD-NNN by operator → Fixed→Issues migration.
//
// ACTION:   Send "Reopen: <HRD-NNN>" where HRD-NNN is the row closed
//           by S08 (env.LastOpenedHRD continues to track this id —
//           pherald's commands handler keys on the id, not the row's
//           current home).
// EXPECTED: reopen classification (Confidence:1.0); fast-path
//           Fixed→Issues atomic migration.
//
// FAIL if env.LastOpenedHRD is empty.
package lifecycle

import (
	"context"
	"fmt"
	"time"
)

func runS10(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S10", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	if env.LastOpenedHRD == "" {
		r.FailureReason = "S10 prerequisite missing: env.LastOpenedHRD is empty — S05/S06 and S08 must run first"
		return r
	}

	input := "Reopen: " + env.LastOpenedHRD

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

	cline, cerr := awaitClassification(ctx, env, inID, "reopen")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (reopen): %v", cerr)
		return r
	}
	r.ClassificationSeen = "reopen"
	r.ActionSeen = "fixed→issues atomic"

	issuesAfter, fixedAfter, _ := snapshotIssuesFixed(env)
	issuesHunk, derr := assertIssuesDelta(issuesBefore, issuesAfter, 1)
	if derr != nil {
		r.FailureReason = fmt.Sprintf("Issues.md delta (+1): %v", derr)
		r.Evidence = append(r.Evidence, EvidenceFragment{Kind: "issues-diff", Content: issuesHunk})
		return r
	}
	fixedHunk, derr := assertIssuesDelta(fixedBefore, fixedAfter, -1)
	if derr != nil {
		r.FailureReason = fmt.Sprintf("Fixed.md delta (-1): %v", derr)
		r.Evidence = append(r.Evidence, EvidenceFragment{Kind: "fixed-diff", Content: fixedHunk})
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
		{Kind: "issues-diff", Content: issuesHunk},
		{Kind: "fixed-diff", Content: fixedHunk},
		{Kind: "reopened-hrd", Content: env.LastOpenedHRD},
	}
	return r
}
