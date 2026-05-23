// qaherald lifecycle T1 — Cobra subcommand unit tests.
//
// Each test constructs a fresh *cobra.Command via newLifecycleCmd(),
// drives it with cobra.Command.SetArgs(...) + Execute(), and asserts on
// the captured *lifecycleFlags struct fields — NOT on stdout. This is
// the §107 anti-bluff posture: the test confirms the actual resolved
// flag state, not "command compiled" or "no error printed".
//
// Hermetic env handling: t.Setenv() scopes env-var overrides to the
// subtest, so cases that exercise env fallbacks do not leak into
// sibling cases. We explicitly UNSET HERALD_QA_BOT_TOKEN and friends in
// the "all-flags-set" + missing-required cases to prevent ambient
// developer env from masking the assertions.
//
// Security mandate: test fixtures use SYNTHETIC tokens like
// "test:TOKEN" and "test:NONOPTOKEN". Never put a real bot token in a
// test fixture; never log the resolved QABotToken field — the assertion
// uses an equality check against the synthetic.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// resetLifecycleEnv clears every lifecycle-relevant env var on the
// current process. Tests that don't t.Setenv a var still need that var
// to be EMPTY so the validator fires. t.Setenv() with "" achieves
// exactly that and auto-restores at test end.
func resetLifecycleEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"HERALD_QA_BOT_TOKEN",
		"HERALD_QA_BOT_TOKEN_NON_OPERATOR",
		"HERALD_TGRAM_CHAT_ID",
		"HERALD_PHERALD_BOT_USERNAME",
		"HERALD_QA_OUT_DIR",
	} {
		t.Setenv(k, "")
	}
}

// chdirToTemp chdirs to t.TempDir() so the RunE's MkdirAll("docs/qa/...")
// does not pollute the repo working tree. t.Cleanup restores cwd.
func chdirToTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

// TestLifecycleFlagsResolveFromEnv exercises the env-fallback path for
// each REQUIRED + OPTIONAL env-backed flag. Per-case t.Setenv sets the
// env var; the test then Execute()s the command with no CLI flags and
// asserts the captured struct field matches the env value.
func TestLifecycleFlagsResolveFromEnv(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		assertFn  func(t *testing.T, f *lifecycleFlags)
		expectErr bool
	}{
		{
			name: "all-required-from-env",
			env: map[string]string{
				"HERALD_QA_BOT_TOKEN":         "test:TOKEN",
				"HERALD_TGRAM_CHAT_ID":        "-100123456789",
				"HERALD_PHERALD_BOT_USERNAME": "atmosphere_worker_bot",
			},
			assertFn: func(t *testing.T, f *lifecycleFlags) {
				if f.QABotToken != "test:TOKEN" {
					t.Errorf("QABotToken: got %q, want test:TOKEN", f.QABotToken)
				}
				if f.ChatID != -100123456789 {
					t.Errorf("ChatID: got %d, want -100123456789", f.ChatID)
				}
				if f.PheraldBotUsername != "atmosphere_worker_bot" {
					t.Errorf("PheraldBotUsername: got %q, want atmosphere_worker_bot", f.PheraldBotUsername)
				}
			},
		},
		{
			name: "optional-non-op-token-from-env",
			env: map[string]string{
				"HERALD_QA_BOT_TOKEN":              "test:TOKEN",
				"HERALD_QA_BOT_TOKEN_NON_OPERATOR": "test:NONOPTOKEN",
				"HERALD_TGRAM_CHAT_ID":             "1",
				"HERALD_PHERALD_BOT_USERNAME":      "u",
			},
			assertFn: func(t *testing.T, f *lifecycleFlags) {
				if f.QABotTokenNonOp != "test:NONOPTOKEN" {
					t.Errorf("QABotTokenNonOp: got %q, want test:NONOPTOKEN", f.QABotTokenNonOp)
				}
			},
		},
		{
			name: "pherald-qa-out-dir-from-env",
			env: map[string]string{
				"HERALD_QA_BOT_TOKEN":         "test:TOKEN",
				"HERALD_TGRAM_CHAT_ID":        "1",
				"HERALD_PHERALD_BOT_USERNAME": "u",
				"HERALD_QA_OUT_DIR":           "/tmp/pherald-qa-out",
			},
			assertFn: func(t *testing.T, f *lifecycleFlags) {
				if f.PheraldQAOutDir != "/tmp/pherald-qa-out" {
					t.Errorf("PheraldQAOutDir: got %q, want /tmp/pherald-qa-out", f.PheraldQAOutDir)
				}
			},
		},
		{
			name: "flag-wins-over-env",
			env: map[string]string{
				"HERALD_QA_BOT_TOKEN":         "env:TOKEN",
				"HERALD_TGRAM_CHAT_ID":        "1",
				"HERALD_PHERALD_BOT_USERNAME": "envuser",
			},
			// Flag overrides env — assertFn confirms via the CLI args
			// path in the explicit Execute below.
			assertFn: func(t *testing.T, f *lifecycleFlags) {
				if f.QABotToken != "flag:TOKEN" {
					t.Errorf("QABotToken: got %q, want flag:TOKEN (flag must win)", f.QABotToken)
				}
				if f.PheraldBotUsername != "flaguser" {
					t.Errorf("PheraldBotUsername: got %q, want flaguser (flag must win)", f.PheraldBotUsername)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			chdirToTemp(t)
			resetLifecycleEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			cmd, f := newLifecycleCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			// The "flag-wins-over-env" case passes explicit flags; the
			// others rely on the env-only fallback path.
			if tc.name == "flag-wins-over-env" {
				cmd.SetArgs([]string{
					"--qa-bot-token", "flag:TOKEN",
					"--pherald-bot-username", "flaguser",
				})
			} else {
				cmd.SetArgs([]string{})
			}

			err := cmd.Execute()
			if tc.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.assertFn(t, f)
		})
	}
}

