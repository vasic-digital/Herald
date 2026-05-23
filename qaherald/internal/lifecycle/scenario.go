// Package lifecycle implements qaherald's 15-scenario automated
// lifecycle driver (Wave 6.5 T9-auto + qaherald-auto T3..T5).
//
// Scope: each Scenario corresponds 1:1 to a row in
// tests/test_wave6.5_lifecycle.sh (S1..S15). A scenario runs through
// the MessengerClient interface (T2), drives Telegram traffic, then
// asserts on:
//
//  1. pherald's reply text (regex / substring match).
//  2. pherald's own transcript.jsonl (the file pherald listen writes
//     to --qa-out-dir) for §32.6 classification + action verification.
//  3. The fs state of docs/Issues.md + docs/Fixed.md, diffed before vs
//     after the scenario.
//
// §107 anti-bluff anchor: every scenario assertion fails LOUDLY on
// mismatch — no aggregated "all good" reduction. Each PASS in the
// generated report cites the exact inbound message_id, the outbound
// reply message_id, and (where applicable) the raw fs-diff hunk.
//
// Scenarios are pure functions returning a Result. The orchestrator
// (lifecycle.go) sequences them, snapshots fs state before each run,
// passes a shared *Env, and persists the captured Results to the
// Markdown report.
package lifecycle

import (
	"context"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// Scenario is a single lifecycle test case.
//
// RequiresOperatorRole signals scenarios that depend on qa-bot's
// user-id being in HERALD_OPERATOR_IDS (S5/S6/S8/S10/S11/S12).
//
// RequiresNonOperatorBot signals scenarios that need a SECOND bot
// whose user-id is NOT in HERALD_OPERATOR_IDS (S9 only). When the
// non-operator messenger is nil, the orchestrator emits a
// SKIP-with-reason per §11.4.5.
type Scenario struct {
	ID                     string
	Name                   string
	Description            string
	Run                    ScenarioRun
	RequiresOperatorRole   bool
	RequiresNonOperatorBot bool
}

// ScenarioRun is the function signature every S0X file exports.
type ScenarioRun func(ctx context.Context, env *Env) Result

// Env is the per-scenario environment populated by the orchestrator.
//
// The qaherald-side transcript is owned by the orchestrator (not by
// Env) — scenarios do NOT directly write to that file. Per-scenario
// events are emitted by the orchestrator's writeEvent closure before/
// after Run, citing the captured Result.
type Env struct {
	Msgr           messenger.MessengerClient // qa-bot (always)
	MsgrNonOp      messenger.MessengerClient // 2nd qa-bot NOT in OPERATOR_IDS; nil → S9 SKIPs
	PheraldBotUser string                    // pherald-bot username (no @ prefix)
	DocsDir        string                    // docs/ for Issues.md/Fixed.md snapshots
	PheraldQADir   string                    // where pherald listen writes transcript.jsonl
	ChatID         int64
	PerTimeout     time.Duration
	LastOpenedHRD  string // populated by S5/S6 for S8/S10 to consume
}

// Result is the per-scenario outcome consumed by the Markdown report.
//
// FailureReason starting with "SKIP:" is the §11.4.5 SKIP-with-reason
// convention; the report renders such results as SKIP rather than
// FAIL and excludes them from the FAIL count.
type Result struct {
	ScenarioID         string
	PASS               bool
	FailureReason      string
	InboundMessageID   messenger.MessageID
	ReplyMessageID     messenger.MessageID
	ClassificationSeen string
	ActionSeen         string
	Evidence           []EvidenceFragment
	StartedAt          time.Time
	Duration           time.Duration
}

// EvidenceFragment carries one anti-bluff anchor — a raw bytes blob
// (reply text, JSONL classification line, fs-diff hunk, sha256) the
// report quotes verbatim.
type EvidenceFragment struct {
	Kind    string
	Content string
}

// Registry returns the 15 scenarios in S01..S15 order.
//
// Each entry binds the per-scenario Go func; the orchestrator filters
// by --scenarios flag and runs the remaining scenarios sequentially.
//
// §107 anchor: the slice length MUST equal 15. The unit test
// TestRegistry_15Scenarios asserts this. A Wave 7 mutation that drops
// a scenario silently would fail that test.
func Registry() []Scenario {
	return []Scenario{
		{ID: "S01", Name: "plain-greeting-query-fallthrough", Description: "Plain greeting → query fallthrough → CC dispatch (Confidence:0)", Run: runS01},
		{ID: "S02", Name: "help-fastpath", Description: "Help: fast-path → BuiltinHelp catalogue (no CC)", Run: runS02},
		{ID: "S03", Name: "status-fastpath", Description: "Status: fast-path → docs/Status.md (no CC)", Run: runS03},
		{ID: "S04", Name: "continue-fastpath", Description: "Continue: fast-path → docs/CONTINUATION.md (no CC)", Run: runS04},
		{ID: "S05", Name: "bug-prefix-cc-issue-open", Description: "Bug: prefix → CC issue.open path → HRD-NNN appended to Issues.md", Run: runS05, RequiresOperatorRole: true},
		{ID: "S06", Name: "task-prefix-cc-issue-open", Description: "Task: prefix → task classification → HRD-NNN appended", Run: runS06, RequiresOperatorRole: true},
		{ID: "S07", Name: "query-prefix-cc-research", Description: "Query: prefix → explicit query classification (Confidence:1)", Run: runS07},
		{ID: "S08", Name: "done-operator-migrate", Description: "Done: HRD-NNN by operator → Issues→Fixed atomic migration", Run: runS08, RequiresOperatorRole: true},
		{ID: "S09", Name: "done-non-operator-reject", Description: "Done: HRD-NNN from NON-operator bot → rejection reply", Run: runS09, RequiresNonOperatorBot: true},
		{ID: "S10", Name: "reopen-operator-migrate", Description: "Reopen: HRD-NNN by operator → Fixed→Issues migration", Run: runS10, RequiresOperatorRole: true},
		{ID: "S11", Name: "inbound-photo-bug-caption", Description: "Inbound photo + Bug: caption → image/* attachment + bug classification", Run: runS11, RequiresOperatorRole: true},
		{ID: "S12", Name: "inbound-document-task-caption", Description: "Inbound document + Task: caption → application/* attachment + task", Run: runS12, RequiresOperatorRole: true},
		{ID: "S13", Name: "inbound-voice-audio", Description: "Inbound voice/audio attachment captured with sha256", Run: runS13},
		{ID: "S14", Name: "outbound-attachment-fanout", Description: "Outbound attachment via SendReply fan-out", Run: runS14},
		{ID: "S15", Name: "natural-language-emoji-fallthrough", Description: "Natural-language + emojis → query fallthrough (Confidence:0)", Run: runS15},
	}
}

// ByName resolves a scenario by name. Returns (Scenario{}, false) on
// miss. Used by ad-hoc CLI tooling; the orchestrator filters by ID via
// the registry slice directly.
func ByName(name string) (Scenario, bool) {
	for _, s := range Registry() {
		if s.Name == name {
			return s, true
		}
	}
	return Scenario{}, false
}

// ByID resolves a scenario by ID (e.g. "S01"). Returns
// (Scenario{}, false) on miss.
func ByID(id string) (Scenario, bool) {
	for _, s := range Registry() {
		if s.ID == id {
			return s, true
		}
	}
	return Scenario{}, false
}
