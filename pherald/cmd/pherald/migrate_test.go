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

// TestMigrateCommand_RegistersSubcommands asserts that both `up` and
// `status` are wired up under `migrate`. The pre-Task-6 stub registered
// `up`, `down`, `status`, `validate`; Task 6 spec says implement up+status,
// the others stay as future work but their absence shouldn't break this
// test — we only assert the two we're implementing are present.
func TestMigrateCommand_RegistersSubcommands(t *testing.T) {
	cmd := newMigrateCmd()
	subs := cmd.Commands()
	want := map[string]bool{"up": false, "status": false}
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
