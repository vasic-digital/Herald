// Wave 5 Task 5 Scenario #5 — attachment-upload-photo.
//
// Operator-side flow: qaherald sends a photo to the Herald bot →
// pherald ingests as a CloudEvent with attachment → pherald
// fan-out delivers the attachment back to a subscriber → qaherald
// downloads the round-tripped fileID → asserts
// sha256(uploaded) == sha256(downloaded).
//
// §107 anti-bluff anchor: the byte-level sha256 comparison is the
// load-bearing assertion. T10 mutation gate (c) plants a tolerance
// (e.g. "any sha256 starting with the same 8 bytes passes"); this
// scenario FAILs because the comparison uses bytes.Equal on the full
// 32-byte digest.
package scenario

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

func init() {
	Register(Scenario{
		Name:        "attachment-upload-photo",
		Description: "Photo upload → pherald round-trip → sha256 match",
		Run:         runAttachPhoto,
	})
}

func runAttachPhoto(ctx context.Context, o *Orchestrator) error {
	body, contentType, filename := minimalPNGBytes()
	uploadedSum := sha256.Sum256(body)
	uploadedHex := hex.EncodeToString(uploadedSum[:])

	// Record the upload event with the attachment metadata before we
	// send the bytes over the wire. Doing this first means a network
	// failure mid-upload still leaves an auditable "we tried to upload
	// X" event in the transcript.
	attach, err := o.Transcript.AttachFile(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("transcript.AttachFile (upload): %w", err)
	}
	if attach.Sha256 != uploadedHex {
		return fmt.Errorf("attach-photo: pre-upload sha256 mismatch — wrote %q, transcript recorded %q (§107 bluff guard)", uploadedHex, attach.Sha256)
	}
	uploadPayload, _ := json.Marshal(map[string]any{
		"filename":     filename,
		"content_type": contentType,
		"sha256":       uploadedHex,
		"size_bytes":   len(body),
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction:   transcript.DirectionOut,
		Kind:        transcript.KindTGUpload,
		Scenario:    "attachment-upload-photo",
		Payload:     uploadPayload,
		Attachments: []transcript.Attachment{attach},
		Note:        "operator → bot upload (image/png)",
	})

	msgID, fileID, err := o.TG.Upload(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("TG.Upload: %w", err)
	}

	// The Herald-side trace: pherald sees this Telegram upload as an
	// inbound webhook → CloudEvent → fan-out. We record the bot-side
	// observation as a KindHeraldPost transcript event with the
	// msg+fileID metadata; the actual /v1/events POST is internal to
	// pherald and not directly observed by qaherald in this scenario.
	heraldNotePayload, _ := json.Marshal(map[string]any{
		"telegram_message_id": msgID,
		"telegram_file_id":    fileID,
		"sha256":              uploadedHex,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "attachment-upload-photo",
		Payload:   heraldNotePayload,
		Note:      "bot upload → pherald CloudEvent ingest (implicit via Telegram webhook)",
	})

	// Wait for pherald's fan-out delivery to land back in the chat as
	// a photo from the bot. Predicate: message has a Photo payload
	// whose FileUniqueID is not empty AND is distinct from the just-
	// uploaded fileID (Telegram re-IDs files on re-upload, so a
	// matching FileID would be a no-op echo we want to skip).
	deliveredMsg, err := o.TG.WaitForMessage(60*time.Second, func(m tele.Message) bool {
		return m.Photo != nil && m.Photo.FileID != "" && m.Photo.FileID != fileID
	})
	if err != nil {
		return fmt.Errorf("WaitForMessage (pherald delivery): %w", err)
	}
	deliveredFileID := deliveredMsg.Photo.FileID
	deliveredPayload, _ := json.Marshal(map[string]any{
		"message_id":          deliveredMsg.ID,
		"delivered_file_id":   deliveredFileID,
		"caption":             deliveredMsg.Caption,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "attachment-upload-photo",
		Payload:   deliveredPayload,
		Note:      "pherald → tgram → chat delivery",
	})

	// Download + sha256-compare the round-tripped file.
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

	dlAttach, err := o.Transcript.AttachFile(bytes.NewReader(downloadedBytes), contentType, "qa-roundtrip.png")
	if err != nil {
		return fmt.Errorf("transcript.AttachFile (download): %w", err)
	}
	downloadPayload, _ := json.Marshal(map[string]any{
		"file_id":    deliveredFileID,
		"sha256":     downloadedHex,
		"size_bytes": len(downloadedBytes),
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction:   transcript.DirectionIn,
		Kind:        transcript.KindTGDownload,
		Scenario:    "attachment-upload-photo",
		Payload:     downloadPayload,
		Attachments: []transcript.Attachment{dlAttach},
		Note:        "round-tripped photo bytes",
	})

	// §107 load-bearing assertion: full-digest equality.
	if !bytes.Equal(uploadedSum[:], downloadedSum[:]) {
		return fmt.Errorf("attach-photo: sha256 mismatch — uploaded=%s downloaded=%s (§107 bluff: bytes are NOT round-tripping)", uploadedHex, downloadedHex)
	}
	return nil
}
