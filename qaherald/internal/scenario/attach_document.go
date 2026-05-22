// Wave 5 Task 5 Scenario #6 — attachment-upload-document.
//
// Two variants in the same scenario file:
//   - "small" — a minimal valid PDF (~250 bytes). Round-trip
//     sha256-match assertion identical to scenario #5.
//   - "oversized" — an 11MB application/octet-stream blob. Asserts
//     pherald returns 413 (or the operator-configured cap). Logs the
//     413 response as a sentinel TG-side event so the bidirectional
//     invariant holds even though no real delivery occurs.
//
// The full scenario runs the small variant first; on success it runs
// the oversized variant. Both share the scenario name; the report
// disambiguates via the Note field on each event.
//
// §107 anti-bluff anchor: same as scenario #5 — full sha256 equality
// for the small variant. The oversized variant defends against a
// silent-413 bluff (a server that accepts every payload and pretends
// 413).
package scenario

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

func init() {
	Register(Scenario{
		Name:        "attachment-upload-document",
		Description: "Document upload (small PDF + 11MB oversized variant)",
		Run:         runAttachDocument,
	})
}

func runAttachDocument(ctx context.Context, o *Orchestrator) error {
	if err := runAttachDocumentSmall(ctx, o); err != nil {
		return fmt.Errorf("small variant: %w", err)
	}
	if err := runAttachDocumentOversized(ctx, o); err != nil {
		return fmt.Errorf("oversized variant: %w", err)
	}
	return nil
}

func runAttachDocumentSmall(ctx context.Context, o *Orchestrator) error {
	body, contentType, filename := minimalPDFBytes()
	uploadedSum := sha256.Sum256(body)
	uploadedHex := hex.EncodeToString(uploadedSum[:])

	attach, err := o.Transcript.AttachFile(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("transcript.AttachFile (upload): %w", err)
	}
	uploadPayload, _ := json.Marshal(map[string]any{
		"variant":      "small",
		"filename":     filename,
		"content_type": contentType,
		"sha256":       uploadedHex,
		"size_bytes":   len(body),
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction:   transcript.DirectionOut,
		Kind:        transcript.KindTGUpload,
		Scenario:    "attachment-upload-document",
		Payload:     uploadPayload,
		Attachments: []transcript.Attachment{attach},
		Note:        "small variant — minimal PDF",
	})

	msgID, fileID, err := o.TG.Upload(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("TG.Upload: %w", err)
	}

	heraldNotePayload, _ := json.Marshal(map[string]any{
		"variant":             "small",
		"telegram_message_id": msgID,
		"telegram_file_id":    fileID,
		"sha256":              uploadedHex,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "attachment-upload-document",
		Payload:   heraldNotePayload,
		Note:      "small variant — bot upload → pherald CloudEvent ingest",
	})

	deliveredMsg, err := o.TG.WaitForMessage(60*time.Second, func(m tele.Message) bool {
		return m.Document != nil && m.Document.FileID != "" && m.Document.FileID != fileID
	})
	if err != nil {
		return fmt.Errorf("WaitForMessage (small delivery): %w", err)
	}
	deliveredFileID := deliveredMsg.Document.FileID
	deliveredPayload, _ := json.Marshal(map[string]any{
		"variant":           "small",
		"message_id":        deliveredMsg.ID,
		"delivered_file_id": deliveredFileID,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "attachment-upload-document",
		Payload:   deliveredPayload,
		Note:      "small variant — pherald → tgram → chat delivery",
	})

	rc, err := o.TG.Download(deliveredFileID)
	if err != nil {
		return fmt.Errorf("TG.Download: %w", err)
	}
	defer rc.Close()
	downloadedBytes, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read downloaded body: %w", err)
	}
	downloadedSum := sha256.Sum256(downloadedBytes)
	downloadedHex := hex.EncodeToString(downloadedSum[:])
	dlAttach, err := o.Transcript.AttachFile(bytes.NewReader(downloadedBytes), contentType, "qa-roundtrip.pdf")
	if err != nil {
		return fmt.Errorf("transcript.AttachFile (download): %w", err)
	}
	downloadPayload, _ := json.Marshal(map[string]any{
		"variant":    "small",
		"file_id":    deliveredFileID,
		"sha256":     downloadedHex,
		"size_bytes": len(downloadedBytes),
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction:   transcript.DirectionIn,
		Kind:        transcript.KindTGDownload,
		Scenario:    "attachment-upload-document",
		Payload:     downloadPayload,
		Attachments: []transcript.Attachment{dlAttach},
		Note:        "small variant — round-tripped PDF bytes",
	})

	if !bytes.Equal(uploadedSum[:], downloadedSum[:]) {
		return fmt.Errorf("attach-document small: sha256 mismatch — uploaded=%s downloaded=%s", uploadedHex, downloadedHex)
	}
	return nil
}

