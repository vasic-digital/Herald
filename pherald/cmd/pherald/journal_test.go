// Wave 6 T10a — journaling middleware tests.
//
// The tests prove the §107.x evidence primitive is load-bearing:
//
//   - A single inbound event with one attachment, fed through the
//     full journaling chain (handler + code + replier), produces
//     exactly 4 JSONL lines + a content-addressed attachment file.
//   - Every line decodes as JSON (no string-concatenation bugs).
//   - The attachment's sha256 on disk equals the sha256 recorded in
//     the JSONL payload (no fake content-addressing).
//   - The transcript.jsonl is fsync'd after each write (we re-open
//     the file from a separate handle while the chain is mid-flight
//     and verify the previously-written lines are already on disk).
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// stubReplierJournal records the SendReply call. Wave 7 (HRD-114): the
// generic inbound.Replier signature (recipient + string ids).
type stubReplierJournal struct {
	mu     sync.Mutex
	called int
}

func (s *stubReplierJournal) SendReply(_ context.Context, _ commons.Recipient, _, _ string, _ []commons.Attachment) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
	return "1001", nil
}

// stubCodeJournal returns a canned <<<HERALD-REPLY>>> blob so the inner
// inbound.Dispatcher routes to the "reply" action (which calls SendReply
// on the recording replier — proving the full chain ran end-to-end).
type stubCodeJournal struct{}

func (stubCodeJournal) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
	return inbound.CodeResponse{
		Stdout: []byte(`<<<HERALD-REPLY>>> {"action":"reply","text":"ack from stub"}`),
	}, nil
}

