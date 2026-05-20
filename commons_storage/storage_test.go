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

// TestParseDSN verifies the URL-form parser used by `pherald migrate`.
// Anti-bluff: every case asserts user-visible state on the returned
// *postgres.Config rather than just "no error returned".
func TestParseDSN(t *testing.T) {
	t.Run("happy_path", func(t *testing.T) {
		cfg, err := ParseDSN("postgres://herald:secret@127.0.0.1:24100/herald")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "127.0.0.1" {
			t.Errorf("Host = %q, want 127.0.0.1", cfg.Host)
		}
		if cfg.Port != 24100 {
			t.Errorf("Port = %d, want 24100", cfg.Port)
		}
		if cfg.User != "herald" {
			t.Errorf("User = %q, want herald", cfg.User)
		}
		if cfg.Password != "secret" {
			t.Errorf("Password = %q, want secret", cfg.Password)
		}
		if cfg.DBName != "herald" {
			t.Errorf("DBName = %q, want herald", cfg.DBName)
		}
		if cfg.SSLMode != "disable" {
			t.Errorf("SSLMode = %q, want disable (default)", cfg.SSLMode)
		}
	})
	t.Run("postgresql_scheme", func(t *testing.T) {
		cfg, err := ParseDSN("postgresql://u:p@h:5432/d")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DBName != "d" {
			t.Errorf("DBName = %q, want d", cfg.DBName)
		}
	})
	t.Run("sslmode_override", func(t *testing.T) {
		cfg, err := ParseDSN("postgres://u:p@h:5432/d?sslmode=require")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.SSLMode != "require" {
			t.Errorf("SSLMode = %q, want require", cfg.SSLMode)
		}
	})
	t.Run("default_port", func(t *testing.T) {
		cfg, err := ParseDSN("postgres://u:p@h/d")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != 5432 {
			t.Errorf("Port = %d, want 5432 (default)", cfg.Port)
		}
	})
	t.Run("empty_rejected", func(t *testing.T) {
		_, err := ParseDSN("")
		if err == nil {
			t.Fatal("expected error for empty DSN")
		}
	})
	t.Run("bad_scheme_rejected", func(t *testing.T) {
		_, err := ParseDSN("mysql://u:p@h:3306/d")
		if err == nil {
			t.Fatal("expected error for non-postgres scheme")
		}
		if !strings.Contains(err.Error(), "unsupported scheme") {
			t.Errorf("error should mention unsupported scheme, got %v", err)
		}
	})
	t.Run("bad_port_rejected", func(t *testing.T) {
		_, err := ParseDSN("postgres://u:p@h:notaport/d")
		if err == nil {
			t.Fatal("expected error for non-numeric port")
		}
	})
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
