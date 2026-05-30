//go:build integration

package claude_code

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
)

// TestDispatch_LiveClaudeInvocation exercises Dispatcher.Dispatch against
// the actual `claude` CLI on PATH. Per §11.4.3 the test SKIPs cleanly
// (hardware_not_present) when the binary, project name, or session UUID
// is absent; otherwise it asserts the §107 bluff guard: a real PASS
// requires Outcome + Summary populated from the parsed
// <<<HERALD-REPLY>>> JSON, not a hand-rolled default.
//
// `claude --resume <UUID>` resolves the session against its on-disk
// project store keyed by the working directory of the invocation, so
// the workdir MUST be the directory the session was created in. The
// operator supplies workdir + UUID via env vars; the test cleans up
// the anchor file it writes there.
//
// §11.4.98 rule-2 (UUID-collision safety): the test MUST NOT `claude
// --resume` an arbitrary operator-supplied session UUID, because if it
// collides with the session the dev conductor is currently driving,
// claude silently exits -1 (Herald 2026-05-28 lesson). Therefore this
// test requires a DEDICATED, test-only session via
// HERALD_CLAUDE_TEST_SESSION_UUID (the legacy HERALD_CLAUDE_SESSION_UUID
// is no longer accepted) and HARD-FAILS if that UUID equals the
// conductor's live anchored session for this workdir/project.
func TestDispatch_LiveClaudeInvocation(t *testing.T) {
	binary := os.Getenv("HERALD_CLAUDE_BIN")
	if binary == "" {
		binary = "claude"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("skip: hardware_not_present — %s not on PATH per §11.4.3", binary)
	}
	projectName := os.Getenv("HERALD_CLAUDE_PROJECT_NAME")
	if projectName == "" {
		t.Skipf("skip: hardware_not_present — HERALD_CLAUDE_PROJECT_NAME absent per §11.4.3")
	}
	// §11.4.98 rule-2: a DEDICATED test-only session UUID is mandatory.
	// We deliberately do NOT accept HERALD_CLAUDE_SESSION_UUID anymore —
	// resuming an arbitrary operator-supplied UUID risks colliding with
	// the conductor's live session (silent claude exit -1).
	if os.Getenv("HERALD_CLAUDE_SESSION_UUID") != "" {
		t.Fatalf("HERALD_CLAUDE_SESSION_UUID is rejected per §11.4.98 rule-2 — it risks colliding with the dev conductor's live session. Use HERALD_CLAUDE_TEST_SESSION_UUID with a DEDICATED test-only session instead.")
	}
	testSessionStr := os.Getenv("HERALD_CLAUDE_TEST_SESSION_UUID")
	if testSessionStr == "" {
		t.Skipf("skip: hardware_not_present — HERALD_CLAUDE_TEST_SESSION_UUID absent per §11.4.3 (claude --resume needs a DEDICATED test-only session, distinct from any conductor session, per §11.4.98 rule-2)")
	}
	sessionUUID, err := uuid.Parse(testSessionStr)
	if err != nil {
		t.Fatalf("HERALD_CLAUDE_TEST_SESSION_UUID %q is not a valid UUID: %v", testSessionStr, err)
	}

	workdir := os.Getenv("HERALD_CLAUDE_WORKDIR")
	if workdir == "" {
		workdir, err = os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
	}

	d, err := New(binary, workdir, projectName)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// §11.4.98 rule-2 collision guard: if the conductor already anchored a
	// live session for this workdir/project, the test session UUID MUST
	// differ from it. Resuming the conductor's UUID would cause the silent
	// exit -1 collision this rule exists to prevent.
	if conductorUUID, _, rerr := d.ResolveSession(); rerr == nil && conductorUUID != uuid.Nil && conductorUUID == sessionUUID {
		t.Fatalf("HERALD_CLAUDE_TEST_SESSION_UUID %s collides with the conductor's live anchored session for workdir=%q project=%q — §11.4.98 rule-2 forbids resuming it (silent claude exit -1). Use a dedicated test-only session UUID.", sessionUUID, workdir, projectName)
	}
	_, anchor, _ := d.ResolveSession()
	if err := d.PersistSession(sessionUUID, anchor); err != nil {
		t.Fatalf("PersistSession: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort: remove the anchor file and any empty parents we
		// created. Stop walking up at the first non-empty / non-removable
		// directory so we never delete operator data.
		_ = os.Remove(anchor)
		for dir := filepath.Dir(anchor); strings.HasPrefix(dir, workdir) && dir != workdir; dir = filepath.Dir(dir) {
			if err := os.Remove(dir); err != nil {
				break
			}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	req := DispatchRequest{
		InboundID:    "INB-INTEG-1",
		Sender:       "tgram:integration-test",
		Channel:      commons.ChannelTelegram,
		Conversation: "(no prior thread — integration test invocation)",
		UserMessage:  "Integration test ping: please reply with the <<<HERALD-REPLY>>> JSON envelope per the dispatch contract. Outcome should be \"answered\" with a short summary acknowledging this is a Herald integration test.",
		Classification: Classification{
			Type:        "query",
			Criticality: "low",
			Confidence:  0.99,
		},
	}
	resp, err := d.Dispatch(ctx, req)
	if err != nil {
		// Honest FAIL — surface the diagnostic so it's visible whether
		// the failure is exec-level (exit code), parse-level (no marker),
		// or session-level (claude rejected the resume UUID).
		t.Fatalf("Dispatch: %v", err)
	}

	// §107 bluff guard: prove Claude actually emitted the structured
	// reply rather than parseReply having a "default" path.
	if strings.TrimSpace(resp.Outcome) == "" {
		t.Fatal("Dispatch returned empty Outcome — §107 bluff guard")
	}
	if strings.TrimSpace(resp.Summary) == "" {
		t.Fatal("Dispatch returned empty Summary — §107 bluff guard")
	}
	if resp.SessionUUID != sessionUUID {
		t.Errorf("response SessionUUID = %s, want %s", resp.SessionUUID, sessionUUID)
	}
	if resp.AnchorPath != anchor {
		t.Errorf("response AnchorPath = %q, want %q", resp.AnchorPath, anchor)
	}
}
