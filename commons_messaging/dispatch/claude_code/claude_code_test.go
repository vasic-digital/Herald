package claude_code

import (
	"context"
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

// TestDispatchCommandIncludesOpusModel proves the Wave 6 operator-locked
// Opus pinning is load-bearing on the literal argv of the spawned `claude`
// process — not "intended via config", not "passed as env var hopefully
// read by the binary". Inspects the *exec.Cmd.Args slice constructed by
// the internal buildCmd helper (exported for tests via export_test.go).
//
// §107 anti-bluff: the assertion requires `--model` and `claude-opus-4-7`
// to appear as two contiguous slice entries. A single joined string like
// "--model claude-opus-4-7" (one argv entry) would be rejected by the
// Claude CLI flag parser at runtime; the test catches that too.
//
// Wave 6 T12 mutation gate (b) swaps the literal to "claude-sonnet-4-6"
// and asserts this test FAILs — proving the assertion catches real model
// drift rather than passing trivially.
func TestDispatchCommandIncludesOpusModel(t *testing.T) {
	d, err := New("/nonexistent/claude", t.TempDir(), "TestProj")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Pre-create the session anchor so ResolveSession returns a non-Nil UUID.
	// buildCmd never spawns the binary; it only assembles argv.
	anchorDir := filepath.Join(d.WorkingDirForTest(), ".herald", "claude-code", "sessions")
	if err := os.MkdirAll(anchorDir, 0o755); err != nil {
		t.Fatalf("mkdir anchor dir: %v", err)
	}
	anchorFile := filepath.Join(anchorDir, "TestProj.session")
	if err := os.WriteFile(anchorFile, []byte("11111111-2222-3333-4444-555555555555\n"), 0o644); err != nil {
		t.Fatalf("write anchor: %v", err)
	}

	cmd, err := d.BuildCmdForTest(context.Background(), DispatchRequest{
		InboundID:    "INB-OPUS-1",
		Sender:       "tgram:test",
		Channel:      commons.ChannelTelegram,
		UserMessage:  "hi",
		Conversation: "(no prior thread)",
		Classification: Classification{
			Type:        "query",
			Criticality: "low",
			Confidence:  0.5,
		},
	})
	if err != nil {
		t.Fatalf("BuildCmdForTest: %v", err)
	}

	args := cmd.Args
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--model" && args[i+1] == "claude-opus-4-7" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("argv missing contiguous [--model, claude-opus-4-7]; got: %v", args)
	}

	// Also defensive: ensure the model arg appears between --resume and --print
	// (positional sanity — `claude` accepts flags in any order, but we
	// document the intent here so a future refactor doesn't accidentally
	// place --model after the envelope payload, which would make the
	// envelope text be parsed as the model name).
	idxResume := -1
	idxModel := -1
	idxPrint := -1
	for i, a := range args {
		switch a {
		case "--resume":
			idxResume = i
		case "--model":
			idxModel = i
		case "--print":
			idxPrint = i
		}
	}
	if idxResume < 0 || idxModel < 0 || idxPrint < 0 {
		t.Fatalf("expected --resume, --model, --print all present; got args: %v", args)
	}
	if !(idxResume < idxModel && idxModel < idxPrint) {
		t.Fatalf("expected ordering --resume < --model < --print; got resume=%d model=%d print=%d in args: %v",
			idxResume, idxModel, idxPrint, args)
	}
}

