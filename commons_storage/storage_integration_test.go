//go:build integration

// HRD-010 Task 3 — §107 E14 evidence: RLS tenant-isolation live round-trip
// against real Postgres.
//
// Run with:
//
//	go test -tags=integration -timeout 5m -count=1 -run TestRLS_TenantIsolation_RoundTrip ./commons_storage/...
//
// Requires a running Podman or Docker runtime on the host. Without one the
// default `go test ./commons_storage/...` (no -tags) cleanly skips this file.
//
// Anti-bluff per §107 + §11.4.5 + §11.4.68:
//
//   - The test asserts EXACT-1 row count for each tenant (not >=1). A test
//     that asserted ">=1" would PASS even if RLS leaked tenant B's row into
//     tenant A's session.
//   - The test asserts EXACT row-ID match (the returned id is exactly the
//     UUID we inserted for that tenant). A test that only checked
//     non-empty would PASS even if RLS returned the wrong row.
//   - The test inserts ONE row per tenant under that tenant's own
//     WithTenantContext so RLS's WITH CHECK clause is exercised on insert
//     AND so the SELECT's RLS USING clause is exercised on read.
//   - Both tenants are checked SYMMETRICALLY — tenant A AND tenant B —
//     so a one-way leak (B sees A but A doesn't see B) would still fail.
//
// This is the load-bearing RLS proof: it proves WithTenantContext actually
// isolates rows, not just that the SQL compiles.

package storage_test

import (
	"context"
	"os"
	"testing"
	"time"

	db "digital.vasic.database/pkg/database"
	"github.com/google/uuid"

	infra "github.com/vasic-digital/herald/commons_infra"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// TestRLS_TenantIsolation_RoundTrip is the §107 E14 evidence: two tenants
// each insert a subscribers row under their own RLS context; each tenant's
// read MUST see ONLY its own row.
func TestRLS_TenantIsolation_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Test-scope env: the quickstart compose declares ${HERALD_DB_PASSWORD}
	// (and friends) as required. Use throwaway values; t.Setenv auto-restores
	// on test end so nothing leaks beyond this test. Same pattern as Task 2's
	// TestUp_PopulatesPool in commons_infra/clients_integration_test.go.
	t.Setenv("HERALD_DB_PASSWORD", "test-postgres-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_REDIS_PASSWORD", "test-redis-password-DO-NOT-USE-IN-PROD")
	t.Setenv("HERALD_PROJECT_NAME", "Herald-Integration-Test")
	t.Setenv("HERALD_TENANT_ID", "00000000-0000-0000-0000-000000000099")

	if os.Getenv("DOCKER_HOST") == "" {
		if sock := os.Getenv("PODMAN_MAC_SOCK"); sock != "" {
			t.Setenv("DOCKER_HOST", "unix://"+sock)
		}
	}

	boot, err := infra.NewQuickstartBoot(infra.Config{
		Services: []string{"postgres"}, // limit blast radius: only postgres
	})
	if err != nil {
		t.Skipf("compose runtime not available; skipping (closed-set reason: hardware_not_present): %v", err)
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

	tenantA := uuid.New()
	tenantB := uuid.New()
	subA := uuid.New()
	subB := uuid.New()

	// Seed both tenants in the `tenants` table so the FK in subscribers
	// (if any) and audit trails resolve. The constitution_state test
	// in commons_constitution does the same — pattern lifted.
	for _, id := range []uuid.UUID{tenantA, tenantB} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO tenants (id, name, environment) VALUES ($1, $2, $3)
			 ON CONFLICT (id) DO NOTHING`,
			id, "rls-e14-"+id.String()[:8], "quickstart",
		); err != nil {
			t.Fatalf("seed tenant %s: %v", id, err)
		}
	}

	// insert places one row in `subscribers` for the given tenant, under
	// that tenant's RLS context. Per migration 000003_subscribers.up.sql:
	// columns are (id UUID PK, tenant_id UUID NOT NULL, handle TEXT,
	// display_name TEXT, locale TEXT, timezone TEXT, kind TEXT,
	// roles TEXT[], metadata JSONB, created_at TIMESTAMPTZ).
	// Only id + tenant_id are required; display_name is set so the row
	// is human-identifiable in any debug dump.
	insert := func(tenant, sub uuid.UUID, name string) {
		err := storage.WithTenantContext(ctx, pool, tenant, func(tx db.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO subscribers (id, tenant_id, display_name)
				 VALUES ($1, $2, $3)`,
				sub, tenant, name,
			)
			return err
		})
		if err != nil {
			t.Fatalf("insert tenant=%s sub=%s: %v", tenant, sub, err)
		}
	}
	insert(tenantA, subA, "rls-e14-sub-A")
	insert(tenantB, subB, "rls-e14-sub-B")

	// read returns every visible subscribers.id under the given tenant's
	// RLS context. With RLS working, this MUST equal exactly [subX] for
	// tenant X. Note: the SELECT has NO WHERE tenant_id clause — the
	// load-bearing point is that RLS adds it implicitly. A bluff
	// implementation that re-applied tenant filtering in Go would pass
	// this test even with RLS disabled; we control for that by inserting
	// under each tenant's own context (which exercises WITH CHECK).
	read := func(tenant uuid.UUID) []uuid.UUID {
		var ids []uuid.UUID
		err := storage.WithTenantContext(ctx, pool, tenant, func(tx db.Tx) error {
			// Filter to ONLY the two e14 rows so other integration tests
			// that may have left subscribers rows in the same compose
			// stack don't pollute the assertions. We still rely on RLS
			// to hide the OTHER tenant's e14 row; this IN-clause only
			// excludes pre-existing rows from prior unrelated runs.
			//
			// Note: explicit IN ($1, $2) instead of `ANY($1)` because
			// pgx-v5's text-format encoder rejects []uuid.UUID without
			// a pgtype.Array wrapper; two scalar params are simpler and
			// avoid the dependency on pgtype.
			rows, err := tx.Query(ctx,
				`SELECT id FROM subscribers WHERE id IN ($1, $2) ORDER BY id`,
				subA, subB,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if err != nil {
			t.Fatalf("read tenant=%s: %v", tenant, err)
		}
		return ids
	}

	// Tenant A: must see exactly [subA].
	readA := read(tenantA)
	if len(readA) != 1 {
		t.Fatalf("tenant A: expected EXACTLY 1 row, got %d (RLS LEAK if 2 — §107 E14 bluff guard)", len(readA))
	}
	if readA[0] != subA {
		t.Fatalf("tenant A: expected subA=%s, got %s (RLS ROW-MIX bluff)", subA, readA[0])
	}

	// Tenant B: must see exactly [subB] — checked SYMMETRICALLY so a
	// one-way leak (B sees A but A doesn't see B, or vice versa) fails.
	readB := read(tenantB)
	if len(readB) != 1 {
		t.Fatalf("tenant B: expected EXACTLY 1 row, got %d (RLS LEAK if 2 — §107 E14 bluff guard)", len(readB))
	}
	if readB[0] != subB {
		t.Fatalf("tenant B: expected subB=%s, got %s (RLS ROW-MIX bluff)", subB, readB[0])
	}

	// Captured-evidence log (§11.4.5): print the asserted invariants so
	// the test output contains positive proof, not just absence-of-fail.
	t.Logf("E14 PASS: tenantA=%s saw [%s] (1 row); tenantB=%s saw [%s] (1 row); RLS isolation proven",
		tenantA, subA, tenantB, subB)
}
