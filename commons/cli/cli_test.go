package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestStubCmd_ReturnsErrorWithHRDPointer(t *testing.T) {
	cmd := StubCmd("destructive-guard", "HRD-033", "wrap rm + git-reset with prerequisite checks")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetOut(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-nil error from stub")
	}
	msg := err.Error()
	if !strings.Contains(msg, "HRD-033") {
		t.Errorf("error should contain HRD reference, got: %q", msg)
	}
	if !strings.Contains(msg, "destructive-guard") {
		t.Errorf("error should contain command name, got: %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "not yet implemented") {
		t.Errorf("error should explain non-implementation, got: %q", msg)
	}
}
