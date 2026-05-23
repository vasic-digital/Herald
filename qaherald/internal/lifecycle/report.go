// Report generator (T5) — emits docs/qa/<run-id>/report.md.
//
// Every PASS section MUST cite:
//   - inbound message_id (qa-bot → chat)
//   - reply message_id (pherald → chat)
//   - classification (from pherald's transcript JSONL line)
//   - action (cc.dispatch / tgram.send_reply / atomic-migration)
//   - evidence fragments (reply text, transcript line, fs-diff hunk)
//
// FAIL sections cite the FailureReason verbatim PLUS whatever
// partial evidence the scenario captured before failing — never
// "all good" / never "see logs".
//
// SKIP sections render distinctly (SKIP badge + reason line + skip
// count distinct from FAIL in summary).
package lifecycle

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// writeReport renders the Markdown report at `path`.
//
// The msgr argument is consumed to look up the qa-bot's username for
// the header attribution. PheraldBotUsername is taken directly from
// cfg.
//
// Returns a non-nil error only on filesystem failure; per-scenario
// PASS/FAIL is recorded inside the report body, not bubbled up.
func writeReport(path, runID string, msgr messenger.MessengerClient, pheraldBotUsername string, results []Result) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report.md: %w", err)
	}
	defer f.Close()

	qaUser := ""
	if msgr != nil {
		// Best-effort getMe lookup — Me() is cached on the messenger
		// after the first Preflight call, so this is free.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		u, _, merr := msgr.Me(ctx)
		cancel()
		if merr == nil {
			qaUser = u
		}
	}

	fmt.Fprintf(f, "# Lifecycle Report — run-id `%s`\n\n", runID)
	fmt.Fprintf(f, "Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(f, "- **qa-bot:** `%s`\n", qaUser)
	fmt.Fprintf(f, "- **pherald-bot:** `%s`\n", pheraldBotUsername)
	fmt.Fprintf(f, "- **scenarios scheduled:** %d\n\n", len(results))

	var passN, failN, skipN int
	for _, r := range results {
		switch {
		case strings.HasPrefix(r.FailureReason, "SKIP:"):
			skipN++
		case r.PASS:
			passN++
		default:
			failN++
		}
	}
	fmt.Fprintf(f, "**Summary:** %d PASS / %d FAIL / %d SKIP (of %d scenarios)\n\n", passN, failN, skipN, len(results))

	fmt.Fprintf(f, "## §107 anti-bluff posture\n\n")
	fmt.Fprintf(f, "Every PASS line below cites the inbound `message_id` (qa-bot → chat), ")
	fmt.Fprintf(f, "the outbound reply `message_id` (pherald → chat), the §32.6 classification observed ")
	fmt.Fprintf(f, "in pherald's own `transcript.jsonl`, and (for fs-mutation scenarios) the rendered ")
	fmt.Fprintf(f, "Issues.md / Fixed.md diff hunk. No PASS is rendered without a concrete evidence anchor.\n\n")

	fmt.Fprintf(f, "## Per-Scenario Detail\n\n")
	for _, r := range results {
		writeScenarioSection(f, r)
	}

	writeAggregate(f, results)
	return nil
}

// writeScenarioSection renders one scenario block.
func writeScenarioSection(w io.Writer, r Result) {
	status := "PASS"
	switch {
	case strings.HasPrefix(r.FailureReason, "SKIP:"):
		status = "SKIP"
	case !r.PASS:
		status = "FAIL"
	}

	fmt.Fprintf(w, "### %s — %s\n\n", r.ScenarioID, status)
	fmt.Fprintf(w, "- **Duration:** %s\n", r.Duration)
	if r.InboundMessageID != "" {
		fmt.Fprintf(w, "- **Inbound message_id:** `%s`\n", r.InboundMessageID)
	}
	if r.ReplyMessageID != "" {
		fmt.Fprintf(w, "- **Reply message_id:** `%s`\n", r.ReplyMessageID)
	}
	if r.ClassificationSeen != "" {
		fmt.Fprintf(w, "- **Classification:** `%s`\n", r.ClassificationSeen)
	}
	if r.ActionSeen != "" {
		fmt.Fprintf(w, "- **Action:** `%s`\n", r.ActionSeen)
	}
	if r.FailureReason != "" {
		fmt.Fprintf(w, "- **Reason:** %s\n", r.FailureReason)
	}
	if len(r.Evidence) > 0 {
		fmt.Fprintf(w, "\n#### Evidence\n\n")
		for _, e := range r.Evidence {
			fmt.Fprintf(w, "**%s:**\n\n```\n%s\n```\n\n", e.Kind, truncate(e.Content, 2000))
		}
	}
	fmt.Fprintln(w)
}

// writeAggregate renders the bottom-of-report at-a-glance table.
func writeAggregate(w io.Writer, results []Result) {
	fmt.Fprintf(w, "## Aggregate\n\n")
	fmt.Fprintf(w, "| Scenario | Status | Duration | Inbound | Reply | Classification |\n")
	fmt.Fprintf(w, "|---|---|---|---|---|---|\n")
	for _, r := range results {
		status := "PASS"
		switch {
		case strings.HasPrefix(r.FailureReason, "SKIP:"):
			status = "SKIP"
		case !r.PASS:
			status = "FAIL"
		}
		fmt.Fprintf(w, "| %s | %s | %s | `%s` | `%s` | `%s` |\n",
			r.ScenarioID, status, r.Duration, r.InboundMessageID, r.ReplyMessageID, r.ClassificationSeen)
	}
}

// truncate bounds long evidence fragments so the report renders
// cleanly in markdown previewers. Truncated bytes are explicit so
// the reader knows there is more.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "... (truncated)"
}