// TestLifecycleRequiredFieldsValidation drives the command with each
// required field missing in turn and asserts the error message names
// the missing flag + env-var. The error MUST NOT contain the literal
// token string (security mandate).
func TestLifecycleRequiredFieldsValidation(t *testing.T) {
	cases := []struct {
		name           string
		env            map[string]string
		args           []string
		wantErrSubstrs []string
	}{
		{
			name: "missing-qa-bot-token",
			env:  map[string]string{
				// no HERALD_QA_BOT_TOKEN
				"HERALD_TGRAM_CHAT_ID":        "1",
				"HERALD_PHERALD_BOT_USERNAME": "u",
			},
			wantErrSubstrs: []string{
				"HRD-101",
				"--qa-bot-token",
				"HERALD_QA_BOT_TOKEN",
			},
		},
		{
			name: "missing-chat-id",
			env: map[string]string{
				"HERALD_QA_BOT_TOKEN": "test:TOKEN",
				// no HERALD_TGRAM_CHAT_ID
				"HERALD_PHERALD_BOT_USERNAME": "u",
			},
			wantErrSubstrs: []string{
				"HRD-101",
				"--chat-id",
				"HERALD_TGRAM_CHAT_ID",
			},
		},
		{
			name: "missing-pherald-bot-username",
			env: map[string]string{
				"HERALD_QA_BOT_TOKEN":  "test:TOKEN",
				"HERALD_TGRAM_CHAT_ID": "1",
				// no HERALD_PHERALD_BOT_USERNAME
			},
			wantErrSubstrs: []string{
				"HRD-101",
				"--pherald-bot-username",
				"HERALD_PHERALD_BOT_USERNAME",
			},
		},
		{
			name:           "all-required-missing",
			env:            map[string]string{},
			wantErrSubstrs: []string{"HRD-101", "--qa-bot-token"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			chdirToTemp(t)
			resetLifecycleEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			cmd, _ := newLifecycleCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			msg := err.Error()
			for _, s := range tc.wantErrSubstrs {
				if !strings.Contains(msg, s) {
					t.Errorf("error message missing %q: got %q", s, msg)
				}
			}

			// Security check: the error MUST NOT contain "test:TOKEN"
			// (the synthetic token used in the env). This proves the
			// validator does not echo the value.
			if strings.Contains(msg, "test:TOKEN") {
				t.Errorf("error message LEAKED the QA bot token value: %q", msg)
			}
		})
	}
}

// TestLifecycleDefaultRunID drives a happy-path Execute and asserts the
// resolved RunID matches the shape "<ISO-ts>-<4 hex chars>".
//
// Shape: YYYY-MM-DDTHH-MM-SS-XXXX where XXXX is 4 lowercase hex chars.
// We tolerate any valid ISO-ish timestamp; the strict assertion is the
// regex below.
func TestLifecycleDefaultRunID(t *testing.T) {
	chdirToTemp(t)
	resetLifecycleEnv(t)
	t.Setenv("HERALD_QA_BOT_TOKEN", "test:TOKEN")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "1")
	t.Setenv("HERALD_PHERALD_BOT_USERNAME", "u")

	cmd, f := newLifecycleCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if f.RunID == "" {
		t.Fatalf("RunID empty — generator did not fire")
	}

	// Shape: 2026-05-23T14-22-00-9f2c — 19 chars timestamp + "-" + 4 hex.
	pattern := `^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-[0-9a-f]{4}$`
	matched, err := regexp.MatchString(pattern, f.RunID)
	if err != nil {
		t.Fatalf("regexp: %v", err)
	}
	if !matched {
		t.Errorf("RunID %q does not match pattern %q", f.RunID, pattern)
	}

	// Also: RunID should be parseable up to the ts portion.
	parts := strings.Split(f.RunID, "-")
	if len(parts) < 6 {
		t.Errorf("RunID %q has too few hyphen-separated parts", f.RunID)
	}

	// Generator monotonicity sanity — second call must differ (random
	// hex suffix). One in 65 536 chance of collision; we accept that
	// theoretical possibility but flag it for repro if it ever fires.
	second := generateLifecycleRunID()
	if second == f.RunID && !strings.HasSuffix(f.RunID, "-0000") {
		t.Errorf("RunID generator produced identical IDs back-to-back: %q", f.RunID)
	}
}