// runAttachDocumentOversized pushes 11MB through pherald's /v1/events
// directly (NOT via Telegram upload — Telegram's own 50MB upload cap
// would reject a 11MB document silently otherwise, masking pherald's
// own 413 contract). We base64-encode the bytes into the CloudEvent's
// data_base64 field, POST it, and assert pherald returns 413.
func runAttachDocumentOversized(ctx context.Context, o *Orchestrator) error {
	body, contentType, filename := largeDocumentBytes()
	bodySum := sha256.Sum256(body)
	bodyHex := hex.EncodeToString(bodySum[:])

	// Note: this attaches the 11MB blob to the transcript on-disk.
	// At 11MB the run directory grows by ~22MB (one upload event +
	// the eventual report manifest reference), which is acceptable
	// for a single-scenario QA run.
	attach, err := o.Transcript.AttachFile(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("transcript.AttachFile (oversized upload): %w", err)
	}
	uploadPayload, _ := json.Marshal(map[string]any{
		"variant":      "oversized",
		"filename":     filename,
		"content_type": contentType,
		"sha256":       bodyHex,
		"size_bytes":   len(body),
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction:   transcript.DirectionOut,
		Kind:        transcript.KindTGUpload,
		Scenario:    "attachment-upload-document",
		Payload:     uploadPayload,
		Attachments: []transcript.Attachment{attach},
		Note:        "oversized variant — 11MB octet-stream staged for pherald POST",
	})

	// Encode into a CloudEvent's data_base64 field per CloudEvents §3.1.
	encoded := base64.StdEncoding.EncodeToString(body)
	ce := herald.CloudEvent{
		SpecVersion: "1.0",
		ID:          fmt.Sprintf("qaherald-oversized-%d", o.now().UnixNano()),
		Source:      "qaherald",
		Type:        "qa.oversized-document",
		Time:        o.now().UTC(),
		DataB64:     encoded,
	}
	cePayload, _ := json.Marshal(map[string]any{
		"variant":     "oversized",
		"event_id":    ce.ID,
		"data_size":   len(body),
		"data_sha256": bodyHex,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "attachment-upload-document",
		Payload:   cePayload,
		Note:      "oversized variant — direct POST /v1/events (expect 413)",
	})

	receipt, status, headers, err := o.Herald.PostEvent(ctx, ce, herald.AcceptJSON)
	// Network errors here include httptest's read-body-too-large
	// signal; those count as a pherald-side rejection.
	receiptPayload, _ := json.Marshal(receipt)
	noteBase := fmt.Sprintf("oversized variant — status=%d content-type=%s", status, headers.Get("Content-Type"))
	if err != nil {
		noteBase += " err=" + err.Error()
	}
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindHeraldResponse,
		Scenario:  "attachment-upload-document",
		Payload:   receiptPayload,
		Note:      noteBase,
	})
	if status != http.StatusRequestEntityTooLarge {
		// Surface a structured error citing both the actual status
		// and the err (if any) so the report distinguishes
		// "pherald accepted the 11MB blob" (bluff) from "the
		// connection failed before pherald responded" (transport
		// issue worth investigating).
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("oversized variant: timeout before pherald responded — likely an over-quota request that the server stalled on rather than 413-rejected (status=%d)", status)
		}
		return fmt.Errorf("oversized variant: expected 413 Payload Too Large, got %d (likely pherald accepted the 11MB blob — §107 bluff)", status)
	}

	// Bidirectional sentinel TG event: the oversized path never
	// delivers a Telegram message (the POST was rejected), but the
	// invariant requires ≥1 tg.* event. Append a sentinel.
	sentinelPayload, _ := json.Marshal(map[string]any{
		"variant":      "oversized",
		"observed":     0,
		"sha256":       bodyHex,
		"status_seen":  status,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "attachment-upload-document",
		Payload:   sentinelPayload,
		Note:      "oversized variant — zero TG deliveries observed (pherald 413-rejected before fan-out)",
	})

	return nil
}
