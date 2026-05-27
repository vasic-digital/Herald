//go:build integration

// Live Postgres integration tests for the M2 Postgres backends.
//
// Boot procedure: this test relies on commons_infra/QuickstartBoot having
// brought Postgres up at 127.0.0.1:24100 BEFORE this test starts. Use
// TestMain (in postgres_testmain_test.go, same build tag) to boot/down.
//
// Anti-bluff per §11.4.76 + §44.8: every assertion verifies a real DB
// effect (SELECT-after-mutate). No mocks. No fakes. The test fails if the
// schema is broken, the SQL is wrong, or RLS isolation leaks.

package constitution_test

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"
	"time"

	"digital.vasic.database/pkg/postgres"
	"github.com/google/uuid"

	constitution "github.com/vasic-digital/herald/commons_constitution"
	cladder "github.com/vasic-digital/herald/commons_constitution/ladder"
	cstate "github.com/vasic-digital/herald/commons_constitution/state"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// pgConfig returns the postgres.Config for the booted quickstart Postgres.
// Mirrors the docker-compose.quickstart.yml mapping (host:24100 → 5432).
func pgConfig() *postgres.Config {
	password := os.Getenv("HERALD_DB_PASSWORD")
	if password == "" {
		password = "test-postgres-password-DO-NOT-USE-IN-PROD"
	}
	return storage.ConfigForHerald(
		"127.0.0.1", 24100, "herald", password, "herald",
	)
}

func TestPostgresStore_RecordAndGet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pgDB, err := storage.Open(ctx, pgConfig())
	if err != nil {
		t.Skipf("Postgres not reachable on 127.0.0.1:24100 — boot quickstart compose first: %v", err)
	}
	defer pgDB.Close()

	// Apply migrations (idempotent).
	if _, err := storage.RunMigrations(ctx, pgDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Seed a fresh tenant for this test (avoids cross-test contamination).
	tenantID := uuid.New()
	if _, err := pgDB.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		tenantID, "rec-"+tenantID.String()[:8], "quickstart",
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	store := cstate.NewPostgres(pgDB)
	bundle := constitution.CaptureBytes([]byte("rev-1"))
	mkResult := func(d constitution.Decision, ev string) constitution.Result {
		return constitution.Result{
			Decision:  d,
			Evidence:  ev,
			DigestSHA: sha256.Sum256([]byte(d.String() + ":" + ev)),
		}
	}

	// First Record → FirstSeen + Changed.
	trans, err := store.Record(ctx, tenantID, "§11.4.10", "subj-A",
		mkResult(constitution.DecisionFail, "missing"), bundle, "evidence://x")
	if err != nil {
		t.Fatalf("Record first: %v", err)
	}
	if !trans.FirstSeen || !trans.Changed {
		t.Errorf("first Record: FirstSeen=%v Changed=%v; want both true", trans.FirstSeen, trans.Changed)
	}

	// Second Record same values → no transition.
	trans, err = store.Record(ctx, tenantID, "§11.4.10", "subj-A",
		mkResult(constitution.DecisionFail, "missing"), bundle, "evidence://x")
	if err != nil {
		t.Fatalf("Record second: %v", err)
	}
	if trans.Changed {
		t.Errorf("identical Record reported Changed=true (transitions-only discipline violated)")
	}
	if trans.FirstSeen {
		t.Errorf("second Record reported FirstSeen=true")
	}

	// Get returns the row.
	row, ok, err := store.Get(ctx, tenantID, "§11.4.10", "subj-A")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if row.Decision != constitution.DecisionFail {
		t.Errorf("Get decision = %v; want fail", row.Decision)
	}
	if row.EvidenceURI != "evidence://x" {
		t.Errorf("Get evidence = %q; want evidence://x", row.EvidenceURI)
	}

	// Decision change → Changed=true.
	trans, err = store.Record(ctx, tenantID, "§11.4.10", "subj-A",
		mkResult(constitution.DecisionPass, "recovered"), bundle, "evidence://y")
	if err != nil {
		t.Fatalf("Record decision-change: %v", err)
	}
	if !trans.Changed {
		t.Errorf("decision change should report Changed=true")
	}
	if trans.OldDecision != constitution.DecisionFail || trans.NewDecision != constitution.DecisionPass {
		t.Errorf("transition direction = %v→%v; want fail→pass", trans.OldDecision, trans.NewDecision)
	}
}

