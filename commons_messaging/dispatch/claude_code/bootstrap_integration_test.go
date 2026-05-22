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
)

// TestBootstrapSession_LiveClaudeInvocation exercises bootstrapSession
// against the actual `claude` CLI on PATH (HRD-012 step 7 root-cause
// fix, 2026-05-22). Per §11.4.3 the test SKIPs cleanly when the binary
// is absent.
//
// §107 evidence: a real PASS requires (a) bootstrapSession returns a
// non-Nil UUID and nil error, (b) the anchor file is persisted under
// the dispatcher's working dir with the returned UUID as content,
// (c) the session transcript file appears under
// ~/.claude/projects/<encoded-cwd>/<uuid>.jsonl (proves the claude
// subprocess actually created a session, not just exited 0).
//
// Concretely: this is the test that would have caught the original
// PASS-bluff — before the bootstrap landed, no test exercised the
// uuid.Nil path against real claude, so docs/Fixed.md was free to
// claim HRD-012 step 7 closed while the runtime errored.
func TestBootstrapSession_LiveClaudeInvocation(t *testing.T) {
	binary := os.Getenv("HERALD_CLAUDE_BIN")
	if binary == "" {
		binary = "claude"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("skip: hardware_not_present — %s not on PATH per §11.4.3", binary)
	}

	workdir := t.TempDir()
	projectName := "BootstrapIntegrationProj-" + uuid.New().String()[:8]

	d, err := New(binary, workdir, projectName)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Stretch budget a bit — first-time claude invocations on a cold
	// project directory can do auth + tooling discovery.
	d.SetBootstrapTimeout(180 * time.Second)

	_, anchor, err := d.ResolveSession()
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	// (a) anchor must NOT exist pre-test.
	if _, statErr := os.Stat(anchor); !os.IsNotExist(statErr) {
		t.Fatalf("anchor must not exist pre-test; stat err=%v", statErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	gotUUID, err := d.bootstrapSession(ctx, anchor)
	if err != nil {
		t.Fatalf("bootstrapSession: %v", err)
	}
	if gotUUID == uuid.Nil {
		t.Fatal("bootstrapSession returned uuid.Nil with err=nil — invariant violation")
	}

	// (b) anchor file MUST exist and contain the returned UUID.
	raw, err := os.ReadFile(anchor)
	if err != nil {
		t.Fatalf("read anchor: %v", err)
	}
	parsed, err := uuid.Parse(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("anchor body not a UUID: %v (raw=%q)", err, string(raw))
	}
	if parsed != gotUUID {
		t.Fatalf("anchor UUID %s != returned UUID %s", parsed, gotUUID)
	}

	// (c) claude session transcript MUST exist under
	//     ~/.claude/projects/<encoded-cwd>/<uuid>.jsonl. Best-effort:
	//     macOS resolves /tmp -> /private/tmp so we check both forms.
	//     If neither path resolves, fall back to a recursive scan of
	//     ~/.claude/projects/ for the UUID-named jsonl — the encoding
	//     scheme is claude-internal and may change.
	home, _ := os.UserHomeDir()
	transcriptName := gotUUID.String() + ".jsonl"
	candidates := []string{
		filepath.Join(home, ".claude", "projects", encodePath(workdir), transcriptName),
		filepath.Join(home, ".claude", "projects", encodePath("/private"+workdir), transcriptName),
	}
	found := false
	for _, c := range candidates {
		if _, statErr := os.Stat(c); statErr == nil {
			found = true
			t.Logf("transcript found at %s", c)
			break
		}
	}
	if !found {
		// Recursive fallback — search any sub-dir of ~/.claude/projects/
		// for the transcript file. Bounded by walking only one level
		// deep so we don't traverse the operator's whole .claude tree.
		projectsRoot := filepath.Join(home, ".claude", "projects")
		entries, _ := os.ReadDir(projectsRoot)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(projectsRoot, e.Name(), transcriptName)
			if _, statErr := os.Stat(candidate); statErr == nil {
				found = true
				t.Logf("transcript found at %s (via recursive fallback)", candidate)
				break
			}
		}
	}
	if !found {
		t.Errorf("transcript %s NOT found under %s/.claude/projects/ — claude may not have actually created a session (§107 bluff guard)",
			transcriptName, home)
	}

	// Cleanup: remove the anchor file. Leaving the transcript in place
	// is harmless — claude rotates project dirs on its own schedule.
	t.Cleanup(func() {
		_ = os.Remove(anchor)
	})
}

// encodePath mirrors claude's project-dir encoding: replace os.PathSeparator
// with "-" and prefix with "-". This may drift if claude changes its
// scheme — the recursive fallback above protects against that.
func encodePath(p string) string {
	out := strings.ReplaceAll(p, string(os.PathSeparator), "-")
	if !strings.HasPrefix(out, "-") {
		out = "-" + out
	}
	return out
}
