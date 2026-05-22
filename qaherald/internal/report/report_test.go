package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// TestGenerate_CannedTranscript builds a self-contained <runDir> with a
// known 4-event JSONL transcript + one attachment file + a one-entry
// results.json, then asserts the generated report.md contains every
// required section AND surfaces the attachment sha. The transcript is
// shaped to mirror the §107.x evidence layout (scenario.start →
// herald.post → herald.response → scenario.end), so the test doubles
// as a documentation example of the minimum bidirectional invariant.
func TestGenerate_CannedTranscript(t *testing.T) {
	dir := t.TempDir()
	attachDir := filepath.Join(dir, "attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		t.Fatalf("mkdir attachments: %v", err)
	}

	// Constant content-addressed attachment metadata. The on-disk file
	// is zero bytes — report.go does NOT re-hash; it only checks the
	// file exists. The §11.4 attachment-sha mismatch surface is
	// covered by T10's paired mutation gate (c), not by this report
	// generator.
	const sha = "abc123def4567890abc123def4567890abc123def4567890abc123def4567890"
	attachPath := filepath.Join(attachDir, sha+".png")
	if err := os.WriteFile(attachPath, nil, 0o644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	// Build the 4 events. Timestamps are deterministic — the post →
	// response delta is 100ms, which means the rendered p50 latency
	// MUST be "100ms" (sort-then-index over a 1-element delta slice).
	base := time.Date(2026, 5, 22, 15, 30, 0, 0, time.UTC)
	events := []transcript.Event{
		{
			TS:        base,
			Direction: transcript.DirectionInternal,
			Kind:      transcript.KindScenarioStart,
			Scenario:  "happy-canned",
			Note:      "canned-transcript test scenario",
		},
		{
			TS:        base.Add(10 * time.Millisecond),
			Direction: transcript.DirectionOut,
			Kind:      transcript.KindHeraldPost,
			Scenario:  "happy-canned",
			Payload:   json.RawMessage(`{"specversion":"1.0","id":"qa-canned-1","source":"qaherald","type":"qa.canned"}`),
			Note:      "Accept: application/toon",
		},
		{
			TS:        base.Add(110 * time.Millisecond),
			Direction: transcript.DirectionIn,
			Kind:      transcript.KindHeraldResponse,
			Scenario:  "happy-canned",
			Payload:   json.RawMessage(`{"event_id":"qa-canned-1","recipients":1,"status":"accepted"}`),
			Attachments: []transcript.Attachment{
				{
					Sha256:           sha,
					ContentType:      "image/png",
					SizeBytes:        0,
					OriginalFilename: "sample.png",
				},
			},
			Note: "status=202 content-type=application/toon",
		},
		{
			TS:        base.Add(120 * time.Millisecond),
			Direction: transcript.DirectionInternal,
			Kind:      transcript.KindScenarioEnd,
			Scenario:  "happy-canned",
			Note:      "PASS=true err=<nil>",
		},
	}

	// Write transcript.jsonl.
	transcriptPath := filepath.Join(dir, "transcript.jsonl")
	tf, err := os.Create(transcriptPath)
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	enc := json.NewEncoder(tf)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
	if err := tf.Close(); err != nil {
		t.Fatalf("close transcript: %v", err)
	}

	// Write results.json — one PASS scenario at 120ms duration.
	resultsPath := filepath.Join(dir, "results.json")
	resBody := []map[string]any{
		{
			"scenario": "happy-canned",
			"pass":     true,
			"duration": int64(120 * time.Millisecond),
			"error":    "",
		},
	}
	rb, _ := json.Marshal(resBody)
	if err := os.WriteFile(resultsPath, rb, 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}

	// Generate.
	outPath := filepath.Join(dir, "report.md")
	if err := Generate(transcriptPath, resultsPath, outPath); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	s := string(body)

	// Required sections per plan T6.
	for _, want := range []string{
		"# qaherald run report",
		"## Summary",
		"## Scenario timelines",
		"## Attachment manifest",
		// Real scenario row — PASS verbatim, no coercion.
		"happy-canned",
		"PASS",
		// Attachment manifest surfaces the sha.
		"abc123",
		// Aggregate latency derived from real post/response delta.
		// One delta of 100ms → p50 == 100ms == p99.
		"100ms",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("report missing %q\n----- report.md -----\n%s", want, s)
		}
	}
}

// TestGenerate_EmptyResults verifies the §107 anti-bluff contract:
// when results.json is absent (or empty), the summary table emits the
// "(no scenarios run)" placeholder rather than fabricating a PASS row.
// This is the inverse of TestGenerate_CannedTranscript and exists so
// the T10 mutation gate (a) can blanks Writer.Append, the resulting
// run has zero events, and this code path still produces a valid
// report.md whose content honestly reflects the empty state.
func TestGenerate_EmptyResults(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, nil, 0o644); err != nil {
		t.Fatalf("write empty transcript: %v", err)
	}
	resultsPath := filepath.Join(dir, "results.json")
	if err := os.WriteFile(resultsPath, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write empty results: %v", err)
	}
	outPath := filepath.Join(dir, "report.md")
	if err := Generate(transcriptPath, resultsPath, outPath); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	body, _ := os.ReadFile(outPath)
	s := string(body)
	for _, want := range []string{
		"## Summary",
		"(no scenarios run)",
		"## Attachment manifest",
		"(none)",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("empty-state report missing %q\n----- report.md -----\n%s", want, s)
		}
	}
}
