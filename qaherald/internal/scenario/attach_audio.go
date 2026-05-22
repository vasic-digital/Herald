// Wave 5 Task 5 Scenario #7 — attachment-upload-audio.
//
// Operator-side flow identical to scenario #5 but with audio/ogg
// content type (which Telegram delivers as tele.Voice).
//
// §107 anti-bluff anchor: same as scenario #5 — full 32-byte sha256
// equality on uploaded vs round-tripped bytes.
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
		Name:        "attachment-upload-audio",
		Description: "Voice (audio/ogg) upload → pherald round-trip → sha256 match",
		Run:         runAttachAudio,
	})
}

func runAttachAudio(ctx context.Context, o *Orchestrator) error {
	body, contentType, filename := minimalOGGBytes()
	uploadedSum := sha256.Sum256(body)
	uploadedHex := hex.EncodeToString(uploadedSum[:])

	attach, err := o.Transcript.AttachFile(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("transcript.AttachFile (upload): %w", err)
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
		Scenario:    "attachment-upload-audio",
		Payload:     uploadPayload,
		Attachments: []transcript.Attachment{attach},
		Note:        "operator → bot upload (audio/ogg)",
	})

	msgID, fileID, err := o.TG.Upload(bytes.NewReader(body), contentType, filename)
	if err != nil {
		return fmt.Errorf("TG.Upload: %w", err)
	}

	heraldNotePayload, _ := json.Marshal(map[string]any{
		"telegram_message_id": msgID,
		"telegram_file_id":    fileID,
		"sha256":              uploadedHex,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionOut,
		Kind:      transcript.KindHeraldPost,
		Scenario:  "attachment-upload-audio",
		Payload:   heraldNotePayload,
		Note:      "bot upload → pherald CloudEvent ingest",
	})

	deliveredMsg, err := o.TG.WaitForMessage(60*time.Second, func(m tele.Message) bool {
		return m.Voice != nil && m.Voice.FileID != "" && m.Voice.FileID != fileID
	})
	if err != nil {
		return fmt.Errorf("WaitForMessage (pherald delivery): %w", err)
	}
	deliveredFileID := deliveredMsg.Voice.FileID
	deliveredPayload, _ := json.Marshal(map[string]any{
		"message_id":        deliveredMsg.ID,
		"delivered_file_id": deliveredFileID,
	})
	_ = o.Transcript.Append(transcript.Event{
		Direction: transcript.DirectionIn,
		Kind:      transcript.KindTGReceive,
		Scenario:  "attachment-upload-audio",
		Payload:   deliveredPayload,
		Note:      "pherald → tgram → chat delivery",
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
	dlAttach, err := o.Transcript.AttachFile(bytes.NewReader(downloadedBytes), contentType, "qa-roundtrip.ogg")
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
		Scenario:    "attachment-upload-audio",
		Payload:     downloadPayload,
		Attachments: []transcript.Attachment{dlAttach},
		Note:        "round-tripped voice bytes",
	})

	if !bytes.Equal(uploadedSum[:], downloadedSum[:]) {
		return fmt.Errorf("attach-audio: sha256 mismatch — uploaded=%s downloaded=%s", uploadedHex, downloadedHex)
	}
	return nil
}