// TestLifecycleDefaultOutDir drives Execute and asserts the OutDir
// default lands at docs/qa/HRD-101-lifecycle-<runID> when --out is
// unset.
func TestLifecycleDefaultOutDir(t *testing.T) {
	chdirToTemp(t)
	resetLifecycleEnv(t)
	t.Setenv("HERALD_QA_BOT_TOKEN", "test:TOKEN")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "1")
	t.Setenv("HERALD_PHERALD_BOT_USERNAME", "u")

	cmd, f := newLifecycleCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	want := fmt.Sprintf("docs/qa/HRD-101-lifecycle-%s", f.RunID)
	if f.OutDir != want {
		t.Errorf("OutDir: got %q, want %q", f.OutDir, want)
	}

	// MkdirAll must have created the OutDir + attachments/ subdir.
	for _, p := range []string{f.OutDir, f.OutDir + "/attachments"} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("stat %q: %v", p, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%q exists but is not a directory", p)
		}
	}
}

// TestLifecycleExplicitOutDir confirms the operator-supplied --out
// flag wins over the default-derivation path.
func TestLifecycleExplicitOutDir(t *testing.T) {
	chdirToTemp(t)
	resetLifecycleEnv(t)
	t.Setenv("HERALD_QA_BOT_TOKEN", "test:TOKEN")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "1")
	t.Setenv("HERALD_PHERALD_BOT_USERNAME", "u")

	cmd, f := newLifecycleCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--out", "custom/qa/dir"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if f.OutDir != "custom/qa/dir" {
		t.Errorf("OutDir: got %q, want custom/qa/dir", f.OutDir)
	}
}

// TestLifecycleManualFlagShortCircuits asserts that --manual produces
// the documented short-circuit output and returns nil.
func TestLifecycleManualFlagShortCircuits(t *testing.T) {
	chdirToTemp(t)
	resetLifecycleEnv(t)
	t.Setenv("HERALD_QA_BOT_TOKEN", "test:TOKEN")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "1")
	t.Setenv("HERALD_PHERALD_BOT_USERNAME", "u")

	var out bytes.Buffer
	cmd, _ := newLifecycleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--manual"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "--manual") {
		t.Errorf("stdout missing --manual mention: %q", out.String())
	}
}

// TestLifecycleSkeletonMessage asserts the documented T1 short-circuit
// message ("skeleton — T2/T3 not wired yet") surfaces on stdout when
// the happy-path RunE fires without --manual.
func TestLifecycleSkeletonMessage(t *testing.T) {
	chdirToTemp(t)
	resetLifecycleEnv(t)
	t.Setenv("HERALD_QA_BOT_TOKEN", "test:TOKEN")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "1")
	t.Setenv("HERALD_PHERALD_BOT_USERNAME", "u")

	var out bytes.Buffer
	cmd, _ := newLifecycleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "skeleton") {
		t.Errorf("stdout missing skeleton sentinel: %q", out.String())
	}
}

// TestLifecycleFlagDefaults confirms the documented default values for
// every flag that has a non-zero default — these are the values
// callers (and T4 orchestrator) rely on when no override is supplied.
func TestLifecycleFlagDefaults(t *testing.T) {
	chdirToTemp(t)
	resetLifecycleEnv(t)
	t.Setenv("HERALD_QA_BOT_TOKEN", "test:TOKEN")
	t.Setenv("HERALD_TGRAM_CHAT_ID", "1")
	t.Setenv("HERALD_PHERALD_BOT_USERNAME", "u")

	cmd, f := newLifecycleCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if f.DocsDir != "docs" {
		t.Errorf("DocsDir default: got %q, want docs", f.DocsDir)
	}
	if f.PerScenarioTimeout != 60*time.Second {
		t.Errorf("PerScenarioTimeout default: got %s, want 60s", f.PerScenarioTimeout)
	}
	if f.OverallTimeout != 30*time.Minute {
		t.Errorf("OverallTimeout default: got %s, want 30m", f.OverallTimeout)
	}
	if f.SkipPreflight {
		t.Errorf("SkipPreflight default: got true, want false")
	}
	if f.Manual {
		t.Errorf("Manual default: got true, want false")
	}
}
