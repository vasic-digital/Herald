// Orchestrator (T4) — ties scenarios + messenger + transcript + report
// together into a single Run entry point that the Cobra command calls.
//
// Flow:
//
//  1. Construct MessengerClient(s) from cfg (operator bot always; non-
//     operator bot when its token is set).
//  2. Open the qaherald-side transcript writer.
//  3. Run preflight (Preflight() against the bot endpoint).
//  4. Filter scenarios by cfg.Scenarios.
//  5. For each scenario:
//     a. Capture pherald-transcript file offset at scenario start.
//     b. Write `scenario.start` event to qaherald transcript.
//     c. Run the scenario with a per-scenario timeout context.
//     d. Write `scenario.end` event with the captured Result.
//  6. Generate the Markdown report.
//  7. Return non-nil if any scenario FAILed (SKIP does NOT cause
//     non-zero exit).
//
// §107: every IO step writes a transcript event before/after — even
// FAIL paths emit a `scenario.end` line. The orchestrator NEVER
// returns early on the first FAIL; it runs every scheduled scenario
// so a single early flake does not hide later failures.
package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/messenger"
)

// Config is the orchestrator input. Mirrors the Cobra flag surface
// in lifecycle.go (cmd) with the addition of OverallTimeout and the
// derived RunID.
type Config struct {
	QABotToken         string
	QABotTokenNonOp    string
	ChatID             int64
	PheraldBotUsername string
	PheraldBotUserID   int64
	OutDir             string
	RunID              string
	DocsDir            string
	PheraldQAOutDir    string
	Scenarios          []string
	PerScenarioTimeout time.Duration
	SkipPreflight      bool
	BotAPIBaseURL      string // empty → defaultBotAPIBaseURL; tests pass httptest URL
}

