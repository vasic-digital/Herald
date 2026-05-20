package storage

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSetTenantContextProducesValidSQL(t *testing.T) {
	id := uuid.MustParse("01931a7c-3f4e-7000-9abc-def012345678")
	got := SetTenantContext(id)
	if !strings.Contains(got, "SET LOCAL app.tenant_id") {
		t.Errorf("expected SET LOCAL app.tenant_id in %q", got)
	}
	if !strings.Contains(got, id.String()) {
		t.Errorf("expected UUID %q in %q", id, got)
	}
}

func TestMigrationsFS_ContainsExpectedFiles(t *testing.T) {
	expected := []string{
		"migrations/000001_init_core.up.sql",
		"migrations/000001_init_core.down.sql",
		"migrations/000002_idempotency_keys.up.sql",
		"migrations/000002_idempotency_keys.down.sql",
		"migrations/000003_subscribers.up.sql",
		"migrations/000003_subscribers.down.sql",
		"migrations/000004_channel_addresses.up.sql",
		"migrations/000004_channel_addresses.down.sql",
		"migrations/000005_inbound_pipeline.up.sql",
		"migrations/000005_inbound_pipeline.down.sql",
	}
	mFS := MigrationsFS()
	for _, name := range expected {
		f, err := mFS.Open(name)
		if err != nil {
			t.Errorf("missing embedded migration: %s (%v)", name, err)
			continue
		}
		info, err := f.Stat()
		_ = f.Close()
		if err != nil {
			t.Errorf("stat %s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("migration %s is empty", name)
		}
	}
}

func TestMigrationsFS_NoUnexpectedFiles(t *testing.T) {
	mFS := MigrationsFS()
	count := 0
	_ = fs.WalkDir(mFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".up.sql") && !strings.HasSuffix(path, ".down.sql") {
			t.Errorf("unexpected non-migration file in embed: %s", path)
		}
		count++
		return nil
	})
	if count == 0 {
		t.Error("no migration files found in embed")
	}
}