func TestPostgresStore_RLSTenantIsolation(t *testing.T) {
	// THE LOAD-BEARING RLS PROOF: tenant B MUST NOT see tenant A's row,
	// even though the SELECT has no WHERE tenant_id clause. RLS policy
	// uses current_setting('app.tenant_id')::uuid for tenant isolation.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pgDB, err := storage.Open(ctx, pgConfig())
	if err != nil {
		t.Skipf("Postgres unreachable: %v", err)
	}
	defer pgDB.Close()
	if _, err := storage.RunMigrations(ctx, pgDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	tenantA, tenantB := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenantA, tenantB} {
		if _, err := pgDB.Exec(ctx,
			`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
			 ON CONFLICT (id) DO NOTHING`,
			id, "rls-"+id.String()[:8], "quickstart",
		); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
	}

	store := cstate.NewPostgres(pgDB)
	bundle := constitution.CaptureBytes([]byte("rls-test"))
	res := constitution.Result{
		Decision:  constitution.DecisionFail,
		Evidence:  "rls-test-evidence",
		DigestSHA: sha256.Sum256([]byte("rls")),
	}

	// Tenant A inserts.
	if _, err := store.Record(ctx, tenantA, "§rls", "subj-rls", res, bundle, ""); err != nil {
		t.Fatalf("Record tenant A: %v", err)
	}

	// Tenant B Get for the SAME (rule, subject) must return ok=false.
	_, ok, err := store.Get(ctx, tenantB, "§rls", "subj-rls")
	if err != nil {
		t.Fatalf("Get tenant B: %v", err)
	}
	if ok {
		t.Errorf("RLS LEAK: tenant B saw tenant A's row (would be §16 + §44.6 violation)")
	}

	// Tenant B List with no filter MUST return zero rows (RLS hides A's row).
	rows, err := store.List(ctx, tenantB, constitution.ListQuery{})
	if err != nil {
		t.Fatalf("List tenant B: %v", err)
	}
	if len(rows) > 0 {
		t.Errorf("RLS LEAK: List for tenant B returned %d rows; want 0 (would be §16 + §44.6 violation)", len(rows))
	}
}