// Run executes the lifecycle. Returns nil iff every SCHEDULED
// scenario PASSed (SKIPs counted as PASS for exit-code purposes,
// FAILs cause non-nil error).
//
// The error path is structured: "%d/%d scenarios FAILed — see
// <out>/report.md". The Cobra RunE surfaces this as an HRD-101
// non-zero exit.
func Run(ctx context.Context, cfg Config) error {
	if cfg.OutDir == "" {
		return errors.New("lifecycle.Run: OutDir is required")
	}
	if err := os.MkdirAll(filepath.Join(cfg.OutDir, "attachments"), 0o755); err != nil {
		return fmt.Errorf("mkdir out-dir: %w", err)
	}

	msgr, err := buildMessenger(cfg.QABotToken, cfg.ChatID, cfg.BotAPIBaseURL)
	if err != nil {
		return fmt.Errorf("build qa-bot client: %w", err)
	}
	defer msgr.Close()

	var msgrNonOp messenger.MessengerClient
	if cfg.QABotTokenNonOp != "" {
		msgrNonOp, err = buildMessenger(cfg.QABotTokenNonOp, cfg.ChatID, cfg.BotAPIBaseURL)
		if err != nil {
			return fmt.Errorf("build qa-bot-non-op client: %w", err)
		}
		defer msgrNonOp.Close()
	}

	// Open the qaherald-side transcript writer rooted at OutDir.
	// Wave 5's transcript.NewWriter creates a child dir named after
	// its OWN run-id; for lifecycle we want events to land directly
	// under cfg.OutDir/transcript.jsonl. Open a raw file instead.
	transcriptPath := filepath.Join(cfg.OutDir, "transcript.jsonl")
	tf, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("transcript writer: %w", err)
	}
	defer tf.Close()
	tjEnc := json.NewEncoder(tf)

	writeEvent := func(direction, kind string, payload any) {
		ev := struct {
			TS        string `json:"ts"`
			Direction string `json:"direction"`
			Kind      string `json:"kind"`
			Payload   any    `json:"payload"`
		}{
			TS:        time.Now().UTC().Format(time.RFC3339Nano),
			Direction: direction,
			Kind:      kind,
			Payload:   payload,
		}
		_ = tjEnc.Encode(ev)
		_ = tf.Sync()
	}

	if !cfg.SkipPreflight {
		if err := runPreflight(ctx, msgr, msgrNonOp, cfg, writeEvent); err != nil {
			return fmt.Errorf("preflight: %w", err)
		}
	}

	env := &Env{
		Msgr:           msgr,
		MsgrNonOp:      msgrNonOp,
		PheraldBotUser: cfg.PheraldBotUsername,
		DocsDir:        cfg.DocsDir,
		PheraldQADir:   cfg.PheraldQAOutDir,
		ChatID:         cfg.ChatID,
		PerTimeout:     cfg.PerScenarioTimeout,
	}

	scenarios := Registry()
	if len(cfg.Scenarios) > 0 {
		scenarios = filterScenarios(scenarios, cfg.Scenarios)
	}

	results := make([]Result, 0, len(scenarios))
	for _, s := range scenarios {
		// Pre-flight SKIP for S09 when MsgrNonOp is nil.
		if s.RequiresNonOperatorBot && msgrNonOp == nil {
			res := Result{
				ScenarioID:    s.ID,
				FailureReason: "SKIP: HERALD_QA_BOT_TOKEN_NON_OPERATOR unset (§11.4.5)",
				StartedAt:     time.Now(),
			}
			results = append(results, res)
			writeEvent("internal", "scenario.skip", map[string]string{
				"id": s.ID, "name": s.Name, "reason": res.FailureReason,
			})
			continue
		}

		// Capture pherald-transcript start offset so scenario helpers
		// only see events emitted DURING this scenario.
		startOff := transcriptFileOffset(env)
		scenarioStartOffset.Store(env, startOff)

		writeEvent("internal", "scenario.start", map[string]any{
			"id":            s.ID,
			"name":          s.Name,
			"description":   s.Description,
			"pherald_start": startOff,
		})

		sctx, cancel := context.WithTimeout(ctx, cfg.PerScenarioTimeout)
		fmt.Fprintf(os.Stderr, "[lifecycle] running %s — %s\n", s.ID, s.Name)
		result := s.Run(sctx, env)
		cancel()

		// Carry env.LastOpenedHRD between scenarios (S05/S06 → S08/S10).
		// runScenario already wrote to env.LastOpenedHRD via the
		// closure; nothing more to do here.

		results = append(results, result)

		writeEvent("internal", "scenario.end", map[string]any{
			"id":              s.ID,
			"pass":            result.PASS,
			"failure_reason":  result.FailureReason,
			"inbound":         result.InboundMessageID,
			"reply":           result.ReplyMessageID,
			"classification":  result.ClassificationSeen,
			"action":          result.ActionSeen,
			"duration_millis": result.Duration.Milliseconds(),
		})
	}

	if err := writeReport(filepath.Join(cfg.OutDir, "report.md"), cfg.RunID, msgr, cfg.PheraldBotUsername, results); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	failed := 0
	for _, r := range results {
		if !r.PASS && !strings.HasPrefix(r.FailureReason, "SKIP:") {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d scenarios FAILed — see %s/report.md", failed, len(results), cfg.OutDir)
	}
	return nil
}

// buildMessenger constructs a messenger.MessengerClient from a token
// and chat-id. BotAPIBaseURL is forwarded; tests pass an httptest
// URL so all wire-calls are observable.
//
// The token is never logged.
func buildMessenger(token string, chatID int64, baseURL string) (messenger.MessengerClient, error) {
	return messenger.NewTelegramClient(token, chatID, baseURL)
}

// filterScenarios returns the subset of `all` whose IDs are in
// `subset`. Case-insensitive, whitespace-trimmed. Unknown IDs are
// silently ignored (operator typos surface as "fewer scenarios ran
// than expected" in the report).
func filterScenarios(all []Scenario, subset []string) []Scenario {
	keep := map[string]bool{}
	for _, s := range subset {
		keep[strings.ToUpper(strings.TrimSpace(s))] = true
	}
	out := make([]Scenario, 0, len(all))
	for _, s := range all {
		if keep[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

// GenerateRunID returns a fresh run-id. RFC3339-ish + 4 hex chars.
// Used by the Cobra command when --run-id is unset.
func GenerateRunID() string {
	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ts + "-0000"
	}
	return ts + "-" + hex.EncodeToString(b[:])
}

// PrintScenarios writes the full scenario catalogue (ID + name +
// description) to out. Cobra --manual mode calls this.
func PrintScenarios(out io.Writer) error {
	for _, s := range Registry() {
		_, _ = fmt.Fprintf(out, "%s\t%s\n        %s\n", s.ID, s.Name, s.Description)
	}
	return nil
}
