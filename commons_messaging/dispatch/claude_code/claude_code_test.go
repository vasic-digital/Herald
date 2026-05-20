package claude_code

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
)

func TestNew_EmptyProjectFails(t *testing.T) {
	if _, err := New("claude", "/tmp", ""); err == nil {
		t.Errorf("New must reject empty project_name (spec §18.2.5)")
	}
}

func TestResolveSession_NoAnchorReturnsNil(t *testing.T) {
	dir := t.TempDir()
	d, err := New("claude", dir, "TestProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	u, anchor, err := d.ResolveSession()
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if u != uuid.Nil {
		t.Errorf("expected uuid.Nil when no anchor, got %s", u)
	}
	want := filepath.Join(dir, ".herald", "claude-code", "sessions", "TestProj.session")
	if anchor != want {
		t.Errorf("anchor = %q, want %q", anchor, want)
	}
}

func TestResolveSession_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	d, _ := New("claude", dir, "RoundTrip")

	u := uuid.New()
	_, anchor, _ := d.ResolveSession()
	if err := d.PersistSession(u, anchor); err != nil {
		t.Fatalf("PersistSession: %v", err)
	}

	got, _, err := d.ResolveSession()
	if err != nil {
		t.Fatalf("ResolveSession after persist: %v", err)
	}
	if got != u {
		t.Errorf("persisted %s, resolved %s", u, got)
	}
}

func TestResolveSession_InvalidUUID(t *testing.T) {
	dir := t.TempDir()
	d, _ := New("claude", dir, "Bad")
	_, anchor, _ := d.ResolveSession()
	_ = os.MkdirAll(filepath.Dir(anchor), 0o755)
	_ = os.WriteFile(anchor, []byte("not-a-uuid"), 0o644)
	if _, _, err := d.ResolveSession(); err == nil {
		t.Errorf("expected error parsing non-UUID anchor")
	}
}

func TestFormatEnvelope_ContainsExpectedFields(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "ATMOSphere")
	env := d.FormatEnvelope(DispatchRequest{
		InboundID:   "INB-1",
		Sender:      "tgram:alice",
		Channel:     commons.ChannelTelegram,
		UserMessage: "Bug: telemetry pipe drops every hour",
		Classification: Classification{
			Type:        "bug",
			Criticality: "high",
			Confidence:  0.92,
		},
		Conversation: "(no prior thread)",
		Attachments:  []commons.Attachment{{Filename: "log.txt", MIMEType: "text/plain", SizeBytes: 1024}},
	})
	checks := []string{
		"<<<HERALD-DISPATCH-v1>>>",
		"Project:        ATMOSphere",
		"Inbound ID:     INB-1",
		"Sender:         tgram:alice",
		"Channel:        tgram",
		"type=bug criticality=high confidence=0.92",
		"log.txt:text/plain:1024",
		"Bug: telemetry pipe drops every hour",
		"<<<HERALD-REPLY>>>",
		"DO NOT commit. DO NOT push.",
		"<<<END-HERALD-DISPATCH>>>",
	}
	for _, want := range checks {
		if !strings.Contains(env, want) {
			t.Errorf("envelope missing %q\n---ENVELOPE---\n%s", want, env)
		}
	}
}

func TestFormatEnvelope_TaskVerbVariesByType(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "X")
	for itemType, expect := range map[string]string{
		"bug":           "reproduce + identify affected code paths",
		"query":         "research + answer",
		"implementation": "scope effort + propose approach",
	} {
		env := d.FormatEnvelope(DispatchRequest{
			Classification: Classification{Type: itemType},
		})
		if !strings.Contains(env, expect) {
			t.Errorf("envelope for type=%q missing task verb %q", itemType, expect)
		}
	}
}