func TestPostgresAudit_RecordAndList(t *testing.T) {
	// Load-bearing HRD-018 proof against REAL Postgres: the constitution_audit
	// row written by the audit write-through must be durable + RLS-scoped, and
	// emitted_event_id must round-trip for ModeEnforce + be NULL for ModeWarn.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pgDB, err := storage.Open(ctx, pgConfig())
	if err != nil {
		t.Skipf("Postgres unreachable: %v", err)
	}
	defer pgDB.Close()
	if _, err := storage.RunMigrations(ctx, pgDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	tenantID := uuid.New()
	if _, err := pgDB.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		tenantID, "aud-"+tenantID.String()[:8], "quickstart",
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	au := cstate.NewPostgresAudit(pgDB)
	bundle := constitution.CaptureBytes([]byte("aud-rev"))
	emittedID := uuid.New()

	// Enforce row carrying an emitted event ID + a prior decision.
	old := constitution.DecisionPass
	var oldDigest [32]byte = sha256.Sum256([]byte("prev"))
	enforceID, err := au.RecordAudit(ctx, constitution.AuditRow{
		TenantID:       tenantID,
		RuleID:         "§11.4.10",
		Subject:        "subj-aud",
		OldDecision:    &old,
		NewDecision:    constitution.DecisionFail,
		OldDigest:      &oldDigest,
		NewDigest:      sha256.Sum256([]byte("now")),
		BundleHash:     bundle,
		EvidenceURI:    "evidence://aud",
		EmittedEventID: emittedID,
		ModeAtEmission: constitution.ModeEnforce,
	})
	if err != nil {
		t.Fatalf("RecordAudit enforce: %v", err)
	}
	if enforceID == uuid.Nil {
		t.Fatalf("RecordAudit returned Nil id (uuidv7 default did not fire)")
	}

	// Warn row: audit-only, NULL emitted_event_id.
	if _, err := au.RecordAudit(ctx, constitution.AuditRow{
		TenantID:       tenantID,
		RuleID:         "§warn",
		Subject:        "subj-warn",
		NewDecision:    constitution.DecisionFail,
		NewDigest:      sha256.Sum256([]byte("w")),
		BundleHash:     bundle,
		ModeAtEmission: constitution.ModeWarn,
	}); err != nil {
		t.Fatalf("RecordAudit warn: %v", err)
	}

	rows, err := au.ListAudit(ctx, tenantID, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 audit rows, got %d", len(rows))
	}

	// Verify the enforce row round-tripped with its emitted ID + old decision.
	var enforceRow *constitution.AuditRow
	for i := range rows {
		if rows[i].RuleID == "§11.4.10" {
			enforceRow = &rows[i]
		}
	}
	if enforceRow == nil {
		t.Fatalf("enforce row not found in ListAudit result")
	}
	if enforceRow.EmittedEventID != emittedID {
		t.Errorf("enforce EmittedEventID = %v; want %v", enforceRow.EmittedEventID, emittedID)
	}
	if enforceRow.OldDecision == nil || *enforceRow.OldDecision != constitution.DecisionPass {
		t.Errorf("enforce OldDecision round-trip mismatch: %v", enforceRow.OldDecision)
	}
	if enforceRow.BundleHash != bundle {
		t.Errorf("enforce BundleHash round-trip mismatch")
	}

	// Verify the warn row has NULL emitted_event_id (uuid.Nil after scan).
	var warnRow *constitution.AuditRow
	for i := range rows {
		if rows[i].RuleID == "§warn" {
			warnRow = &rows[i]
		}
	}
	if warnRow == nil {
		t.Fatalf("warn row not found")
	}
	if warnRow.EmittedEventID != uuid.Nil {
		t.Errorf("warn EmittedEventID = %v; want Nil (NULL in DB)", warnRow.EmittedEventID)
	}
	if warnRow.OldDecision != nil {
		t.Errorf("warn row OldDecision should be nil (FirstSeen); got %v", *warnRow.OldDecision)
	}

	// RLS: a different tenant must see zero rows.
	other := uuid.New()
	if _, err := pgDB.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		other, "oth-"+other.String()[:8], "quickstart"); err != nil {
		t.Fatalf("seed other tenant: %v", err)
	}
	otherRows, err := au.ListAudit(ctx, other, constitution.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit other: %v", err)
	}
	if len(otherRows) != 0 {
		t.Errorf("RLS LEAK: other tenant saw %d audit rows; want 0", len(otherRows))
	}
}