// TestFormatEnvelopePreText proves the Wave 6 operator-mandated pre-text
// wrapper renders verbatim. The opening sentence ("We have received new
// message from our communication channel <name>.") MUST appear as a
// strict prefix of the rendered output — §107 forensic anchor (operator
// mandate, 2026-05-22). The existing structured <<<HERALD-DISPATCH-v1>>>
// block is preserved byte-for-byte; FormatEnvelopeWithPreText only adds
// the human-language prelude.
//
// Five sub-assertions: (a) verbatim prefix, (b) ordering (pre-text before
// structured marker), (c) blank-line separator, (d) attachment surfaces
// in pre-text, (e) classification surfaces in pre-text.
func TestFormatEnvelopePreText(t *testing.T) {
	d, err := New("/bin/true", t.TempDir(), "AtmosphereProject")
	if err != nil {
		t.Fatal(err)
	}
	req := DispatchRequest{
		InboundID:      "01HXYZ",
		Sender:         "tgram:milos85vasic",
		Channel:        commons.ChannelTelegram,
		Classification: Classification{Type: "query", Criticality: "low", Confidence: 0.88},
		Conversation:   "milos: ping?",
		Attachments:    []commons.Attachment{{Filename: "shot.png", MIMEType: "image/png", SizeBytes: 1234}},
		UserMessage:    "ping",
	}
	out := d.FormatEnvelopeWithPreText(req, "tgram")

	// (a) verbatim opening line (the operator's mandated wording)
	if !strings.HasPrefix(out, "We have received new message from our communication channel tgram.") {
		end := 80
		if end > len(out) {
			end = len(out)
		}
		t.Fatalf("missing verbatim pre-text opener; got first 80 bytes: %q", out[:end])
	}
	// (b) pre-text appears BEFORE the structured marker
	preIdx := strings.Index(out, "We have received new message")
	markerIdx := strings.Index(out, "<<<HERALD-DISPATCH-v1>>>")
	if preIdx < 0 || markerIdx < 0 || preIdx >= markerIdx {
		t.Fatalf("ordering wrong: preIdx=%d markerIdx=%d", preIdx, markerIdx)
	}
	// (c) blank line between pre-text and structured block
	blank := strings.Index(out[preIdx:], "\n\n<<<HERALD-DISPATCH-v1>>>")
	if blank < 0 {
		t.Fatalf("no blank line between pre-text and structured marker")
	}
	// (d) attachment filename surfaces in the pre-text (so the LLM sees
	//     the attachment context in natural language before the structured
	//     list)
	if !strings.Contains(out, "shot.png") {
		t.Fatalf("attachment filename not surfaced in pre-text")
	}
	// (e) classification surfaces in the pre-text
	if !strings.Contains(strings.ToLower(out), "query") {
		t.Fatalf("classification not surfaced in pre-text")
	}
}

