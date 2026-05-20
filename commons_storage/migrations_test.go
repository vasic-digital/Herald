package storage

import (
	"io/fs"
	"strings"
	"testing"
)

// TestMigrationsBundle verifies the embed.FS contains exactly the
// migrations we expect, each with matching up + down. Anti-bluff per
// operator mandate 2026-05-20: pass-without-execution is forbidden.
// This test fails immediately if a migration file is added but its
// .down sibling is missing, or vice-versa.
func TestMigrationsBundle(t *testing.T) {
	files := make(map[string]bool)
	if err := fs.WalkDir(migrationsFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".sql") {
			files[p] = true
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}

	// Foundation expects exactly these 8 migrations:
	//   5 original + 2 for §44 + 000008 for §44.6 RLS-FORCE+app-grant.
	expectedNames := []string{
		"000001_init_core",
		"000002_idempotency_keys",
		"000003_subscribers",
		"000004_channel_addresses",
		"000005_inbound_pipeline",
		"000006_constitution_state",
		"000007_constitution_bindings",
		"000008_force_rls",
	}

	for _, name := range expectedNames {
		upPath := "migrations/" + name + ".up.sql"
		downPath := "migrations/" + name + ".down.sql"
		if !files[upPath] {
			t.Errorf("missing UP migration: %s", upPath)
		}
		if !files[downPath] {
			t.Errorf("missing DOWN migration: %s", downPath)
		}

		// Anti-bluff: empty files would silently no-op. Reject them.
		upBody, err := migrationsFS.ReadFile(upPath)
		if err != nil {
			t.Errorf("read %s: %v", upPath, err)
			continue
		}
		if len(strings.TrimSpace(string(upBody))) < 10 {
			t.Errorf("%s is suspiciously short (%d bytes)", upPath, len(upBody))
		}
		downBody, err := migrationsFS.ReadFile(downPath)
		if err != nil {
			t.Errorf("read %s: %v", downPath, err)
			continue
		}
		if len(strings.TrimSpace(string(downBody))) < 5 {
			t.Errorf("%s is suspiciously short (%d bytes)", downPath, len(downBody))
		}
	}

	// And no unexpected files.
	expectedCount := len(expectedNames) * 2 // up + down each
	if len(files) != expectedCount {
		t.Errorf("migration count = %d; want %d (5 original + 3 §44 = 8 × {up, down})", len(files), expectedCount)
	}
}

// TestConstitutionStateMigrationCarriesRLS is an anti-bluff smoke that
// the §42.1.2 RLS policy line is actually present in 000006. A common
// failure mode is to add a table without remembering the ENABLE RLS.
func TestConstitutionStateMigrationCarriesRLS(t *testing.T) {
	body, err := migrationsFS.ReadFile("migrations/000006_constitution_state.up.sql")
	if err != nil {
		t.Fatalf("read 000006: %v", err)
	}
	src := string(body)
	required := []string{
		"CREATE TABLE constitution_state",
		"CREATE TABLE constitution_audit",
		"ENABLE ROW LEVEL SECURITY",
		"current_setting('app.tenant_id')",
		"tenant_audit_no_update",
		"tenant_audit_no_delete",
	}
	for _, frag := range required {
		if !strings.Contains(src, frag) {
			t.Errorf("000006 missing required fragment %q", frag)
		}
	}
}

func TestConstitutionBindingsMigrationCarriesRLS(t *testing.T) {
	body, err := migrationsFS.ReadFile("migrations/000007_constitution_bindings.up.sql")
	if err != nil {
		t.Fatalf("read 000007: %v", err)
	}
	src := string(body)
	required := []string{
		"CREATE TABLE constitution_bindings",
		"ENABLE ROW LEVEL SECURITY",
		"current_setting('app.tenant_id')",
		"mode BETWEEN 0 AND 2",
	}
	for _, frag := range required {
		if !strings.Contains(src, frag) {
			t.Errorf("000007 missing required fragment %q", frag)
		}
	}
}
