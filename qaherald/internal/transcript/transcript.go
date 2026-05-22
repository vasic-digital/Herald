// Package transcript provides the qaherald JSONL transcript writer and
// content-addressed attachment store.
//
// Every Wave 5 scenario records its bidirectional events to
// <out>/transcript.jsonl (append-only, fsync'd per event) and stages
// attachments under <out>/attachments/<sha256>.<ext>. The run-ID is
// computed once at Writer construction: RFC3339-ish + a UUIDv7 short
// suffix (operator-locked 2026-05-22 — see
// docs/superpowers/plans/2026-05-22-wave5-qaherald.md).
//
// §107.x anchor: this Writer is the SOLE interface scenarios use to
// record observed behaviour. Scenarios never fmt.Println, never write
// directly to disk — every byte of qaherald's auditable evidence flows
// through Append + AttachFile. The Wave 5 mutation gate (T10 paired
// §1.1) plants a blank Append body; every scenario then fails its
// post-condition because no transcript events are recorded.
package transcript

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Direction enumerates which leg of the bidirectional channel an event
// belongs to. "out" means qaherald → external service (Telegram or
// pherald); "in" means external service → qaherald. "internal" means a
// qaherald-internal step (scenario assertion, timing checkpoint).
type Direction string

const (
	DirectionOut      Direction = "out"
	DirectionIn       Direction = "in"
	DirectionInternal Direction = "internal"
)

// Kind enumerates the event class. The full enum lives in
// docs/specs/mvp/specification.V3.md §47 (T11 addition); the values
// below are stable across all scenarios.
type Kind string

const (
	KindTGSend         Kind = "tg.send"
	KindTGReceive      Kind = "tg.receive"
	KindTGUpload       Kind = "tg.upload"
	KindTGDownload     Kind = "tg.download"
	KindHeraldPost     Kind = "herald.post"
	KindHeraldResponse Kind = "herald.response"
	KindHeraldGet      Kind = "herald.get"
	KindScenarioStart  Kind = "scenario.start"
	KindScenarioEnd    Kind = "scenario.end"
	KindAssert         Kind = "assert"
	KindWait           Kind = "wait"
)

// Attachment carries the sha256-indexed file metadata. The actual bytes
// live at <out>/attachments/<Sha256>.<Ext>.
type Attachment struct {
	Sha256           string `json:"sha256"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	OriginalFilename string `json:"original_filename,omitempty"`
}

// Event is one JSONL row.
type Event struct {
	TS          time.Time       `json:"ts"`
	Direction   Direction       `json:"direction"`
	Kind        Kind            `json:"kind"`
	Scenario    string          `json:"scenario,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Attachments []Attachment    `json:"attachments,omitempty"`
	Note        string          `json:"note,omitempty"`
}

// Writer is the single entry point for transcript recording. It is
// safe for concurrent use.
type Writer struct {
	mu        sync.Mutex
	runID     string
	outDir    string
	attachDir string
	file      *os.File
	enc       *json.Encoder
}

// NewWriter creates <outDirParent>/<runID>/ + transcript.jsonl +
// attachments/. The run-ID is RFC3339-ish + UUIDv7 short suffix.
func NewWriter(outDirParent string) (*Writer, error) {
	runID := computeRunID(time.Now().UTC())
	outDir := filepath.Join(outDirParent, runID)
	attachDir := filepath.Join(outDir, "attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		return nil, fmt.Errorf("transcript: mkdir %s: %w", attachDir, err)
	}
	f, err := os.OpenFile(filepath.Join(outDir, "transcript.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("transcript: open jsonl: %w", err)
	}
	return &Writer{
		runID:     runID,
		outDir:    outDir,
		attachDir: attachDir,
		file:      f,
		enc:       json.NewEncoder(f),
	}, nil
}

// RunID returns the canonical run identifier computed at construction.
func (w *Writer) RunID() string { return w.runID }

// OutDir returns the absolute path of <outDirParent>/<runID>/.
func (w *Writer) OutDir() string { return w.outDir }

// Append writes one event to the JSONL file with an immediate fsync —
// scenario failures must not lose the last few events.
func (w *Writer) Append(ev Event) error {
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enc.Encode(ev); err != nil {
		return fmt.Errorf("transcript: encode: %w", err)
	}
	return w.file.Sync()
}

// AttachFile reads r, computes sha256 streamingly, persists under
// <outDir>/attachments/<sha256>.<ext>, returns the Attachment record
// the caller embeds in its Event.
func (w *Writer) AttachFile(r io.Reader, contentType, originalFilename string) (Attachment, error) {
	ext := pickExt(contentType, originalFilename)
	tmp, err := os.CreateTemp(w.attachDir, "stage-*"+ext)
	if err != nil {
		return Attachment{}, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		tmp.Close()
		return Attachment{}, err
	}
	if err := tmp.Close(); err != nil {
		return Attachment{}, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	final := filepath.Join(w.attachDir, sum+ext)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return Attachment{}, err
	}
	return Attachment{
		Sha256:           sum,
		ContentType:      contentType,
		SizeBytes:        n,
		OriginalFilename: originalFilename,
	}, nil
}

// Close releases the underlying transcript file handle.
func (w *Writer) Close() error { return w.file.Close() }

// AttachmentPath returns the on-disk path for a recorded Attachment.
// Reports use this for the sha256 manifest section.
func (w *Writer) AttachmentPath(a Attachment) string {
	return filepath.Join(w.attachDir, a.Sha256+pickExt(a.ContentType, a.OriginalFilename))
}

func computeRunID(now time.Time) string {
	ts := now.Format("2006-01-02T15-04-05")
	u, _ := uuid.NewV7()
	s := u.String()
	return fmt.Sprintf("%s-%s", ts, s[len(s)-4:])
}

func pickExt(contentType, origName string) string {
	if origName != "" {
		if e := filepath.Ext(origName); e != "" {
			return e
		}
	}
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}
