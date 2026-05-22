// Wave 5 Task 7 — `qaherald run` Cobra subcommand wiring.
//
// Flags + env-var fallbacks; instantiates Transcript, Telegram client,
// Herald REST client; iterates scenarios per `--scenario=<name|all>`;
// persists results.json; calls report.Generate at suite end; exits
// non-zero if any scenario FAILed.
//
// §107 anti-bluff anchor: exit code MUST surface every scenario FAIL
// verbatim. The runScenarios helper is extracted so the integration
// test in run_test.go can drive the orchestration directly (with
// fakeTGSession + httptest-backed *herald.Client) and assert opposite
// outcomes for canned-PASS and canned-FAIL scenarios — proving the
// exit-code propagation without needing a live Telegram bot or real
// pherald.
//
// The hermetic test design (extracted runScenarios + canned scenarios)
// is the §107 hook that, when the T10 mutation gate (b) plants the
// always-202 Herald.PostEvent stub, lets the deny-path scenario's
// status assertion FAIL → runScenarios returns a non-nil error → the
// exit-code path surfaces it. The same propagation is exercised here
// against a synthetic FAIL-by-construction scenario so the test does
// not depend on the wider live stack.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/report"
	"github.com/vasic-digital/herald/qaherald/internal/scenario"
	"github.com/vasic-digital/herald/qaherald/internal/tgram"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// run-subcommand flags. Package-scope so the test in run_test.go can
// reset them between sub-tests without re-creating the Cobra command.
var (
	flagScenario    string
	flagOutDirRoot  string
	flagHeraldURL   string
	flagHeraldToken string
	flagTGToken     string
	flagTGChatID    int64
	flagTimeout     time.Duration
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run qaherald scenarios against the configured pherald + Telegram bot",
	Long: `qaherald run drives the configured set of qaherald scenarios end-to-end:
constructs CloudEvents, POSTs them via HTTPS+JWT (with TOON content
negotiation per Wave 4b), observes Telegram-side delivery, records a
bidirectional transcript + content-addressed attachments under
docs/qa/<run-id>/, and emits a Markdown report.

Per Herald Constitution §107.x (operator mandate, 2026-05-22), every
shipped Herald feature MUST carry a docs/qa/<run-id>/ evidence
artefact. This subcommand is the canonical producer.

Exit code: 0 when every scenario PASSes, non-zero on any FAIL —
§107 anti-bluff: the exit code is the contract every CI gate keys
off, so a silent FAIL would be a §11.4 PASS-bluff at the harness
layer.`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().StringVar(&flagScenario, "scenario", "all",
		"scenario name (e.g. happy-path-single-channel) or 'all'")
	runCmd.Flags().StringVar(&flagOutDirRoot, "out-dir", "docs/qa",
		"parent directory for the <run-id>/ subdirectory")
	runCmd.Flags().StringVar(&flagHeraldURL, "herald-url",
		envOr("HERALD_BASE_URL", "https://localhost:7443"),
		"pherald base URL (env: HERALD_BASE_URL)")
	runCmd.Flags().StringVar(&flagHeraldToken, "herald-token",
		envOr("HERALD_JWT_SECRET", ""),
		"JWT HMAC secret for pherald (env: HERALD_JWT_SECRET) — REQUIRED")
	runCmd.Flags().StringVar(&flagTGToken, "tg-token",
		envOr("HERALD_TGRAM_BOT_TOKEN", ""),
		"Telegram bot token (env: HERALD_TGRAM_BOT_TOKEN) — REQUIRED")
	runCmd.Flags().Int64Var(&flagTGChatID, "tg-chat",
		envOrInt("HERALD_TGRAM_CHAT_ID", 0),
		"Telegram chat ID (env: HERALD_TGRAM_CHAT_ID) — REQUIRED")
	runCmd.Flags().DurationVar(&flagTimeout, "scenario-timeout",
		60*time.Second,
		"per-scenario timeout (each scenario gets its own context)")
	rootCmd.AddCommand(runCmd)
}