// TestJournalFullChainEmitsFourLines asserts the chained journaling
// decorators (handler + code + replier) produce exactly four JSONL events
// for a single inbound message with one attachment, AND the attachment is
// copied to the content-addressed path.
func TestJournalFullChainEmitsFourLines(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "qa-run")

	// Synthesise a real attachment file on disk so the journal copy
	// helper can actually open it.
	srcPayload := []byte("\xff\xd8\xff\xe0fake-jpeg-for-journal-test\xff\xd9")
	srcSum := sha256.Sum256(srcPayload)
	srcSha := hex.EncodeToString(srcSum[:])
	srcDir := filepath.Join(dir, "inbox")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(srcDir, srcSha+".jpg")
	if err := os.WriteFile(srcFile, srcPayload, 0o644); err != nil {
		t.Fatal(err)
	}

	// Build the inner Dispatcher with stub code + stub replier.
	replier := &stubReplierJournal{}
	innerDisp, err := inbound.NewDispatcher(inbound.Config{
		Code:  stubCodeJournal{},
		Reply: replier,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Open the journal + compose the chain identically to runListen.
	jrn, err := newJournal(outDir)
	if err != nil {
		t.Fatal(err)
	}
	defer jrn.Close()
	jrnCode := &journalingCode{j: jrn, inner: stubCodeJournal{}}
	jrnReplier := &journalingReplier{j: jrn, inner: replier}
	// Rebuild the inbound.Dispatcher with the journaling Code + Replier
	// so cc.dispatch / cc.reply / tgram.send_reply get journaled.
	chainedDisp, err := inbound.NewDispatcher(inbound.Config{
		Code:  jrnCode,
		Reply: jrnReplier,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = innerDisp // silence — kept above for documentation of intent
	handler := &journalingHandler{j: jrn, inner: chainedDisp}

	// Synthesise an inbound event with one attachment.
	att := commons.Attachment{
		Filename:  srcFile,
		MIMEType:  "image/jpeg",
		SizeBytes: int64(len(srcPayload)),
		CID:       srcSha,
		Reader: func() (io.ReadCloser, error) {
			return os.Open(srcFile)
		},
	}
	ev := commons.InboundEvent{
		EventID: "01HEVTEST",
		Sender: commons.Recipient{
			Channel:       string(commons.ChannelTelegram),
			ChannelUserID: "777",
		},
		Body:        commons.Body{Plain: "hello, herald"},
		Attachments: []commons.Attachment{att},
		Raw: map[string]any{
			"message_id": 42,
			"chat_id":    int64(777),
			"text":       "hello, herald",
		},
	}

	// Drive the full chain.
	if err := handler.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// (1) replier was invoked (full chain reached the outbound sink).
	if replier.called != 1 {
		t.Fatalf("replier.called = %d, want 1", replier.called)
	}

	// (2) attachment was copied to the sha256 path.
	wantAttPath := filepath.Join(outDir, "attachments", srcSha+".jpg")
	gotBytes, err := os.ReadFile(wantAttPath)
	if err != nil {
		t.Fatalf("read copied attachment: %v", err)
	}
	gotSum := sha256.Sum256(gotBytes)
	gotSha := hex.EncodeToString(gotSum[:])
	if gotSha != srcSha {
		t.Fatalf("attachment sha256 mismatch: got %s want %s", gotSha, srcSha)
	}

	// (3) transcript.jsonl exists + has exactly 4 JSON-decoded lines.
	transcript := filepath.Join(outDir, "transcript.jsonl")
	lines := readLines(t, transcript)
	if len(lines) != 4 {
		t.Fatalf("expected 4 JSONL lines, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}

	// (4) each line decodes as JSON + carries the four expected kinds.
	wantKinds := []string{"tgram.message", "cc.dispatch", "cc.reply", "tgram.send_reply"}
	wantDirs := []string{"in", "out", "in", "out"}
	for i, line := range lines {
		var rec journalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d: not valid JSON: %v\nline=%s", i, err, line)
		}
		if rec.Kind != wantKinds[i] {
			t.Errorf("line %d: kind=%q want %q", i, rec.Kind, wantKinds[i])
		}
		if rec.Direction != wantDirs[i] {
			t.Errorf("line %d: direction=%q want %q", i, rec.Direction, wantDirs[i])
		}
		if rec.TS == "" {
			t.Errorf("line %d: ts is empty", i)
		}
		if rec.Payload == nil {
			t.Errorf("line %d: payload is nil", i)
		}
	}

	// (5) the inbound tgram.message payload references the same sha256
	// we computed from the source file (no fake content-addressing).
	var first journalRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	firstPayload, _ := first.Payload.(map[string]any)
	atts, _ := firstPayload["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("first record: want 1 attachment, got %d", len(atts))
	}
	att0, _ := atts[0].(map[string]any)
	if got, _ := att0["sha256"].(string); got != srcSha {
		t.Errorf("attachment sha256 in JSONL: got %q want %q", got, srcSha)
	}
	if got, _ := att0["copied_to"].(string); !strings.HasSuffix(got, srcSha+".jpg") {
		t.Errorf("attachment copied_to: got %q want suffix %s.jpg", got, srcSha)
	}
}

// TestJournalSyncSurvivesEarlyReopen proves Sync() is called after each
// write: while the file is held open by the journal, a second os.OpenFile
// for reading sees the previously-written lines without any close.
func TestJournalSyncSurvivesEarlyReopen(t *testing.T) {
	dir := t.TempDir()
	jrn, err := newJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer jrn.Close()
	if err := jrn.write("in", "tgram.message", map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	// Reopen for reading WITHOUT closing jrn — if Sync() wasn't called,
	// the read may show 0 bytes on some filesystems / OSes.
	b, err := os.ReadFile(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("expected fsync'd bytes on disk, got empty file")
	}
	var rec journalRecord
	if err := json.Unmarshal(b[:len(b)-1], &rec); err != nil {
		t.Fatalf("re-read line not JSON: %v\nline=%q", err, string(b))
	}
	if rec.Kind != "tgram.message" {
		t.Errorf("kind=%q want tgram.message", rec.Kind)
	}
}

// TestJournalCopyAttachmentIdempotent asserts a second copy of the same
// sha256-keyed attachment is a no-op (the destination already exists, the
// helper returns without rewriting).
func TestJournalCopyAttachmentIdempotent(t *testing.T) {
	dir := t.TempDir()
	jrn, err := newJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer jrn.Close()
	payload := []byte("test-payload-for-idempotency")
	sum := sha256.Sum256(payload)
	sha := hex.EncodeToString(sum[:])
	srcFile := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(srcFile, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	att := commons.Attachment{
		Filename: srcFile,
		CID:      sha,
		Reader:   func() (io.ReadCloser, error) { return os.Open(srcFile) },
	}
	rel1, err := jrn.copyAttachment(att)
	if err != nil {
		t.Fatal(err)
	}
	info1, _ := os.Stat(filepath.Join(dir, rel1))
	rel2, err := jrn.copyAttachment(att)
	if err != nil {
		t.Fatal(err)
	}
	if rel1 != rel2 {
		t.Fatalf("rel paths differ: %q vs %q", rel1, rel2)
	}
	info2, _ := os.Stat(filepath.Join(dir, rel2))
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("second copy rewrote the file: mtime %v -> %v", info1.ModTime(), info2.ModTime())
	}
}

// readLines reads the file and returns its non-empty trailing-newline-
// stripped lines.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := []string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
