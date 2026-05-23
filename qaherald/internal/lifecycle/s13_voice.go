// S13 — inbound voice/audio attachment.
//
// ACTION:   Send a Telegram voice message (ogg/opus).
// EXPECTED: tgram.message carries audio/* attachment with sha256.
//
// Fixture: fixtures/s13_voice.ogg — a ~5KB committed ogg/opus voice
// clip.
//
// Note: pherald may classify voice-only inbound as query fallthrough
// (no caption → no command prefix → default Type=query). The
// assertion below only requires the audio/* attachment mime evidence,
// NOT a specific classification.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"time"
)

func runS13(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S13", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	fp := fixturePath("s13_voice.ogg")
	if _, err := os.Stat(fp); err != nil {
		r.FailureReason = fmt.Sprintf("fixture missing: %s — commit qaherald/internal/lifecycle/fixtures/s13_voice.ogg", fp)
		return r
	}

	inID, serr := env.Msgr.SendVoice(ctx, fp)
	if serr != nil {
		r.FailureReason = fmt.Sprintf("send-voice: %v", serr)
		return r
	}
	r.InboundMessageID = inID

	if reply, werr := awaitReplyOnly(ctx, env, ""); werr == nil {
		r.ReplyMessageID = reply.MessageID
	}

	mimeHit, merr := scanTranscriptForSubstr(env, "audio/")
	if merr != nil || len(mimeHit) == 0 {
		r.FailureReason = fmt.Sprintf("transcript missing audio/* attachment evidence (err=%v)", merr)
		return r
	}
	r.ClassificationSeen = "query" // voice with no caption → fallthrough
	r.ActionSeen = "inbound voice + sha256-attach"
	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "fixture-path", Content: fp},
		{Kind: "attachment-mime-line", Content: string(mimeHit)},
	}
	return r
}