// TestPostgresRunner_EndToEndAuditPersist is the full emit→persist round-trip
// against real Postgres: Runner.Run with Postgres-backed state + ladder +
// audit must (a) UPSERT constitution_state, (b) INSERT constitution_audit,
// (c) the audit row must carry the bundle hash for replay (§42.1.3).
func TestPostgresRunner_EndToEndAuditPersist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pgDB, err := storage.Open(ctx, pgConfig())
	if err != nil {
		t.Skipf("Postgres unreachable: %v", err)
	}
	defer pgDB.Close()
	if _, err := storage.RunMigrations(ctx, pgDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	tenantID := uuid.New()
	if _, err := pgDB.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		tenantID, "e2e-"+tenantID.String()[:8], "quickstart"); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	bus := constitution.NewMemoryBus(constitution.MemoryBusConfig{BufferSize: 16})
	defer bus.Close()
	em, err := constitution.NewEmitter(bus, constitution.EmitterConfig{Source: "e2e"})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	reg := constitution.NewRegistry()
	st := cstate.NewPostgres(pgDB)
	la := cladder.NewPostgres(pgDB)
	au := cstate.NewPostgresAudit(pgDB)
	runner, err := constitution.NewRunner(reg, la, st, em, au)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	bundle := constitution.CaptureBytes([]byte("e2e-bundle"))
	subject := constitution.Subject{Kind: "file", ID: "/e2e/secret"}
	ev := &evalForTest{id: "§e2e", sev: constitution.SeverityHigh,
		result: makeResult(constitution.DecisionFail, "e2e-violation")}
	reg.Register(ev)

	out, err := runner.Run(ctx, ev, tenantID, subject, bundle)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Audited || !out.Emitted {
		t.Fatalf("default-enforce + transition must audit+emit; out=%+v", out)
	}

	// (a) state row persisted.
	row, ok, err := st.Get(ctx, tenantID, "§e2e", subject.ID)
	if err != nil || !ok {
		t.Fatalf("state.Get: ok=%v err=%v", ok, err)
	}
	if row.Decision != constitution.DecisionFail {
		t.Errorf("persisted state decision = %v; want fail", row.Decision)
	}

	// (b)+(c) audit row persisted with bundle hash for replay.
	auditRows, err := au.ListAudit(ctx, tenantID, constitution.AuditQuery{RuleID: "§e2e"})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(auditRows) != 1 {
		t.Fatalf("expected 1 e2e audit row, got %d", len(auditRows))
	}
	if auditRows[0].BundleHash != bundle {
		t.Errorf("audit bundle hash != evaluated bundle (replay correlation broken)")
	}
	if auditRows[0].EmittedEventID == uuid.Nil {
		t.Errorf("enforce audit row must carry the emitted event ID; got Nil")
	}
}

func TestPostgresLadder_GetDefaultsToEnforce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pgDB, err := storage.Open(ctx, pgConfig())
	if err != nil {
		t.Skipf("Postgres unreachable: %v", err)
	}
	defer pgDB.Close()
	if _, err := storage.RunMigrations(ctx, pgDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	tenantID := uuid.New()
	if _, err := pgDB.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		tenantID, "lad-"+tenantID.String()[:8], "quickstart",
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	l := cladder.NewPostgres(pgDB)
	got, err := l.Get(ctx, tenantID, "§unbound")
	if err != nil {
		t.Fatalf("Get unbound: %v", err)
	}
	if got != constitution.ModeEnforce {
		t.Errorf("unbound rule default = %v; want ModeEnforce", got)
	}
}

func TestPostgresLadder_SetGetRoundtrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pgDB, err := storage.Open(ctx, pgConfig())
	if err != nil {
		t.Skipf("Postgres unreachable: %v", err)
	}
	defer pgDB.Close()
	if _, err := storage.RunMigrations(ctx, pgDB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	tenantID := uuid.New()
	if _, err := pgDB.Exec(ctx,
		`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		tenantID, "ld2-"+tenantID.String()[:8], "quickstart",
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	l := cladder.NewPostgres(pgDB)
	if err := l.Set(ctx, tenantID, "§A", constitution.ModeWarn, "ops@test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := l.Get(ctx, tenantID, "§A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != constitution.ModeWarn {
		t.Errorf("after Set(warn), Get = %v; want warn", got)
	}

	// Overwrite + verify.
	if err := l.Set(ctx, tenantID, "§A", constitution.ModeAllow, "sre@test"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	got, _ = l.Get(ctx, tenantID, "§A")
	if got != constitution.ModeAllow {
		t.Errorf("after overwrite, Get = %v; want allow", got)
	}

	// List returns the binding.
	all, err := l.List(ctx, tenantID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if all["§A"] != constitution.ModeAllow {
		t.Errorf("List[§A] = %v; want allow", all["§A"])
	}
}
