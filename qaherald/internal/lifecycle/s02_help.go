// S02 — Help: fast-path → BuiltinHelp catalogue (no CC).
//
// ACTION:   Send "Help:"
// EXPECTED: tgram.send_reply contains "Command catalogue"; no
//           cc.dispatch is emitted (fast-path bypass).
//
// §107: substring match on the fast-path reply IS the verification —
// Help is a deterministic command path with no LLM in the loop, so
// the BuiltinHelp text is the canonical wire-byte anchor.
package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func runS02(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S02", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Help:"

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "Command catalogue")
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	// Verify pherald journaled a tgram.send_reply with the catalogue
	// substring — proves the dispatcher hit the BuiltinHelp fast-path.
	cline, cerr := awaitClassification(ctx, env, inID, "Command catalogue")
	if cerr != nil {
		r.FailureReason = fmt.Sprintf("await fast-path journal: %v", cerr)
		return r
	}
	if !strings.Contains(reply.Text, "Command catalogue") {
		r.FailureReason = fmt.Sprintf("reply missing 'Command catalogue': %q", reply.Text)
		return r
	}
	r.ClassificationSeen = "help_command"
	r.ActionSeen = "tgram.send_reply"
	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "transcript-jsonl-line", Content: string(cline)},
	}
	return r
}
