//go:build integration

// HRD-012 Task 7 — §107 E18 evidence: Claude Code session-state persistence
// live round-trip against real Postgres + real `claude` CLI.
//
// Run with:
//
//	HERALD_CLAUDE_PROJECT_NAME=Herald \
//	HERALD_CLAUDE_SESSION_UUID=<uuid-from-T6> \
//	HERALD_CLAUDE_BIN=claude \
//	HERALD_CLAUDE_WORKDIR=/Users/milosvasic/Projects/Herald \
//	  go test -tags=integration -timeout 10m -count=1 \
//	    -run TestDispatch_PersistsSessionState \
//	    ./commons_messaging/dispatch/claude_code/...
//
// Requires:
//   - A running Podman or Docker runtime on the host.
//   - HERALD_CLAUDE_PROJECT_NAME + HERALD_CLAUDE_SESSION_UUID + `claude` on
//     PATH (skipped per §11.4.3 hardware_not_present otherwise — no fakes).
//
// Anti-bluff per §107 + §11.4.5:
//
//   - Asserts the persisted session_uuid EXACTLY equals the dispatch
//     response's SessionUUID. A Herald-synthetic UUID would silently bind a
//     wrong/dead session on restart — that's the load-bearing §107 invariant.
//   - Asserts the persisted anchor_path EXACTLY equals the dispatch
//     response's AnchorPath.
//   - Round-trips the last_response JSONB through DispatchResponse decode
//     and asserts Outcome equality + non-empty Summary.
//   - Reads back under HeraldSystemTenant's RLS context, exercising the
//     claude_code_sessions_tenant_isolation policy.

package claude_code

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"

	infra "github.com/vasic-digital/herald/commons_infra"
	storage "github.com/vasic-digital/herald/commons_storage"
)

func TestDispatch_PersistsSessionState(t *testing.T) {
	binary := os.Getenv("HERALD_CLAUDE_BIN")
	if binary == "" {
		binary = "claude"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("skip: hardware_not_present — %s not on PATH per §11.4.3", binary)
	}
	projectName := os.Getenv("HERALD_CLAUDE_PROJECT_NAME")
	sessionUUIDStr := os.Getenv("HERALD_CLAUDE_SESSION_UUID")
	if projectName == "" || sessionUUIDStr == "" {
		t.Skipf("skip: hardware_not_present — HERALD_CLAUDE_PROJECT_NAME or HERALD_CLAUDE_SESSION_UUID absent per §11.4.3")
	}
	sessionUUID, err := uuid.Parse(sessionUUIDStr)
	if err != nil {
		t.Fatalf("HERALD_CLAUDE_SESSION_UUID %q is not a valid UUID: %v", sessionUUIDStr, err)
	}
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skipf("skip: hardware_not_present — no container runtime (podman/docker) on PATH per §11.4.3")
		}
	}

	// Test-scope env: the quickstart compose declares ${HERALD_DB_PASSWORD}
	// (and friends) as required. Pattern lifted from
	// commons_storage/storage_integration_test.go::TestRLS_TenantIsolation_RoundTrip.
	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	boot, err := infra.NewQuickstartBoot(infra.Config{
		Services: []string{"postgres"}, // limit blast radius: only postgres
	})
	if err != nil {
		t.Skipf("skip: compose runtime not available (hardware_not_present): %v", err)
	}

	if err := boot.Up(ctx); err != nil {
		t.Fatalf("boot.Up: %v", err)
	}
	t.Cleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer downCancel()
		if err := boot.Down(downCtx); err != nil {
			t.Logf("boot.Down (cleanup): %v", err)
		}
	})

	pool, err := boot.Pool()
	if err != nil {
		t.Fatalf("Pool() after Up(): %v", err)
	}
	if pool == nil {
		t.Fatal("Pool() returned nil without error — §107 PASS-bluff guard")
	}

	// Workdir must match the operator's pre-resolved session.
	workdir := os.Getenv("HERALD_CLAUDE_WORKDIR")
	if workdir == "" {
		workdir = "."
	}

	d, err := NewWithStorage(binary, workdir, projectName, pool)
	if err != nil {
		t.Fatalf("NewWithStorage: %v", err)
	}

	// Bootstrap the anchor file from the operator-supplied UUID — mirrors
	// the T6 dispatch_integration_test pattern. Without this, ResolveSession
	// returns uuid.Nil and Dispatch fails with "no anchored session".
	_, anchor, _ := d.ResolveSession()
	if err := d.PersistSession(sessionUUID, anchor); err != nil {
		t.Fatalf("PersistSession (anchor bootstrap): %v", err)
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

	req := DispatchRequest{
		UserMessage: "Herald E18 persist test " + time.Now().Format(time.RFC3339Nano) +
			" — please reply with outcome=answered, summary set to a short ack",
	}
	resp, err := d.Dispatch(ctx, req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Read back claude_code_sessions row under HeraldSystemTenant.
	var persistedSessionUUID, persistedAnchorPath, persistedResponseJSON string
	err = storage.WithTenantContext(ctx, pool, HeraldSystemTenant, func(tx db.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT session_uuid::text, anchor_path, last_response::text
			 FROM claude_code_sessions
			 WHERE tenant_id = $1 AND project_name = $2`,
			HeraldSystemTenant, projectName,
		).Scan(&persistedSessionUUID, &persistedAnchorPath, &persistedResponseJSON)
	})
	if err != nil {
		t.Fatalf("read-back: %v", err)
	}

	// §107 load-bearing invariant: persisted session_uuid MUST equal the
	// uuid the dispatcher resolved (via the anchor file + live claude
	// --resume). A bluff implementation could insert uuid.New() here —
	// that would FAIL this check.
	if persistedSessionUUID != resp.SessionUUID.String() {
		t.Fatalf("persisted session_uuid mismatch: got %q want %q (§107 bluff guard)",
			persistedSessionUUID, resp.SessionUUID)
	}
	if persistedAnchorPath != resp.AnchorPath {
		t.Fatalf("persisted anchor_path mismatch: got %q want %q",
			persistedAnchorPath, resp.AnchorPath)
	}

	// Round-trip last_response JSONB and assert Outcome + Summary survive.
	var roundTripped DispatchResponse
	if err := json.Unmarshal([]byte(persistedResponseJSON), &roundTripped); err != nil {
		t.Fatalf("unmarshal persisted response: %v (raw=%q)", err, persistedResponseJSON)
	}
	if roundTripped.Outcome != resp.Outcome {
		t.Fatalf("persisted Outcome mismatch: got %q want %q",
			roundTripped.Outcome, resp.Outcome)
	}
	if roundTripped.Summary == "" {
		t.Fatal("persisted Summary empty — §107 bluff guard: a meaningful Claude reply must include a summary")
	}
}