// envOr returns os.Getenv(name) when non-empty, else def. Used by the
// flag definitions to make every flag env-var-resolvable.
func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// envOrInt returns int64(os.Getenv(name)) when parseable, else def.
func envOrInt(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// runRun is the Cobra RunE entrypoint. It validates required flags,
// builds the Orchestrator from real Telegram + Herald clients, resolves
// the scenario set, delegates to runScenarios, then writes results.json
// + report.md. Returns a non-nil error (which Cobra propagates to
// os.Exit(1)) when any scenario FAILed or any orchestration step
// errored.
func runRun(cmd *cobra.Command, args []string) error {
	if flagHeraldToken == "" {
		return fmt.Errorf("--herald-token (or HERALD_JWT_SECRET) is required")
	}
	if flagTGToken == "" {
		return fmt.Errorf("--tg-token (or HERALD_TGRAM_BOT_TOKEN) is required")
	}
	if flagTGChatID == 0 {
		return fmt.Errorf("--tg-chat (or HERALD_TGRAM_CHAT_ID) is required")
	}

	tw, err := transcript.NewWriter(flagOutDirRoot)
	if err != nil {
		return fmt.Errorf("transcript.NewWriter: %w", err)
	}
	// Note: tw.Close is called explicitly BEFORE report.Generate so the
	// JSONL file is flushed + closed and report.Generate's bufio scanner
	// sees the final state of the file. A defer here would run AFTER
	// the report generation and risk a half-flushed transcript.
	fmt.Printf("qaherald run id: %s\n", tw.RunID())

	tg, err := tgram.NewClient(flagTGToken, flagTGChatID)
	if err != nil {
		_ = tw.Close()
		return fmt.Errorf("tgram.NewClient: %w", err)
	}
	defer tg.Close()

	hc := herald.New(flagHeraldURL, []byte(flagHeraldToken))

	orch := &scenario.Orchestrator{
		TG:         tg,
		Herald:     hc,
		Transcript: tw,
		ChatID:     flagTGChatID,
		Now:        func() time.Time { return time.Now().UTC() },
	}

	scenarios, err := resolveScenarios(flagScenario)
	if err != nil {
		_ = tw.Close()
		return err
	}

	// cmd.Context() may be nil in some Cobra harnesses (e.g. tests that
	// build runCmd without piping a context through Execute). Fall back
	// to context.Background() so the per-scenario timeout still applies.
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	results, runErr := runScenarios(parentCtx, orch, scenarios, flagTimeout)

	// Always close the transcript writer before generating the report —
	// report.Generate re-opens the JSONL file from disk; an unflushed
	// writer would yield a partial report and a noisy §107 evidence
	// artefact.
	if cerr := tw.Close(); cerr != nil && runErr == nil {
		runErr = fmt.Errorf("transcript.Close: %w", cerr)
	}

	if err := writeResults(tw.OutDir(), results); err != nil {
		// Persisting results.json is best-effort — surface the error but
		// do not mask any pre-existing scenario FAIL.
		if runErr == nil {
			runErr = err
		} else {
			fmt.Fprintf(os.Stderr, "qaherald: writeResults: %v\n", err)
		}
	}
	if err := report.Generate(
		filepath.Join(tw.OutDir(), "transcript.jsonl"),
		filepath.Join(tw.OutDir(), "results.json"),
		filepath.Join(tw.OutDir(), "report.md"),
	); err != nil {
		if runErr == nil {
			runErr = fmt.Errorf("report.Generate: %w", err)
		} else {
			fmt.Fprintf(os.Stderr, "qaherald: report.Generate: %v\n", err)
		}
	}

	return runErr
}

// resolveScenarios returns the slice of scenarios named by `name`. The
// special value "all" expands to scenario.All(). Returns an error when
// `name` is neither "all" nor a registered scenario.
func resolveScenarios(name string) ([]scenario.Scenario, error) {
	if name == "all" {
		return scenario.All(), nil
	}
	s, ok := scenario.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown scenario: %s (registered: %v)",
			name, scenario.Names())
	}
	return []scenario.Scenario{s}, nil
}

// runScenarios executes each scenario in turn against the supplied
// Orchestrator, with a per-scenario timeout. Returns the aggregate
// Result[] and a non-nil error when any scenario FAILed (so Cobra can
// surface os.Exit(1)).
//
// Extracted as a package-level function so the integration test in
// run_test.go can call it directly with fakes — proving the exit-code
// contract without needing live services or Cobra flag mocking.
//
// §107 anti-bluff: a FAIL here MUST propagate as a non-nil error.
// runRun calls writeResults + report.Generate AFTER consuming the
// returned error, so a partial run still produces auditable evidence
// even when the suite exits non-zero.
func runScenarios(
	ctx context.Context,
	orch *scenario.Orchestrator,
	scenarios []scenario.Scenario,
	timeout time.Duration,
) ([]scenario.Result, error) {
	results := make([]scenario.Result, 0, len(scenarios))
	for _, s := range scenarios {
		// Each scenario gets its own bounded context so a single
		// scenario stall does not starve the remainder.
		scenarioCtx, cancel := context.WithTimeout(ctx, timeout)
		r := orch.RunScenario(scenarioCtx, s)
		cancel()
		results = append(results, r)
		verdict := "PASS"
		if !r.PASS {
			verdict = "FAIL"
		}
		// One-line-per-scenario verdict for the CLI operator. The
		// machine-readable form lives in results.json.
		fmt.Printf("%s: %s (%s) %s\n",
			r.Scenario, verdict, r.Duration, r.ErrorText)
	}
	if n := failCount(results); n > 0 {
		return results, fmt.Errorf("%d scenario(s) FAILed", n)
	}
	return results, nil
}

// failCount returns the number of FAILed scenarios in results.
func failCount(results []scenario.Result) int {
	n := 0
	for _, r := range results {
		if !r.PASS {
			n++
		}
	}
	return n
}

// writeResults marshals results to <outDir>/results.json with 2-space
// indent so the file is human-diffable when committed under
// docs/qa/<run-id>/.
func writeResults(outDir string, results []scenario.Result) error {
	path := filepath.Join(outDir, "results.json")
	body, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal results: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
