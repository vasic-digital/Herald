// S12 — inbound document/PDF with Task: caption.
//
// ACTION:   Send a PDF document with caption "Task: review attached
//           spec".
// EXPECTED: tgram.message carries application/* attachment; Type:
//           'task'.
//
// Fixture: fixtures/s12_spec.pdf — a ~10KB committed PDF.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"time"
)

func runS12(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S12", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	fp := fixturePath("s12_spec.pdf")
	if _, err := os.Stat(fp); err != nil {
		r.FailureReason = fmt.Sprintf("fixture missing: %s — commit qaherald/internal/lifecycle/fixtures/s12_spec.pdf", fp)
		return r
	}

	const caption = "Task: review attached spec"

	inID, serr := env.Msgr.SendDocument(ctx, fp, caption)
	if serr != nil {
		r.FailureReason = fmt.Sprintf("send-document: %v", serr)
		return r
	}
	r.InboundMessageID = inID

	if reply, werr := awaitReplyOnly(ctx, env, ""); werr == nil {
		r.ReplyMessageID = reply.MessageID
	}

	cline, cerr := awaitClassification(ctx, env, inID, "task")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (task+doc): %v", cerr)
		return r
	}
	r.ClassificationSeen = "task"
	r.ActionSeen = "inbound document + classify"

	mimeHit, merr := scanTranscriptForSubstr(env, "application/")
	if merr != nil || len(mimeHit) == 0 {
		r.FailureReason = fmt.Sprintf("transcript missing application/* attachment evidence (err=%v)", merr)
		return r
	}

	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "fixture-path", Content: fp},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
		{Kind: "attachment-mime-line", Content: string(mimeHit)},
	}
	return r
}