// TestFormatEnvelopePreText_ActionGuidance — Wave 6.5 §107 fix (2026-05-23).
// Without explicit ACTION FORMAT GUIDANCE the LLM defaulted to action:"reply"
// for every classification, leaving the issue.open pipeline dead in production
// (HRD-101 S5 live evidence). This test asserts the guidance + the
// SUGGESTED ACTION line both surface in the rendered envelope.
func TestFormatEnvelopePreText_ActionGuidance(t *testing.T) {
	d, _ := New("/bin/true", t.TempDir(), "Guidance")

	cases := []struct {
		name       string
		classType  string
		wantAction string // substring that MUST appear in the SUGGESTED ACTION line
	}{
		{"bug", "bug", "emit issue.open"},
		{"task", "task", "emit issue.open"},
		{"implementation", "implementation", "emit issue.open"},
		{"investigation", "investigation", "emit issue.open"},
		{"query", "query", "emit reply"},
		{"empty", "", "emit reply"},
		{"event_trigger", "event_trigger", "emit event.emit"},
		{"unknown", "moonshot", "asking for clarification"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := DispatchRequest{
				InboundID:      "01HXYZ",
				Sender:         "tgram:milos",
				Channel:        commons.ChannelTelegram,
				Classification: Classification{Type: tc.classType, Criticality: "middle", Confidence: 1.0},
				UserMessage:    "x",
			}
			out := d.FormatEnvelopeWithPreText(req, "tgram")

			// Generic guidance block must be present for every classification.
			if !strings.Contains(out, "ACTION FORMAT GUIDANCE") {
				t.Fatalf("envelope missing ACTION FORMAT GUIDANCE block")
			}
			if !strings.Contains(out, "<<<HERALD-REPLY>>>") {
				t.Fatalf("envelope missing <<<HERALD-REPLY>>> marker reference")
			}
			if !strings.Contains(out, "\"action\":\"issue.open\"") {
				t.Fatalf("envelope missing issue.open JSON example")
			}
			if !strings.Contains(out, "\"action\":\"reply\"") {
				t.Fatalf("envelope missing reply JSON example")
			}

			// Per-type SUGGESTED ACTION line must match the classification.
			if !strings.Contains(out, "SUGGESTED ACTION") {
				t.Fatalf("envelope missing SUGGESTED ACTION line")
			}
			if !strings.Contains(out, tc.wantAction) {
				t.Fatalf("SUGGESTED ACTION missing %q for type=%q; envelope:\n%s",
					tc.wantAction, tc.classType, out)
			}

			// Guidance + suggestion appear BEFORE the structured marker.
			guidanceIdx := strings.Index(out, "ACTION FORMAT GUIDANCE")
			suggIdx := strings.Index(out, "SUGGESTED ACTION")
			markerIdx := strings.Index(out, "<<<HERALD-DISPATCH-v1>>>")
			if guidanceIdx < 0 || suggIdx < 0 || markerIdx < 0 {
				t.Fatalf("missing anchor — guidance=%d sugg=%d marker=%d", guidanceIdx, suggIdx, markerIdx)
			}
			if !(guidanceIdx < suggIdx && suggIdx < markerIdx) {
				t.Fatalf("ordering wrong — guidance=%d sugg=%d marker=%d", guidanceIdx, suggIdx, markerIdx)
			}
		})
	}
}

// TestFormatEnvelope_IntentInferenceInstruction is the TIER 2 envelope check
// (docs/design/INTENT_RECOGNITION.md §4): the rendered envelope MUST instruct
// the LLM that users speak plain language (no command syntax), to map natural
// language onto Herald's command set, and to return action=clarify with a
// precise question rather than guess when the intent is not determinable. A
// regression that drops this instruction would let the LLM silently guess
// (§11.4.6 no-guessing violation) — this test bites.
func TestFormatEnvelope_IntentInferenceInstruction(t *testing.T) {
	d, _ := New("/bin/true", t.TempDir(), "Intent")
	req := DispatchRequest{
		InboundID:      "01HXYZ",
		Sender:         "tgram:milos",
		Channel:        commons.ChannelTelegram,
		Classification: Classification{Type: "query", Criticality: "low", Confidence: 0.5},
		UserMessage:    "do the ATM-9 thing",
	}
	out := d.FormatEnvelopeWithPreText(req, "tgram")

	mustContain := []string{
		"INTENT RECOGNITION",   // the instruction block header
		"PLAIN LANGUAGE",       // users speak plain language
		"action=clarify",       // the clarify fallback directive
		"DO NOT guess",         // no-guessing
		"investigation.start",  // command-set mapping is present
		"\"action\":\"clarify\"", // the clarify JSON example
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Fatalf("intent-inference instruction missing %q in envelope:\n%s", s, out)
		}
	}

	// The instruction appears BEFORE the structured dispatch marker.
	instrIdx := strings.Index(out, "INTENT RECOGNITION")
	markerIdx := strings.Index(out, "<<<HERALD-DISPATCH-v1>>>")
	if instrIdx < 0 || markerIdx < 0 || instrIdx >= markerIdx {
		t.Fatalf("ordering wrong: instrIdx=%d markerIdx=%d", instrIdx, markerIdx)
	}
}

func TestFormatEnvelope_TaskVerbVariesByType(t *testing.T) {
	d, _ := New("claude", t.TempDir(), "X")
	for itemType, expect := range map[string]string{
		"bug":            "reproduce + identify affected code paths",
		"query":          "research + answer",
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
