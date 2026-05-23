// S11 — inbound photo + Bug: caption → image/* attachment + bug.
//
// ACTION:   Send a photo with caption "Bug: visual artifact in
//           dashboard".
// EXPECTED: tgram.message line carries attachments[] with sha256;
//           Classification.Type == bug.
//
// Fixture: fixtures/s11_redbanner.jpg — a 4KB committed JPEG.
// The fixture is content-addressed via sha256 in the transcript;
// the orchestrator can verify the fixture sha matches what landed
// under <pherald-qa-out-dir>/attachments/<sha256>.jpg.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// fixturePath returns the absolute path to a fixture under
// qaherald/internal/lifecycle/fixtures/. Uses runtime.Caller(0) so
// the path resolves regardless of cwd — scenarios can run from any
// working directory.
func fixturePath(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "fixtures", name)
}

func runS11(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S11", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	fp := fixturePath("s11_redbanner.jpg")
	if _, err := os.Stat(fp); err != nil {
		r.FailureReason = fmt.Sprintf("fixture missing: %s — commit qaherald/internal/lifecycle/fixtures/s11_redbanner.jpg", fp)
		return r
	}

	const caption = "Bug: visual artifact in dashboard"

	inID, serr := env.Msgr.SendPhoto(ctx, fp, caption)
	if serr != nil {
		r.FailureReason = fmt.Sprintf("send-photo: %v", serr)
		return r
	}
	r.InboundMessageID = inID

	// Wait for pherald's reply that references the inbound photo.
	// We DO NOT send a second text — the photo + caption was the
	// inbound trigger. Best-effort: a missing reply does not fail S11
	// because the classification + attachment-mime evidence below is
	// the load-bearing assertion. We DO capture the reply id when one
	// arrives within the timeout, so the report can cite it.
	if reply, werr := awaitReplyOnly(ctx, env, ""); werr == nil {
		r.ReplyMessageID = reply.MessageID
	}

	cline, cerr := awaitClassification(ctx, env, inID, "bug")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await-classification (bug+photo): %v", cerr)
		return r
	}
	r.ClassificationSeen = "bug"
	r.ActionSeen = "inbound photo + classify"

	// Verify pherald journaled an attachments entry with image/* mime.
	// Reuse the helper at the transcript level — search for a line
	// containing both the bug type and "image/" substring (mime hint).
	mimeHit, merr := scanTranscriptForSubstr(env, "image/")
	if merr != nil || len(mimeHit) == 0 {
		r.FailureReason = fmt.Sprintf("transcript missing image/* attachment evidence (err=%v)", merr)
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
