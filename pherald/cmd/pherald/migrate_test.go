package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestMigrateCommand_StatusRequiresEnv asserts that the `pherald migrate
// status` subcommand fails meaningfully when HERALD_PG_DSN is unset (no
// auto-defaulting per §11.4.6 no-guessing — the operator must be explicit
// about which Postgres to migrate).
func TestMigrateCommand_StatusRequiresEnv(t *testing.T) {
	t.Setenv("HERALD_PG_DSN", "")
	cmd := newMigrateCmd()
	cmd.SetArgs([]string{"status"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when HERALD_PG_DSN unset")
	}
	if !strings.Contains(err.Error(), "HERALD_PG_DSN") {
		t.Fatalf("error should mention HERALD_PG_DSN, got %v", err)
	}
}

// TestMigrateCommand_RegistersSubcommands asserts that all four expected
// subcommands are wired up under `migrate`: `up` and `status` are LIVE
// (Task 6, HRD-010), while `down` and `validate` are honest 501-stubs
// retained so operators get a helpful "not yet implemented" error with
// an HRD pointer instead of Cobra's generic "unknown command".
func TestMigrateCommand_RegistersSubcommands(t *testing.T) {
	cmd := newMigrateCmd()
	subs := cmd.Commands()
	want := map[string]bool{"up": false, "status": false, "down": false, "validate": false}
	for _, sc := range subs {
		if _, ok := want[sc.Name()]; ok {
			want[sc.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("migrate subcommand %q not registered", name)
		}
	}
}

// TestMigrateCommand_DownReturnsHelpfulError asserts that `pherald migrate
// down` returns an explicit "not yet implemented" error pointing operators
// at docs/Issues.md (HRD pointer) rather than a generic cobra error or a
// silent no-op. This is the honest-stub contract for destructive ops per
// §9.1.
func TestMigrateCommand_DownReturnsHelpfulError(t *testing.T) {
	cmd := newMigrateCmd()
	cmd.SetArgs([]string{"down"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from down stub")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("error should explain not-implemented, got %v", err)
	}
	if !strings.Contains(err.Error(), "HRD") {
		t.Fatalf("error should reference HRD/docs/Issues.md, got %v", err)
	}
}

// TestMigrateCommand_ValidateReturnsHelpfulError mirrors the down-stub
// contract for the validate subcommand.
func TestMigrateCommand_ValidateReturnsHelpfulError(t *testing.T) {
	cmd := newMigrateCmd()
	cmd.SetArgs([]string{"validate"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from validate stub")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("error should explain not-implemented, got %v", err)
	}
	if !strings.Contains(err.Error(), "HRD") {
		t.Fatalf("error should reference HRD/docs/Issues.md, got %v", err)
	}
}
