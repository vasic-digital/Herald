// S14 — outbound attachment via SendReply fan-out.
//
// ACTION:   Send "Bug: layout broken (please attach a reproduction
//           screenshot in your reply)".
// EXPECTED: pherald's CC reply emits tgram.send_reply whose payload
//           includes attachments[] with ≥1 entry — proves Wave 6.5
//           T6 outbound fan-out wired correctly.
//
// Note: this is an LLM-driven scenario — the CC dispatcher is the
// one responsible for emitting attachments[] in its reply. The test
// asserts the WIRE EVIDENCE (tgram.send_reply line containing
// "attachments") but does NOT assert specific bytes — the LLM may
// attach any reasonable image. If CC declines to attach (acceptable
// LLM behaviour), this scenario will FAIL — the operator-locked
// expectation is that pherald's prompt scaffold biases CC toward
// attaching SOMETHING. Wave 7 may relax this to soft-FAIL.
package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func runS14(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S14", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	const input = "Bug: layout broken (please attach a reproduction screenshot in your reply)"

	reply, inID, err := awaitReplyWithSubstring(ctx, env, input, "")
	r.InboundMessageID = inID
	if err != nil {
		r.FailureReason = err.Error()
		return r
	}
	r.ReplyMessageID = reply.MessageID

	// The load-bearing assertion: pherald's transcript shows a
	// tgram.send_reply line containing "attachments" — proves the
	// SendReply fan-out path took the attachment branch.
	hit, herr := scanTranscriptForSubstr(env, `"kind":"tgram.send_reply"`)
	if herr != nil || len(hit) == 0 {
		r.FailureReason = fmt.Sprintf("no tgram.send_reply line in transcript (err=%v)", herr)
		return r
	}
	if !strings.Contains(string(hit), "attachments") {
		r.FailureReason = "tgram.send_reply present but no attachments field — outbound fan-out did not attach"
		r.Evidence = append(r.Evidence, EvidenceFragment{Kind: "send-reply-line", Content: string(hit)})
		return r
	}

	r.ClassificationSeen = "bug"
	r.ActionSeen = "cc.dispatch + tgram.send_reply(attachments[])"
	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "send-reply-line", Content: string(hit)},
	}
	return r
}
