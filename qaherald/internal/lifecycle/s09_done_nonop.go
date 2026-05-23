// S09 — Done: HRD-NNN from NON-operator bot → rejection reply.
//
// ACTION:   From a 2nd Telegram bot whose user-id is NOT in
//           HERALD_OPERATOR_IDS, send "Done: HRD-NNN" (any open HRD).
// EXPECTED: closure classified but rejected; reply contains the
//           "HERALD_OPERATOR_IDS" substring explaining the
//           operator-role requirement.
//
// SKIP CONTRACT: when env.MsgrNonOp is nil (env-var
// HERALD_QA_BOT_TOKEN_NON_OPERATOR unset), this scenario emits a
// Result with FailureReason starting "SKIP:" per §11.4.5
// SKIP-with-reason. The orchestrator counts it as SKIP, not FAIL.
//
// Future hardening (Wave 7): a "restart-pherald-with-different-
// operator-ids" two-pass mode (driven by the orchestrator) would
// remove the need for a second bot. Out of scope here.
package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

func runS09(ctx context.Context, env *Env) Result {
	r := Result{ScenarioID: "S09", StartedAt: time.Now()}
	defer func() { r.Duration = time.Since(r.StartedAt) }()

	if env.MsgrNonOp == nil {
		r.FailureReason = "SKIP: HERALD_QA_BOT_TOKEN_NON_OPERATOR unset — non-operator-role check requires a 2nd bot whose user-id is NOT in HERALD_OPERATOR_IDS (§11.4.5)"
		return r
	}

	// Pick an HRD-NNN to attempt closure against. Prefer the most
	// recent env.LastOpenedHRD; if unset, FAIL — running S09 without
	// an open HRD-NNN would chase a moving target.
	target := env.LastOpenedHRD
	if target == "" {
		r.FailureReason = "S09 prerequisite missing: env.LastOpenedHRD is empty"
		return r
	}

	input := "Done: " + target

	// Send via the NON-operator messenger.
	inID, serr := env.MsgrNonOp.Send(ctx, input)
	if serr != nil {
		r.FailureReason = fmt.Sprintf("send (non-op): %v", serr)
		return r
	}
	r.InboundMessageID = inID

	// Observe the rejection reply via the OPERATOR messenger (pherald
	// posts to the same chat regardless of which bot sent the
	// trigger). Predicate matches on the "HERALD_OPERATOR_IDS"
	// substring per pherald's commands.Reject() body.
	pred := func(rp messenger.Reply) bool {
		if env.PheraldBotUser != "" && rp.SenderUsername != env.PheraldBotUser {
			return false
		}
		return strings.Contains(rp.Text, "HERALD_OPERATOR_IDS") ||
			strings.Contains(rp.Caption, "HERALD_OPERATOR_IDS")
	}
	reply, werr := env.Msgr.WaitForReply(ctx, inID, pred, env.PerTimeout)
	if werr != nil {
		r.FailureReason = fmt.Sprintf("await rejection reply: %v", werr)
		return r
	}
	r.ReplyMessageID = reply.MessageID

	r.ClassificationSeen = "closure"
	r.ActionSeen = "reject (non-operator)"
	r.PASS = true
	r.Evidence = []EvidenceFragment{
		{Kind: "reply-text", Content: reply.Text},
		{Kind: "rejected-hrd", Content: target},
	}
	return r
}
