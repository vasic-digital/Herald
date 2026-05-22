package transcript

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriter_RoundTrip exercises the §107.x evidence primitive end to
// end: write 3 events + 1 attachment to a temp dir, close, re-read the
// JSONL from disk, decode, and assert that what came out matches what
// went in — including a byte-for-byte sha256 comparison of the
// attachment.
//
// Anti-bluff: this test deliberately re-opens the file rather than
// asserting on the Writer's own state. The Wave 5 T10 mutation gate (a)
// plants a blank Append body; the JSONL on disk is then empty and the
// event-count assertion below fails. Likewise gate (c) tolerates
// sha256 mismatches; the bytes.Equal assertion catches it.
func TestWriter_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	if !strings.HasPrefix(w.RunID(), "20") {
		t.Fatalf("RunID %q does not look like an ISO year prefix", w.RunID())
	}

	for i := 0; i < 3; i++ {
		if err := w.Append(Event{
			Direction: DirectionOut,
			Kind:      KindHeraldPost,
			Note:      "event",
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	body := []byte("hello qaherald")
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])

	att, err := w.AttachFile(bytes.NewReader(body), "text/plain", "hi.txt")
	if err != nil {
		t.Fatalf("AttachFile: %v", err)
	}
	if att.Sha256 != want {
		t.Fatalf("sha256 mismatch: got %q want %q", att.Sha256, want)
	}
	if att.SizeBytes != int64(len(body)) {
		t.Fatalf("size mismatch: got %d want %d", att.SizeBytes, len(body))
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-read the JSONL from disk — verify what was actually written,
	// not what Writer claims. This is the §107 anchor: the test trusts
	// the bytes on disk, never the in-memory state.
	jsonlPath := filepath.Join(w.OutDir(), "transcript.jsonl")
	f, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var decoded []Event
	for {
		var ev Event
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode: %v", err)
		}
		decoded = append(decoded, ev)
	}
	if len(decoded) != 3 {
		t.Fatalf("expected 3 events, got %d", len(decoded))
	}
	for i, ev := range decoded {
		if ev.Direction != DirectionOut {
			t.Fatalf("event %d direction: got %q want %q", i, ev.Direction, DirectionOut)
		}
		if ev.Kind != KindHeraldPost {
			t.Fatalf("event %d kind: got %q want %q", i, ev.Kind, KindHeraldPost)
		}
		if ev.TS.IsZero() {
			t.Fatalf("event %d TS is zero — Append must stamp time", i)
		}
		if ev.Note != "event" {
			t.Fatalf("event %d note: got %q want %q", i, ev.Note, "event")
		}
	}

	// Read the attachment off disk and assert bytes equal what we
	// uploaded — the canonical §107.x sha256 round-trip check.
	got, err := os.ReadFile(w.AttachmentPath(att))
	if err != nil {
		t.Fatalf("ReadFile attachment: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("attachment bytes mismatch:\n got  %q\n want %q", got, body)
	}

	// And re-hash the bytes on disk to be doubly sure no silent
	// corruption between AttachFile and the rename target.
	got256 := sha256.Sum256(got)
	if hex.EncodeToString(got256[:]) != want {
		t.Fatalf("on-disk sha256 mismatch: got %q want %q",
			hex.EncodeToString(got256[:]), want)
	}
}
